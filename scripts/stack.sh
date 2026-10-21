#!/usr/bin/env bash
# Starts a self-contained CollabOS stack for e2e runs and previews:
# a fresh Postgres database, a bare Git remote with a sample project, two
# developer checkouts, and the server serving the production web build.
#
#   scripts/stack.sh DIR PORT DBNAME
#
# Environment: PGURL (admin URL used to (re)create DBNAME, default the dev DB
# role), plus any COLLAB_* server variables, which are passed through.
set -euo pipefail
DIR=$(realpath -m "${1:?dir}"); PORT=${2:?port}; DBNAME=${3:?dbname}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
PGBASE=${PGBASE:-postgres://collab:collab@localhost:5432}

mkdir -p "$DIR"
if [ -f "$DIR/server.pid" ]; then kill "$(cat "$DIR/server.pid")" 2>/dev/null || true; fi
for p in "$DIR"/daemon-*.pid; do [ -f "$p" ] && kill "$(cat "$p")" 2>/dev/null || true; done
sleep 0.5
rm -rf "${DIR:?}"/*

if [ "${SKIP_BUILD:-}" != 1 ]; then
  (cd "$ROOT/server" && go build -o bin/server ./cmd/server && go build -o bin/collab ./cmd/collab && go build -o bin/migrate ./cmd/migrate)
  (cd "$ROOT" && pnpm --filter @collabos/web build >/dev/null)
fi

psql "$PGBASE/postgres" -qc "DROP DATABASE IF EXISTS \"$DBNAME\" WITH (FORCE)" >/dev/null
psql "$PGBASE/postgres" -qc "CREATE DATABASE \"$DBNAME\"" >/dev/null

# Sample project: a tiny TypeScript library with a README.
SEED="$DIR/seed"; mkdir -p "$SEED/src"
cat > "$SEED/README.md" <<'MD'
# calc

A tiny calculator library used by the CollabOS demo.
MD
cat > "$SEED/src/math.ts" <<'TS'
export function add(a: number, b: number): number {
  return a + b;
}
TS
git -C "$SEED" init -q -b main
git -C "$SEED" -c user.name=seed -c user.email=seed@example.invalid add -A
git -C "$SEED" -c user.name=seed -c user.email=seed@example.invalid commit -qm "init"
git clone -q --bare "$SEED" "$DIR/remote.git"
for who in a b c; do
  git clone -q "$DIR/remote.git" "$DIR/repo-$who"
done

export DATABASE_URL="$PGBASE/$DBNAME?sslmode=disable"
export COLLAB_ADDR=":$PORT" COLLAB_AUTO_MIGRATE=true COLLAB_WEB_DIR="$ROOT/apps/web/dist"
export COLLAB_ALLOWED_ORIGIN=${COLLAB_ALLOWED_ORIGIN:-http://localhost:$PORT}
# server-start.sh (re)starts the server with the same settings; e2e uses it
# to simulate a server outage and the preview uses it from start-preview.sh.
{
  echo '#!/usr/bin/env bash'
  for v in DATABASE_URL COLLAB_ADDR COLLAB_AUTO_MIGRATE COLLAB_WEB_DIR COLLAB_ALLOWED_ORIGIN COLLAB_OFFLINE_GRACE \
    COLLAB_DEV_LOGIN COLLAB_DEV_LOGIN_PASSWORD COLLAB_DEV_LOGIN_EXISTING_ONLY COLLAB_DEFAULT_MODEL COLLAB_HEARTBEAT_SECONDS; do
    if [ -n "${!v:-}" ]; then printf 'export %s=%q\n' "$v" "${!v}"; fi
  done
  echo "nohup '$ROOT/server/bin/server' >>'$DIR/server.log' 2>&1 &"
  echo "echo \$! >'$DIR/server.pid'"
} >"$DIR/server-start.sh"
chmod 700 "$DIR/server-start.sh"
bash "$DIR/server-start.sh"
for _ in $(seq 1 50); do
  curl -sf "http://localhost:$PORT/api/config" >/dev/null && break
  sleep 0.2
done
curl -sf "http://localhost:$PORT/api/config" >/dev/null || { cat "$DIR/server.log"; exit 1; }
echo "stack up: http://localhost:$PORT  remote=$DIR/remote.git"
