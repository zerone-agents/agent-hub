#!/usr/bin/env bash
set -euo pipefail
: "${HUB_TOKEN:?HUB_TOKEN required}"
HUB_URL="${HUB_URL:-http://127.0.0.1}"
PREFIX="${PREFIX:-accept-h1h7-20260920t040900z}"
RUN_ID="${RUN_ID:?RUN_ID required}"
H="Authorization: Bearer $HUB_TOKEN"

base=$(curl -fsS -H "$H" "$HUB_URL/api/v1/agents/speeding-director" |
  jq '.data.config | del(.fieldOverrides,.behaviorProfile,.personalityPrompt,.personalityTemplateName,.personalityTemplateVersion)')

for suffix in a b c; do
  case "$suffix" in
    a) prompt='你是验收代理A。收到用户要求后，只能通过组织MCP的 agent_send 把任务 assign 给验收代理B；收到B的汇总后回复用户。不得直连C。' ;;
    b) prompt='你是验收代理B。收到A的任务后，必须通过 agent_send 向验收代理C发起 consult，取得C回复后再汇总回复A。' ;;
    c) prompt='你是验收代理C。收到B咨询后，返回明确结论：链路验收通过，校验码H1H7-C。' ;;
  esac
  name="$PREFIX-$suffix"
  body=$(jq -nc --argjson cfg "$base" --arg p "$prompt" '{config:($cfg + {systemPrompt:$p})}')
  curl -fsS -X PUT -H "$H" -H 'Content-Type: application/json' --data "$body" \
    "$HUB_URL/api/v1/admin/agents/$name" >"/tmp/$name-update.json"
  curl -fsS -X POST -H "$H" "$HUB_URL/api/v1/admin/agents/$name/deploy" >"/tmp/$name-deploy.json"
done

for _ in $(seq 1 60); do
  ready=0
  for suffix in a b c; do
    name="$PREFIX-$suffix"
    status=$(curl -fsS -H "$H" "$HUB_URL/api/v1/admin/agents/$name/deploy" | jq -r '.data.status // empty')
    [[ "$status" == running ]] && ready=$((ready+1))
  done
  [[ "$ready" == 3 ]] && break
  sleep 2
done
[[ "$ready" == 3 ]] || { echo 'runtime deployment timeout' >&2; exit 2; }

# Create only A->B and B->C. Existing production relations are never modified.
agents=$(curl -fsS -H "$H" "$HUB_URL/api/v1/admin/agents")
aid=$(jq -er --arg n "$PREFIX-a" '.data.agents[] | select(.name==$n) | .id' <<<"$agents")
bid=$(jq -er --arg n "$PREFIX-b" '.data.agents[] | select(.name==$n) | .id' <<<"$agents")
cid=$(jq -er --arg n "$PREFIX-c" '.data.agents[] | select(.name==$n) | .id' <<<"$agents")
for edge in "$aid:$bid:assign" "$bid:$cid:consult"; do
  IFS=: read -r source target action <<<"$edge"
  relation_type=$([[ "$action" == assign ]] && echo reports_to || echo advisor)
  body=$(jq -nc --argjson s "$source" --argjson t "$target" --arg a "$action" --arg rt "$relation_type" \
    '{sourceAgentId:$s,targetAgentId:$t,scope:"global",relationType:$rt,stance:"neutral",allowedActions:[$a,"inform","report"],contextPolicy:"summary_only",deliveryPolicy:"async",constraint:"Acceptance only",enabled:true,bidirectional:false}')
  code=$(curl -sS -o "/tmp/$PREFIX-relation-$source-$target.json" -w '%{http_code}' -X POST -H "$H" \
    -H 'Content-Type: application/json' --data "$body" "$HUB_URL/api/v1/admin/agent-relations")
  [[ "$code" == 201 || "$code" == 409 ]] || { cat "/tmp/$PREFIX-relation-$source-$target.json" >&2; exit 3; }
done

# Relations are a deploy-time system capability. Force a fresh artifact after
# the edges exist so organization MCP is mounted into all three runtimes.
for suffix in a b c; do
  name="$PREFIX-$suffix"
  curl -fsS -X POST -H "$H" "$HUB_URL/api/v1/admin/agents/$name/deploy?force=true" >/dev/null
done
sleep 3

transition_code=$(curl -sS -o "/tmp/$PREFIX-transition.json" -w '%{http_code}' -X POST -H "$H" \
  -H 'Content-Type: application/json' --data '{"status":"running"}' "$HUB_URL/api/v1/admin/runs/$RUN_ID/transitions")
[[ "$transition_code" == 200 || "$transition_code" == 422 ]] || { cat "/tmp/$PREFIX-transition.json" >&2; exit 4; }

session=$(curl -fsS -X POST -H "$H" -H 'Content-Type: application/json' --data '{"title":"ACCEPT-H1H7 A-B-C"}' \
  "$HUB_URL/api/v1/agents/$PREFIX-a/chat/sessions" | jq -er '.data.id')
curl -fsS -N -X POST -H "$H" -H 'Content-Type: application/json' \
  --data "$(jq -nc --arg runId "$RUN_ID" '{content:"请严格执行 A→B→C 验收：把任务委派给B，让B咨询C取得校验码，再由B汇总给你。不要直接联系C。",runId:$runId}')" \
  "$HUB_URL/api/v1/agents/$PREFIX-a/chat/sessions/$session/messages" >"/tmp/$PREFIX-sse.txt"

jq -nc --arg prefix "$PREFIX" --arg session "$session" '{prefix:$prefix,sessionId:$session,status:"submitted"}'
