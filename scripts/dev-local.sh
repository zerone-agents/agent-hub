#!/bin/bash
# Local dev launcher: starts the Agent Hub backend with an embedded SQLite
# database (no Docker/MySQL needed). MySQL remains the production path —
# set DATABASE_URL to a MySQL DSN to use it instead.
set -e
cd "$(dirname "$0")/.."
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export DATABASE_URL="${DATABASE_URL:-sqlite:./agent_hub.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)}"
export AUTH_JWT_SECRET="${AUTH_JWT_SECRET:-devsecret0123456789abcdef0123456789abcdef}"
export SERVER_PORT="${PORT:-7100}"
exec go run ./cmd/server
