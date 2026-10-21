#!/usr/bin/env bash
# Logs in and starts one collab daemon with its own COLLAB_HOME.
#   scripts/daemon.sh DIR NAME SERVER_URL NODE_TOKEN REPO_PATH REMOTE_URL
set -euo pipefail
DIR=$1; NAME=$2; SERVER=$3; TOKEN=$4; REPO=$5; REMOTE=$6
ROOT=$(cd "$(dirname "$0")/.." && pwd)
export COLLAB_HOME="$DIR/home-$NAME"
mkdir -p "$COLLAB_HOME"
"$ROOT/server/bin/collab" login --server "$SERVER" --token "$TOKEN" >/dev/null
"$ROOT/server/bin/collab" repo add "$REPO" --remote "$REMOTE" >/dev/null
nohup "$ROOT/server/bin/collab" run >>"$DIR/daemon-$NAME.log" 2>&1 &
echo $! >"$DIR/daemon-$NAME.pid"
echo "daemon $NAME pid $(cat "$DIR/daemon-$NAME.pid")"
