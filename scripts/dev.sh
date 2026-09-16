#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

case "${1:-}" in
    generate) buf generate ;;
    lint)     buf lint && (cd server && go vet ./...) ;;
    test)     (cd server && go test -race ./...) ;;
    build)    (cd server && go build ./...) ;;
    up)       docker compose up --build ;;
    down)     docker compose down ;;
    *)
        echo "usage: $0 {generate|lint|test|build|up|down}" >&2
        exit 1
        ;;
esac