#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
set +x

# One-shot H0 deployment for a cloud-provider command assistant. It updates
# only the already-running Agent Hub Compose service and never needs SSH.
# Optional selectors (values are never printed):
#   H0_HUB_CONTAINER  Compose-managed Hub container name/ID when auto-detection
#                     finds zero or multiple candidates.
#   H0_HEALTH_URL     Existing externally reachable Hub /health URL when port
#                     8081 is not published and the bridge IP is unreachable.
readonly REPOSITORY_URL="https://github.com/zerone-agents/agent-hub.git"
readonly RELEASE_BRANCH="codex/agent-relations"
readonly RELEASE_COMMIT="0052f8b"
readonly DEPLOY_ROOT="/opt/zerone-agent-hub-h0"
readonly SOURCE_MIRROR="${DEPLOY_ROOT}/repository.git"
readonly IMAGE_TAG="zerone-agent-hub:h0-${RELEASE_COMMIT}"
readonly BACKUP_ROOT="${DEPLOY_ROOT}/backups"

source_dir=""
override_file=""
rollback_file=""
old_image=""
compose_project=""
compose_service=""
compose_workdir=""
container_before=""
deployment_started=0
deployment_verified=0
declare -a compose_cmd=()
declare -a compose_files=()

log() { printf '[h0-deploy] %s\n' "$*"; }
fail() { printf '[h0-deploy] ERROR: %s\n' "$*" >&2; exit 1; }

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

compose() {
  (
    cd "$compose_workdir"
    "${compose_cmd[@]}" --project-name "$compose_project" "${compose_files[@]}" "$@"
  )
}

