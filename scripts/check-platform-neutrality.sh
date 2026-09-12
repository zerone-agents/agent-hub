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
    --glob '!cmd/extension-validate/**' || true
})"

if [[ -n "$matches" ]]; then
  echo "platform-neutrality: vertical application terms found in shipped platform sources:" >&2
  echo "$matches" >&2
  exit 1
fi

# Brand-neutral names are necessary but not sufficient. These identifiers are
# known vertical/game relationship state that must be supplied by a capability
# package, never owned by Hub Core. Tests and external fixtures remain valid
# consumer contract coverage and are intentionally excluded.
semantic_matches="$({
  rg -n -i '\b(wealth|net_?worth|game_?turn|victory_?condition|relationship_?score|default_?stance|public_?humiliation|credit_?stolen)\b' "${scan_paths[@]}" \
    --glob '!**/*_test.go' \
    --glob '!**/*.test.*' \
    --glob '!**/__tests__/**' \
    --glob '!**/testdata/**' \
    --glob '!cmd/extension-validate/**' || true
})"

# H3.1 keeps the former relationship-dynamics storage/API readable until H6
# migrates historical records into an explicit capability package. These exact
# files form the audited compatibility boundary; routing, prompts, MCP and
# product UI are deliberately not allowlisted.
semantic_matches="$(printf '%s\n' "$semantic_matches" | rg -v '^(internal/domain/agentrelation/(model|dynamics)\.go|internal/infrastructure/persistence/agent_relation_repository\.go|internal/application/services/(agent_relation_service|relation_type_service)\.go|internal/handler/(agent_relation|relation_type)\.go|pkg/database/database\.go|frontend/src/api/(agent-relations|relation-types)\.ts):' || true)"

if [[ -n "$semantic_matches" ]]; then
  echo "platform-neutrality: vertical semantics found in shipped platform sources:" >&2
  echo "$semantic_matches" >&2
  exit 1
fi

echo "platform-neutrality: passed"
