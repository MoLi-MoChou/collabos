# Deploying CollabOS (docker compose)

The server is a single Go binary that also serves the built web client
(`COLLAB_WEB_DIR`) and applies migrations on start (`COLLAB_AUTO_MIGRATE=true`).
Files: `deploy/Dockerfile`, `deploy/docker-compose.yml`.

## Build (on a machine with the repo)

```bash
pnpm install --frozen-lockfile && pnpm --filter @collabos/web build
cd server && CGO_ENABLED=0 go build -o bin/server ./cmd/server && CGO_ENABLED=0 go build -o bin/collab ./cmd/collab
mkdir -p /tmp/bundle/web && cp bin/server bin/collab /tmp/bundle/ && cp -r ../apps/web/dist/* /tmp/bundle/web/
cp ../deploy/Dockerfile ../deploy/docker-compose.yml /tmp/bundle/
tar cz -C /tmp/bundle . | ssh root@HOST 'mkdir -p /opt/collabos && tar xz -C /opt/collabos'
```

## Database

Any PostgreSQL 16+ works (tested on 18). Create a dedicated role and database:

```sql
CREATE ROLE collabos LOGIN PASSWORD '<random>';
CREATE DATABASE collabos OWNER collabos;
REVOKE ALL ON DATABASE collabos FROM PUBLIC;
```

The sample compose file joins an existing external docker network (`1panel-network`)
where the Postgres container is reachable as `pgsql`; adjust `networks` and the host
in `DATABASE_URL` for your setup, or add your own `postgres` service.

## /opt/collabos/.env (chmod 600)

```
DATABASE_URL=postgres://collabos:<random>@pgsql:5432/collabos?sslmode=disable
COLLAB_ALLOWED_ORIGIN=http://HOST:8190
COLLAB_DEV_LOGIN=true
COLLAB_DEV_LOGIN_PASSWORD=<random access code>
COLLAB_DEV_LOGIN_EXISTING_ONLY=true   # false while seeding accounts
COLLAB_DEFAULT_MODEL=opencode/minimax-m3.1-flash-free
```

Start: `cd /opt/collabos && docker compose up -d --build` (restart policy
`unless-stopped`, published on port 8190). Open the port in ufw/cloud firewall.
Seed demo data with `node scripts/seed-preview.mjs http://HOST:8190 <remote> <access code>`.

## Connecting a daemon from another computer

1. In the web app (or `POST /api/daemons`) register a device and copy its node token.
2. On that computer, with `collab` and `opencode` on PATH (and bubblewrap on Linux):

```bash
collab login --server http://HOST:8190 --token <NODE_TOKEN>
collab repo add /path/to/checkout --remote <project remote URL>
collab run            # COLLAB_HOME defaults to ~/.collabos
```

The daemon connects outbound to `ws://HOST:8190/daemon/ws`; no inbound port is needed.

## Windows (desktop app + daemon)

The `desktop` workflow (windows-latest) runs the daemon tests on Windows, builds
`collab.exe`, and produces per-user installers (`CollabOS_*_x64-setup.exe`, NSIS
`currentUser`, plus an `.msi`) as the `collabos-windows` artifact. Run it manually with
a different `server_url` to bake another default server.

- App: installs to `%LOCALAPPDATA%\CollabOS` (no admin). It opens `server_url` from
  `%APPDATA%\com.collabos.desktop\config.json` (written on first run; env
  `COLLABOS_SERVER_URL` overrides). Set `"start_daemon": true` to have the app start the
  bundled `collab.exe run` and stop it on exit.
- Daemon: `collab login --server URL --token NODE_TOKEN`, `collab repo add PATH --remote URL`,
  `collab run`. Config lives in `%USERPROFILE%\.collabos`. Phase 1: sandbox `none`, so the
  daemon only accepts its owner's own (self-delegated) tasks. Project test commands run via
  `cmd.exe /c`. Pause/resume suspends the agent process tree (NtSuspendProcess).