write_override() {
  local target=$1 image=$2
  [[ "$compose_service" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || fail "unsafe Compose service name"
  [[ "$image" =~ ^[A-Za-z0-9][A-Za-z0-9._/:@-]*$ ]] || fail "unsafe image reference"
  printf 'services:\n  %s:\n    image: "%s"\n' "$compose_service" "$image" >"$target"
  chmod 600 "$target"
}

find_current_container() {
  local id
  local -a candidate_ids=()
  mapfile -t candidate_ids < <(docker ps --filter label=com.docker.compose.project \
    --filter label=com.docker.compose.service --format '{{.ID}}')
  ((${#candidate_ids[@]} > 0)) || fail "no running Docker Compose containers found"

  local -a matches=()
  for id in "${candidate_ids[@]}"; do
    if is_hub_candidate "$id"; then
      matches+=("$id")
    fi
  done

  if ((${#matches[@]} != 1)); then
    fail "expected exactly one running Compose Hub candidate, found ${#matches[@]}; set H0_HUB_CONTAINER to its container name or ID"
  fi
  printf '%s' "${matches[0]}"
}

is_hub_candidate() {
  local id=$1 service image name ports identity
  service=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.service" }}' "$id")
  image=$(docker inspect --format '{{.Config.Image}}' "$id")
  name=$(docker inspect --format '{{.Name}}' "$id")
  ports=$(docker inspect --format '{{json .Config.ExposedPorts}}' "$id")
  identity="${service,,} ${image,,} ${name,,}"
  [[ "$identity" == *agent-hub* || "$identity" == *agenthub* || "$ports" == *'8081/tcp'* ]]
}

resolve_container() {
  if [[ -n "${H0_HUB_CONTAINER:-}" ]]; then
    docker inspect "$H0_HUB_CONTAINER" >/dev/null 2>&1 || fail "H0_HUB_CONTAINER does not exist"
    [[ "$(docker inspect --format '{{.State.Running}}' "$H0_HUB_CONTAINER")" == true ]] || fail "H0_HUB_CONTAINER is not running"
    [[ -n "$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.service" }}' "$H0_HUB_CONTAINER")" ]] || fail "H0_HUB_CONTAINER is not managed by Docker Compose"
    is_hub_candidate "$H0_HUB_CONTAINER" || fail "H0_HUB_CONTAINER does not look like an Agent Hub service"
    docker inspect --format '{{.ID}}' "$H0_HUB_CONTAINER"
  else
    find_current_container
  fi
}

health_url_for_container() {
  local container=$1 published host_port ip
  if [[ -n "${H0_HEALTH_URL:-}" ]]; then
    [[ "$H0_HEALTH_URL" == http://* || "$H0_HEALTH_URL" == https://* ]] || fail "H0_HEALTH_URL must use http or https"
    printf '%s' "$H0_HEALTH_URL"
    return
  fi

  published=$(docker port "$container" 8081/tcp 2>/dev/null | head -n 1 || true)
  if [[ "$published" =~ :([0-9]+)$ ]]; then
    host_port="${BASH_REMATCH[1]}"
    printf 'http://127.0.0.1:%s/health' "$host_port"
    return
  fi

  ip=$(docker inspect --format '{{range .NetworkSettings.Networks}}{{if .IPAddress}}{{.IPAddress}}{{end}}{{end}}' "$container")
  [[ "$ip" =~ ^[0-9a-fA-F:.]+$ ]] || fail "cannot derive Hub health endpoint; set H0_HEALTH_URL"
  if [[ "$ip" == *:* ]]; then
    printf 'http://[%s]:8081/health' "$ip"
  else
    printf 'http://%s:8081/health' "$ip"
  fi
}

wait_for_health() {
  local container=$1 url
  url=$(health_url_for_container "$container")
  log "waiting for Hub health endpoint"
  for _ in {1..60}; do
    if [[ "$(docker inspect --format '{{.State.Running}}' "$container" 2>/dev/null || true)" != true ]]; then
      return 1
    fi
    if curl --fail --silent --max-time 3 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

rollback() {
  ((deployment_started == 1)) || return 0
  ((deployment_verified == 0)) || return 0
  [[ -n "$old_image" && -n "$rollback_file" ]] || return 0

  log "deployment failed; restoring the previous Hub image"
  write_override "$rollback_file" "$old_image"
  if compose -f "$rollback_file" up -d --no-deps "$compose_service" >/dev/null; then
    local restored
    restored=$(docker ps --filter "label=com.docker.compose.project=$compose_project" \
      --filter "label=com.docker.compose.service=$compose_service" --format '{{.ID}}' | head -n 1)
    if [[ -n "$restored" ]] && wait_for_health "$restored"; then
      log "rollback completed and Hub is healthy"
    else
      printf '[h0-deploy] ERROR: rollback started but health verification failed\n' >&2
    fi
  else
    printf '[h0-deploy] ERROR: rollback Compose operation failed\n' >&2
  fi
}

cleanup() {
  local status=$?
  if ((status != 0)); then rollback || true; fi
  if [[ -n "$source_dir" && "$source_dir" == "$DEPLOY_ROOT"/source.* && -d "$source_dir" ]]; then
    rm -rf -- "$source_dir"
  fi
  exit "$status"
}
trap cleanup EXIT

[[ ${EUID:-$(id -u)} -eq 0 ]] || fail "run this script as root through the cloud command assistant"
for command in docker git curl mktemp cp chmod; do require_command "$command"; done

if docker compose version >/dev/null 2>&1; then
  compose_cmd=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  compose_cmd=(docker-compose)
else
  fail "Docker Compose v2 or docker-compose is required"
fi

[[ ! -L "$DEPLOY_ROOT" && ! -L "$BACKUP_ROOT" && ! -L "$SOURCE_MIRROR" ]] || fail "deployment paths must not be symbolic links"
mkdir -p "$DEPLOY_ROOT" "$BACKUP_ROOT"
chmod 700 "$DEPLOY_ROOT" "$BACKUP_ROOT"

container_before=$(resolve_container)
compose_project=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "$container_before")
compose_service=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.service" }}' "$container_before")
compose_workdir=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project.working_dir" }}' "$container_before")
config_label=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project.config_files" }}' "$container_before")
old_image=$(docker inspect --format '{{.Config.Image}}' "$container_before")

[[ "$compose_project" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || fail "missing or unsafe Compose project label"
[[ "$compose_service" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || fail "missing or unsafe Compose service label"
[[ "$compose_workdir" == /* && -d "$compose_workdir" ]] || fail "Compose working directory is unavailable"
[[ -n "$config_label" ]] || fail "Compose config_files label is unavailable"
[[ -n "$old_image" ]] || fail "current Hub image reference is unavailable"

wait_for_health "$container_before" || fail "current Hub is not healthy; refusing an unsafe deployment"

IFS=',' read -r -a raw_config_files <<<"$config_label"
((${#raw_config_files[@]} > 0)) || fail "no Compose configuration files resolved"
for config_file in "${raw_config_files[@]}"; do
  config_file="${config_file#${config_file%%[![:space:]]*}}"
  config_file="${config_file%${config_file##*[![:space:]]}}"
  [[ -n "$config_file" && "$config_file" != *$'\n'* ]] || fail "unsafe Compose config path"
  [[ "$config_file" == /* ]] || config_file="$compose_workdir/$config_file"
  [[ -f "$config_file" && ! -L "$config_file" ]] || fail "Compose config is missing or is a symlink: $config_file"
  compose_files+=(-f "$config_file")
done

timestamp=$(date -u +%Y%m%dT%H%M%SZ)
backup_dir="$BACKUP_ROOT/$timestamp"
mkdir -p "$backup_dir/configs"
chmod 700 "$backup_dir" "$backup_dir/configs"
printf 'project=%s\nservice=%s\nworking_dir=%s\nold_image=%s\ncontainer=%s\n' \
  "$compose_project" "$compose_service" "$compose_workdir" "$old_image" "$container_before" >"$backup_dir/metadata"
chmod 600 "$backup_dir/metadata"
for ((i=0; i<${#compose_files[@]}; i+=2)); do
  config_file=${compose_files[i+1]}
  cp --parents --preserve=mode,timestamps -- "$config_file" "$backup_dir/configs"
done
log "secured Compose backup at $backup_dir"

if [[ ! -d "$SOURCE_MIRROR" ]]; then
  git clone --mirror --filter=blob:none "$REPOSITORY_URL" "$SOURCE_MIRROR" >/dev/null
else
  actual_origin=$(git -C "$SOURCE_MIRROR" remote get-url origin)
  [[ "$actual_origin" == "$REPOSITORY_URL" || "$actual_origin" == "${REPOSITORY_URL%.git}" ]] || fail "existing source mirror has an unexpected origin"
fi
git -C "$SOURCE_MIRROR" fetch --prune origin "+refs/heads/$RELEASE_BRANCH:refs/remotes/origin/$RELEASE_BRANCH" >/dev/null

git -C "$SOURCE_MIRROR" cat-file -e "${RELEASE_COMMIT}^{commit}" 2>/dev/null || fail "release commit was not fetched"
branch_tip=$(git -C "$SOURCE_MIRROR" rev-parse "refs/remotes/origin/$RELEASE_BRANCH")
git -C "$SOURCE_MIRROR" merge-base --is-ancestor "$RELEASE_COMMIT" "$branch_tip" || fail "release commit is not on $RELEASE_BRANCH"
resolved_commit=$(git -C "$SOURCE_MIRROR" rev-parse --short=7 "$RELEASE_COMMIT")
[[ "$resolved_commit" == "$RELEASE_COMMIT" ]] || fail "release commit did not resolve exactly"

source_dir=$(mktemp -d "$DEPLOY_ROOT/source.XXXXXX")
git clone --quiet "$SOURCE_MIRROR" "$source_dir"
git -C "$source_dir" checkout --quiet --detach "$RELEASE_COMMIT"
[[ "$(git -C "$source_dir" rev-parse --short=7 HEAD)" == "$RELEASE_COMMIT" ]] || fail "source checkout verification failed"

log "building pinned H0 image"
docker build --pull -t "$IMAGE_TAG" "$source_dir"

override_file="$backup_dir/h0-image.override.yml"
rollback_file="$backup_dir/rollback-image.override.yml"
write_override "$override_file" "$IMAGE_TAG"

log "updating only Compose service $compose_service in project $compose_project"
deployment_started=1
compose -f "$override_file" up -d --no-deps "$compose_service"

container_after=$(docker ps --filter "label=com.docker.compose.project=$compose_project" \
  --filter "label=com.docker.compose.service=$compose_service" --format '{{.ID}}' | head -n 1)
[[ -n "$container_after" ]] || fail "updated Hub container is not running"
running_image=$(docker inspect --format '{{.Config.Image}}' "$container_after")
[[ "$running_image" == "$IMAGE_TAG" ]] || fail "updated Hub is not using the expected H0 image"
wait_for_health "$container_after" || fail "updated Hub failed its health check"

deployment_verified=1
log "deployment complete: H0 commit $RELEASE_COMMIT is healthy"
log "rollback metadata is stored under $backup_dir"
