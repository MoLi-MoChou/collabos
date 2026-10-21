#!/usr/bin/env bash
# Starts (or restarts) the public demo preview on this machine:
# server + production web on :$PORT, two daemons (张三, 李四) and a Cloudflare
# quick tunnel. Login requires the demo access code and only the seeded demo
# accounts can sign in.
#
#   ./start-preview.sh          restart everything, keeping data
#   ./start-preview.sh --reset  wipe the preview database and re-seed
set -euo pipefail
ROOT=$(cd "$(dirname "$0")" && pwd)
DIR=${PREVIEW_DIR:-$HOME/collabos-preview}
PORT=${PREVIEW_PORT:-8100}
DB=collab_preview
CLOUDFLARED=${CLOUDFLARED:-$(command -v cloudflared || echo "$HOME/.local/bin/cloudflared")}
export PATH="$HOME/.opencode/bin:$PATH"

stop() { [ -f "$DIR/$1.pid" ] && kill "$(cat "$DIR/$1.pid")" 2>/dev/null || true; }

if [ "${1:-}" = "--reset" ] || [ ! -f "$DIR/server-start.sh" ]; then
  stop tunnel
  PASS="demo-$(head -c 4 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  COLLAB_DEV_LOGIN_PASSWORD=$PASS COLLAB_DEV_LOGIN_EXISTING_ONLY=false \
    "$ROOT/scripts/stack.sh" "$DIR" "$PORT" "$DB"
  eval "$(node "$ROOT/scripts/seed-preview.mjs" "http://localhost:$PORT" "$DIR/remote.git" "$PASS")"
  "$ROOT/scripts/daemon.sh" "$DIR" zs "http://localhost:$PORT" "$ZS_TOKEN" "$DIR/repo-a" "$DIR/remote.git"
  "$ROOT/scripts/daemon.sh" "$DIR" ls "http://localhost:$PORT" "$LS_TOKEN" "$DIR/repo-b" "$DIR/remote.git"
  echo "$PASS" >"$DIR/access-code"
  # From now on only the seeded accounts may sign in.
  sed -i 's/^export COLLAB_DEV_LOGIN_EXISTING_ONLY=.*/export COLLAB_DEV_LOGIN_EXISTING_ONLY=true/' "$DIR/server-start.sh"
  grep -q EXISTING_ONLY "$DIR/server-start.sh" || sed -i '2i export COLLAB_DEV_LOGIN_EXISTING_ONLY=true' "$DIR/server-start.sh"
fi

stop server
for n in zs ls; do stop "daemon-$n"; done
sleep 1
bash "$DIR/server-start.sh"
for _ in $(seq 1 50); do curl -sf "http://localhost:$PORT/api/config" >/dev/null && break; sleep 0.2; done
for n in zs ls; do
  COLLAB_HOME="$DIR/home-$n" nohup "$ROOT/server/bin/collab" run >>"$DIR/daemon-$n.log" 2>&1 &
  echo $! >"$DIR/daemon-$n.pid"
done

if [ "${1:-}" = "--reset" ] || ! kill -0 "$(cat "$DIR/tunnel.pid" 2>/dev/null)" 2>/dev/null; then
  stop tunnel
  : >"$DIR/tunnel.log"
  nohup "$CLOUDFLARED" tunnel --no-autoupdate --url "http://localhost:$PORT" >"$DIR/tunnel.log" 2>&1 &
  echo $! >"$DIR/tunnel.pid"
  for _ in $(seq 1 60); do grep -qo 'https://[a-z0-9-]*\.trycloudflare\.com' "$DIR/tunnel.log" && break; sleep 1; done
fi
URL=$(grep -o 'https://[a-z0-9-]*\.trycloudflare\.com' "$DIR/tunnel.log" | head -1)
echo "preview:     $URL"
echo "local:       http://localhost:$PORT"
echo "access code: $(cat "$DIR/access-code")"
