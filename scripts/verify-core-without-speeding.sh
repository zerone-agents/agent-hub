#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

"$repo_root/scripts/check-platform-neutrality.sh"

tmp_dir="$(mktemp -d)"
server_pid=""
cleanup() {
  if [[ -n "$server_pid" ]] && kill -0 "$server_pid" 2>/dev/null; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

# The verification catalog may compile inert example manifests for the product
# acceptance page. It does not install them, execute their tools, mutate Core
# state or make Hub startup depend on a vertical capability package.
go build -o "$tmp_dir/agent-hub" ./cmd/server

if [[ -z "${H0_DATABASE_URL:-}" ]]; then
  echo "core-without-speeding: build passed (set H0_DATABASE_URL to include the startup health check)"
  exit 0
fi

port="${H0_SERVER_PORT:-18081}"
log_file="$tmp_dir/server.log"
DATABASE_URL="$H0_DATABASE_URL" \
AUTH_MODE=builtin \
SERVER_HOST=127.0.0.1 \
SERVER_PORT="$port" \
"$tmp_dir/agent-hub" >"$log_file" 2>&1 &
server_pid=$!

for _ in {1..40}; do
  if curl --fail --silent "http://127.0.0.1:${port}/health" >"$tmp_dir/health.json" 2>/dev/null; then
    echo "core-without-speeding: build and startup health check passed"
    exit 0
  fi
  if ! kill -0 "$server_pid" 2>/dev/null; then
    echo "core-without-speeding: server exited before becoming healthy" >&2
    cat "$log_file" >&2
    exit 1
  fi
  sleep 0.5
done

echo "core-without-speeding: health check timed out" >&2
cat "$log_file" >&2
exit 1
