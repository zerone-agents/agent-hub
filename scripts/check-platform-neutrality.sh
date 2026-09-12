#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if ! command -v rg >/dev/null 2>&1; then
  echo "platform-neutrality: ripgrep (rg) is required" >&2
  exit 2
fi

# Scan shipped platform sources only. Consumer examples, design documents and
# tests may name vertical applications because they are contract fixtures.
scan_paths=(cmd internal pkg frontend/src quickstart)

matches="$({
  rg -n -i 'speeding|com\.speeding' "${scan_paths[@]}" \
    --glob '!**/*_test.go' \
    --glob '!**/*.test.*' \
    --glob '!**/__tests__/**' \
    --glob '!**/testdata/**' \
    --glob '!frontend/src/features/extensions/ExtensionAcceptancePage.tsx' \
    --glob '!cmd/extension-validate/**' || true
})"

if [[ -n "$matches" ]]; then
  echo "platform-neutrality: vertical application terms found in shipped platform sources:" >&2
  echo "$matches" >&2
  exit 1
fi

echo "platform-neutrality: passed"
