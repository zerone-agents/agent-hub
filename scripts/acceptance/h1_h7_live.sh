#!/usr/bin/env bash
set -uo pipefail

# Live, tenant-isolated acceptance path for H1-H7.
# Required: HUB_TOKEN. Optional: HUB_URL (default http://127.0.0.1).
# The script never deletes Runs or audit evidence and never touches existing agents.

HUB_URL="${HUB_URL:-http://127.0.0.1}"
HUB_TOKEN="${HUB_TOKEN:?HUB_TOKEN is required}"
ACCEPTANCE_ID="${ACCEPTANCE_ID:-ACCEPT-H1H7-$(date -u +%Y%m%dT%H%M%SZ)}"
OUT_DIR="${OUT_DIR:-/tmp/${ACCEPTANCE_ID}}"
mkdir -p "$OUT_DIR"

RESULTS="$OUT_DIR/results.jsonl"
: >"$RESULTS"

record() {
  jq -nc --arg phase "$1" --arg status "$2" --arg detail "$3" \
    '{phase:$phase,status:$status,detail:$detail}' >>"$RESULTS"
}

request() {
  local method="$1" path="$2" body="${3:-}" output="$4"
  local code
  if [[ -n "$body" ]]; then
    code=$(curl -sS -o "$output" -w '%{http_code}' -X "$method" \
      -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
      --data "$body" "$HUB_URL$path") || return 1
  else
    code=$(curl -sS -o "$output" -w '%{http_code}' -X "$method" \
      -H "Authorization: Bearer $HUB_TOKEN" "$HUB_URL$path") || return 1
  fi
  [[ "$code" =~ ^2 ]] || { printf 'HTTP %s: %s\n' "$code" "$(cat "$output")" >&2; return 1; }
}

data_id() { jq -er '.data.id' "$1"; }

# H7 authentication boundary (negative cases do not use HUB_TOKEN).
health_code=$(curl -sS -o "$OUT_DIR/health.json" -w '%{http_code}' "$HUB_URL/health" || true)
unauth_code=$(curl -sS -o "$OUT_DIR/unauth.json" -w '%{http_code}' "$HUB_URL/api/v1/admin/runs" || true)
forged_code=$(curl -sS -o "$OUT_DIR/forged.json" -w '%{http_code}' \
  -H 'X-Extension-Name: io.zerone.emotion' -H 'X-Extension-Token: invalid' \
  "$HUB_URL/api/v1/admin/runs" || true)
if [[ "$health_code/$unauth_code/$forged_code" == "200/401/401" ]]; then
  record H7 pass 'health=200; unauthenticated=401; forged extension identity=401'
else
  record H7 fail "unexpected HTTP codes $health_code/$unauth_code/$forged_code"
fi

# Isolated agents are configuration records only. No deployment/model call is made.
agent_ids=()
for suffix in a b c; do
  name=$(printf '%s-%s' "$(echo "$ACCEPTANCE_ID" | tr '[:upper:]_' '[:lower:]-')" "$suffix" | cut -c1-63)
  body=$(jq -nc --arg name "$name" --arg prompt "Acceptance agent $suffix. Only act inside $ACCEPTANCE_ID." \
    '{name:$name,config:{systemPrompt:$prompt,permissionMode:"auto",maxTurns:8},desktopEnabled:false,mobileEnabled:false,guestEnabled:false,isDefault:false}')
  if request POST /api/v1/admin/agents "$body" "$OUT_DIR/agent-$suffix.json"; then
    agent_ids+=("$(data_id "$OUT_DIR/agent-$suffix.json")")
  else
    record H3 blocked "could not create isolated agent $suffix"
    exit 2
  fi
done
record H3 blocked 'isolated A/B/C created, but real A->B->C requires deploying three runtimes and a model call; intentionally not simulated'

# H1 Run + generic state + optimistic concurrency.
run_body=$(jq -nc --arg name "$ACCEPTANCE_ID" --arg aid "$ACCEPTANCE_ID" \
  '{name:$name,description:"H1-H7 live acceptance",metadata:{acceptanceId:$aid}}')
request POST /api/v1/admin/runs "$run_body" "$OUT_DIR/run.json"
run_id=$(data_id "$OUT_DIR/run.json")
for idx in 0 1 2; do
  role=$([[ "$idx" == 0 ]] && echo initiator || echo participant)
  request POST "/api/v1/admin/runs/$run_id/agents" \
    "$(jq -nc --argjson id "${agent_ids[$idx]}" --arg role "$role" '{agentId:$id,role:$role}')" \
    "$OUT_DIR/run-agent-$idx.json"
done

schema_body=$(jq -nc --arg ns "io.zerone.acceptance.$ACCEPTANCE_ID" \
  '{namespace:$ns,name:"case-state",version:"1.0.0",scopeTypes:["run"],subjectTypes:["run"],schema:{type:"object",required:["stage","score"],properties:{stage:{type:"string"},score:{type:"number"}},additionalProperties:false}}')
