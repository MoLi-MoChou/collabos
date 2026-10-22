use serde::{Deserialize, Serialize};
use std::path::Path;

/// Server baked in at build time (`COLLABOS_DEFAULT_SERVER`), overridable per user.
pub const DEFAULT_SERVER: &str = match option_env!("COLLABOS_DEFAULT_SERVER") {
    Some(s) => s,
    None => "http://localhost:8080",
};

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Config {
    pub server_url: String,
    /// Start the bundled `collab run` daemon with the app.
    pub start_daemon: bool,
}

impl Default for Config {
    fn default() -> Self {
        Config { server_url: DEFAULT_SERVER.into(), start_daemon: false }
    }
}

/// Reads `config.json` from `dir`, writing defaults on first run. An env
/// override wins over the file without being persisted.
pub fn load_or_init(dir: &Path, env_override: Option<String>) -> Config {
    let path = dir.join("config.json");
    let mut cfg = match std::fs::read_to_string(&path) {
        Ok(s) => serde_json::from_str(&s).unwrap_or_default(),
        Err(_) => {
            let c = Config::default();
            let _ = std::fs::create_dir_all(dir);
            let _ = std::fs::write(&path, serde_json::to_string_pretty(&c).unwrap());
            c
        }
    };
    if let Some(u) = env_override.filter(|u| !u.trim().is_empty()) {
        cfg.server_url = u;
    }
    cfg
}

/// Accepts only http(s) URLs; anything else falls back to the offline page.
pub fn parse_server_url(s: &str) -> Option<url::Url> {
    let u = url::Url::parse(s.trim()).ok()?;
    matches!(u.scheme(), "http" | "https").then_some(u)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn first_run_writes_defaults_and_reads_back() {
        let d = std::env::temp_dir().join(format!("collabos-cfg-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&d);
        let a = load_or_init(&d, None);
        assert_eq!(a, Config::default());
        std::fs::write(d.join("config.json"), r#"{"server_url":"https://x.example","start_daemon":true}"#).unwrap();
        let b = load_or_init(&d, None);
        assert_eq!(b.server_url, "https://x.example");
        assert!(b.start_daemon);
        let c = load_or_init(&d, Some("http://env.example:1".into()));
        assert_eq!(c.server_url, "http://env.example:1");
        let _ = std::fs::remove_dir_all(&d);
    }

    #[test]
    fn only_http_urls_are_loaded() {
        assert!(parse_server_url("http://38.76.166.121:8190").is_some());
        assert!(parse_server_url(" https://a.b/ ").is_some());
        assert!(parse_server_url("file:///etc/passwd").is_none());
        assert!(parse_server_url("javascript:alert(1)").is_none());
        assert!(parse_server_url("").is_none());
    }
}
