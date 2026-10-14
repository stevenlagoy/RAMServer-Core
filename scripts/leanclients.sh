#!/usr/bin/env bash
# Opens several leanclients in one tmux window, one pane each, named Player1..PlayerN.
# Usage: scripts/leanclients.sh [count] [leanclient flags...]
#   scripts/leanclients.sh              3 clients
#   scripts/leanclients.sh 4 -addr 127.0.0.1:9001
# Close them all with: tmux kill-session -t leanclients
set -euo pipefail

session=leanclients
count=3
if [[ $# -gt 0 && $1 =~ ^[0-9]+$ ]]; then
    count=$1
    shift
fi
if (( count < 1 )); then
    echo "usage: $0 [count] [leanclient flags...]" >&2
    exit 1
fi
if ! command -v tmux >/dev/null; then
    echo "tmux is not installed" >&2
    exit 1
fi
if tmux has-session -t "$session" 2>/dev/null; then
    echo "tmux session '$session' already exists; close it with: tmux kill-session -t $session" >&2
    exit 1
fi

# Build once so every pane starts immediately instead of compiling with go run
bin="${TMPDIR:-/tmp}/ramserver-leanclient"
(cd "$(dirname "$0")/../server" && go build -o "$bin" ./cmd/leanclient)

client() { # command line for client $1, with extra flags quoted safely
    printf '%q -name Player%d' "$bin" "$1"
    (( $# > 1 )) && printf ' %q' "${@:2}"
    echo
}

tmux new-session -d -s "$session" -x 200 -y 50 "$(client 1 "$@")"
tmux set-option -t "$session" remain-on-exit on # keep a pane open if its client exits, so errors stay visible
for i in $(seq 2 "$count"); do
    tmux split-window -t "$session" "$(client "$i" "$@")"
    tmux select-layout -t "$session" tiled
done

if [[ -n ${TMUX:-} ]]; then
    tmux switch-client -t "$session"
else
    tmux attach -t "$session"
fi