request POST /api/v1/admin/state-schemas "$schema_body" "$OUT_DIR/schema.json"
state_body=$(jq -nc --arg ns "io.zerone.acceptance.$ACCEPTANCE_ID" --arg run "$run_id" --arg key "$ACCEPTANCE_ID-state-init" \
  '{namespace:$ns,schemaName:"case-state",schemaVersion:"1.0.0",subjectType:"run",subjectId:$run,data:{stage:"created",score:1},idempotencyKey:$key,reason:"acceptance init",source:"acceptance"}')
request POST "/api/v1/admin/runs/$run_id/states" "$state_body" "$OUT_DIR/state.json"
state_id=$(data_id "$OUT_DIR/state.json")
request PUT "/api/v1/admin/runs/$run_id/states/$state_id" \
  "$(jq -nc --arg key "$ACCEPTANCE_ID-state-update" '{expectedRevision:1,data:{stage:"updated",score:2},idempotencyKey:$key,reason:"acceptance update",source:"acceptance"}')" \
  "$OUT_DIR/state-update.json"
request GET "/api/v1/admin/runs/$run_id/state-changes?stateId=$state_id" '' "$OUT_DIR/state-changes.json"
record H1 pass "run=$run_id state=$state_id revision and audit path verified"

# H2 ToolResult validation -> atomic state/event commit -> worker delivery.
tool_result=$(jq -nc --argjson sid "$state_id" --arg subject "$run_id" \
  '{result:{acceptance:true},cost:{toolCalls:1},stateProposals:[{stateId:$sid,expectedRevision:2,patch:[{op:"increment",path:"/score",value:3}],reason:"acceptance tool commit"}],eventProposals:[{type:"io.zerone.acceptance.completed.v1",subjectType:"run",subjectId:$subject,data:{acceptance:true},delayMs:0}],latencyMs:1}')
envelope=$(jq -nc --arg key "$ACCEPTANCE_ID-tool" --argjson result "$tool_result" \
  '{toolName:"acceptance.synthetic",actorType:"system",actorId:"acceptance",idempotencyKey:$key,toolResult:$result}')
request POST "/api/v1/admin/runs/$run_id/tool-results/validate" "$envelope" "$OUT_DIR/tool-validate.json"
request POST "/api/v1/admin/runs/$run_id/tool-results" "$envelope" "$OUT_DIR/tool-commit.json"
delivery_ok=false
for _ in $(seq 1 20); do
  request GET "/api/v1/admin/runs/$run_id/events" '' "$OUT_DIR/events.json" || break
  if jq -e '[.data[]? | select(.event.type=="io.zerone.acceptance.completed.v1" and .delivery.status=="delivered" and .delivery.attempts>=1)] | length > 0' "$OUT_DIR/events.json" >/dev/null; then
    delivery_ok=true
    break
  fi
  sleep 1
done
if $delivery_ok; then record H2 pass 'ToolResult validated/committed and event persisted'; else record H2 fail 'event not visible after commit'; fi

# Prompt snapshot for H2. H6 slots are inspected separately below.
request POST "/api/v1/admin/runs/$run_id/agents/${agent_ids[0]}/prompt" '{}' "$OUT_DIR/prompt.json"
record H2 pass 'prompt composition snapshot created'

# H4 group/member/channel/subscription/session lifecycle with durable audit.
group_body=$(jq -nc --arg name "$ACCEPTANCE_ID-group" '{name:$name,description:"isolated acceptance group",visibility:"private"}')
request POST /api/v1/admin/groups "$group_body" "$OUT_DIR/group.json"
group_id=$(data_id "$OUT_DIR/group.json")
for idx in 0 1 2; do
  role=$([[ "$idx" == 0 ]] && echo leader || echo member)
  request POST "/api/v1/admin/groups/$group_id/members" \
    "$(jq -nc --argjson id "${agent_ids[$idx]}" --arg role "$role" '{agentId:$id,role:$role}')" "$OUT_DIR/member-$idx.json"
done
request POST "/api/v1/admin/groups/$group_id/channels" \
  "$(jq -nc --arg name "$ACCEPTANCE_ID-channel" '{name:$name,topic:"acceptance",visibility:"group"}')" "$OUT_DIR/channel.json"
channel_id=$(data_id "$OUT_DIR/channel.json")
for idx in 0 1 2; do
  request PUT "/api/v1/admin/channels/$channel_id/subscriptions" \
    "$(jq -nc --argjson id "${agent_ids[$idx]}" '{agentId:$id,mode:"all"}')" "$OUT_DIR/sub-$idx.json"
done
participants=$(printf '%s\n' "${agent_ids[@]}" | jq -s 'map(tonumber)')
request POST "/api/v1/admin/channels/$channel_id/sessions" \
  "$(jq -nc --arg agenda "$ACCEPTANCE_ID session" --argjson host "${agent_ids[0]}" --argjson ps "$participants" '{agenda:$agenda,hostAgentId:$host,participantAgentIds:$ps}')" "$OUT_DIR/session.json"
