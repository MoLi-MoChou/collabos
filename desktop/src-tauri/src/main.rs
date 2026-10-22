#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod config;

use std::sync::Mutex;
use tauri::{Manager, RunEvent, WebviewUrl, WebviewWindowBuilder};
use tauri_plugin_shell::{process::CommandChild, ShellExt};

struct Daemon(Mutex<Option<CommandChild>>);

fn main() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .manage(Daemon(Mutex::new(None)))
        .setup(|app| {
            let dir = app.path().app_config_dir()?;
            let cfg = config::load_or_init(&dir, std::env::var("COLLABOS_SERVER_URL").ok());
            let url = config::parse_server_url(&cfg.server_url)
                .map(WebviewUrl::External)
                .unwrap_or_else(|| WebviewUrl::App("index.html".into()));
            WebviewWindowBuilder::new(app, "main", url)
                .title("CollabOS")
                .inner_size(1280.0, 820.0)
                .min_inner_size(900.0, 600.0)
                .build()?;
            if cfg.start_daemon {
                // The bundled daemon uses the same ~/.collabos config as the CLI
                // (`collab login`); its own lock prevents a second instance.
                match app.shell().sidecar("collab").and_then(|c| c.args(["run"]).spawn()) {
                    Ok((_rx, child)) => *app.state::<Daemon>().0.lock().unwrap() = Some(child),
                    Err(e) => eprintln!("collab sidecar failed to start: {e}"),
                }
            }
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("failed to build CollabOS");
    app.run(|handle, ev| {
        if let RunEvent::Exit = ev {
            if let Some(child) = handle.state::<Daemon>().0.lock().unwrap().take() {
                let _ = child.kill();
            }
        }
    });
}
