#!/usr/bin/env bash
# Starts a local server and several Python chess clients, then stops the server when every client has closed.
# Usage: scripts/chess.sh [count]
#   scripts/chess.sh       2 clients
#   scripts/chess.sh 4     4 clients (two matches)
# The clients connect to 127.0.0.1:9000, so stop any other server on that port first (e.g. scripts/dev.sh down).
# Server output goes to a log file; client output is printed here, prefixed with the client number.
set -euo pipefail

cd "$(dirname "$0")/.."

count=2
if [[ $# -gt 0 ]]; then
    if ! [[ $1 =~ ^[0-9]+$ ]] || (( $1 < 1 )); then
        echo "usage: $0 [count]" >&2
        exit 1
    fi
    count=$1
fi

addr=127.0.0.1:9000
client_dir=clients/chess/interpreted_client
if (exec 3<>"/dev/tcp/${addr%:*}/${addr#*:}") 2>/dev/null; then
    echo "something is already listening on $addr; stop it first (scripts/dev.sh down)" >&2
    exit 1
fi
if [[ ! -d $client_dir/gen ]]; then
    buf generate
fi
python=python3
if [[ -x $client_dir/.venv/bin/python ]]; then
    python="$(cd "$client_dir" && pwd)/.venv/bin/python" # absolute, but not resolved: the symlink is what selects the venv
fi

server_bin="${TMPDIR:-/tmp}/ramserver-chess"
server_log="${TMPDIR:-/tmp}/ramserver-chess.log"
(cd server && go build -o "$server_bin" ./cmd/ramserver)

pids=()
cleanup() {
    kill "${pids[@]}" "${server_pid:-}" 2>/dev/null || true
    wait 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 130' INT TERM

RAMSERVER_ADDR=$addr "$server_bin" >"$server_log" 2>&1 &
server_pid=$!
for _ in $(seq 50); do # wait up to 5s for the server to accept connections
    if (exec 3<>"/dev/tcp/${addr%:*}/${addr#*:}") 2>/dev/null; then
        break
    fi
    if ! kill -0 "$server_pid" 2>/dev/null; then
        echo "server exited; see $server_log" >&2
        exit 1
    fi
    sleep 0.1
done
echo "server running on $addr (log: $server_log)"

for i in $(seq "$count"); do
    # -u: unbuffered, so client output appears as it happens
    (cd "$client_dir" && exec "$python" -u main.py) > >(sed -u "s/^/[client $i] /") 2>&1 &
    pids+=($!)
    sleep 0.5 # let each client join before the next, so they pair up in order
done

echo "started $count client(s); close their windows or press Ctrl+C to stop"
wait "${pids[@]}" 2>/dev/null || true