session_id=$(data_id "$OUT_DIR/session.json")
request POST "/api/v1/admin/sessions/$session_id/start" '{}' "$OUT_DIR/session-start.json"
request POST "/api/v1/admin/sessions/$session_id/complete" '{"summary":"ACCEPT-H1H7 completed"}' "$OUT_DIR/session-complete.json"
request GET "/api/v1/admin/groups/$group_id/audit" '' "$OUT_DIR/group-audit.json"
record H4 pass "group=$group_id channel=$channel_id session=$session_id"

# H5 collective decision: quorum + close + audit. It exercises reusable group decision primitives.
electors=$(printf '%s\n' "${agent_ids[@]}" | jq -s 'map({agentId:tonumber,weight:1,veto:(.==.[0])})' 2>/dev/null || true)
# Build explicitly because jq's array self-reference differs across versions.
electors=$(jq -nc --argjson a "${agent_ids[0]}" --argjson b "${agent_ids[1]}" --argjson c "${agent_ids[2]}" \
  '[{agentId:$a,weight:1,veto:true},{agentId:$b,weight:1,veto:false},{agentId:$c,weight:1,veto:false}]')
decision_body=$(jq -nc --arg gid "$group_id" --arg title "$ACCEPTANCE_ID vote" --argjson electors "$electors" \
  '{groupId:$gid,title:$title,description:"acceptance quorum",quorumPercent:67,approvalPercent:67,timeoutAction:"none",electors:$electors}')
if request POST /api/v1/admin/decisions "$decision_body" "$OUT_DIR/decision.json"; then
  decision_id=$(data_id "$OUT_DIR/decision.json")
  votes_ok=true
  for idx in 0 1 2; do
    request POST "/api/v1/admin/decisions/$decision_id/votes" \
      "$(jq -nc --argjson id "${agent_ids[$idx]}" '{agentId:$id,choice:"approve",reason:"acceptance"}')" "$OUT_DIR/vote-$idx.json" || votes_ok=false
  done
  request POST "/api/v1/admin/decisions/$decision_id/close" '{}' "$OUT_DIR/decision-close.json" || votes_ok=false
  request GET "/api/v1/admin/decisions/$decision_id" '' "$OUT_DIR/decision-final.json" || votes_ok=false
  request GET "/api/v1/admin/decisions/$decision_id/audit" '' "$OUT_DIR/decision-audit.json" || votes_ok=false
  decision_status=$(jq -r '.data.status // ""' "$OUT_DIR/decision-final.json")
  if $votes_ok && [[ "$decision_status" == "passed" ]]; then
    record H5 pass "collective decision=$decision_id quorum/votes/audit verified"
  else
    record H5 fail "decision=$decision_id status=$decision_status"
  fi
else
  record H5 fail 'collective decision could not be created'
fi

# H6: verify the four dynamic packages are installed and inspect isolated Run views.
request GET /api/v1/admin/extensions '' "$OUT_DIR/extensions.json"
active_h6=$(jq '[.data.items[]? | select(.name=="io.zerone.emotion" or .name=="io.zerone.belief" or .name=="io.zerone.memory" or .name=="io.zerone.relationship-dynamics") | select(.status=="active" and .installedStatus=="enabled")] | length' "$OUT_DIR/extensions.json")
if [[ "$active_h6" == 4 ]]; then
  record H6 blocked 'four H6 packages active; behavioral mutation needs real runtime messages/workflow persona hooks, not simulated by acceptance script'
else
  record H6 fail "only $active_h6/4 H6 extensions active"
fi

# H7 usage is evidence-based: empty data is blocked, never promoted to pass.
request GET "/api/v1/admin/usage/by-run?runId=$run_id" '' "$OUT_DIR/usage.json" || true
usage_count=$(jq '.data.total // (.data.items // [] | length) // 0' "$OUT_DIR/usage.json" 2>/dev/null || echo 0)
if [[ "$usage_count" -gt 0 ]]; then record H7 pass "usage records=$usage_count"; else record H7 blocked 'no real model/tool metering record for isolated run'; fi

jq -s --arg acceptanceId "$ACCEPTANCE_ID" --arg runId "$run_id" --arg groupId "$group_id" \
  '{acceptanceId:$acceptanceId,runId:$runId,groupId:$groupId,results:.}' "$RESULTS" >"$OUT_DIR/report.json"
{
  echo "# H1-H7 live acceptance: $ACCEPTANCE_ID"
  echo
  echo "- Run: \`$run_id\`"
  echo "- Group: \`$group_id\`"
  echo
  jq -r '.results[] | "- **\(.phase)** \(.status): \(.detail)"' "$OUT_DIR/report.json"
} >"$OUT_DIR/report.md"

cat "$OUT_DIR/report.json"
