#!/bin/sh
set -eu

fail() {
  echo "container smoke: $*" >&2
  exit 1
}

assert_image_user() {
  [ "$1" = "65532:65532" ] || fail "image must declare user 65532:65532"
}

assert_history_has_no_credentials() {
  history_value=$1
  runtime_key=$2

  if [ -n "$runtime_key" ] && printf '%s' "$history_value" | grep -F "$runtime_key" >/dev/null; then
    fail "runtime key appeared in image history"
  fi
  if printf '%s' "$history_value" | grep -Eiq \
    '(^|[^[:alnum:]_])([[:alnum:]_.-]*(token|secret|password|passwd|credential|api[_-]?key|access[_-]?key|private[_-]?key|encryption[_-]?key)[[:alnum:]_.-]*)[[:space:]]*[:=][[:space:]]*[^[:space:]]{8,}|bearer[[:space:]]+[A-Za-z0-9._~+/=-]{8,}|eyJ[A-Za-z0-9_-]{2,}\.[A-Za-z0-9_-]{2,}\.[A-Za-z0-9_-]{2,}'; then
    fail "credential-like value appeared in image history"
  fi
}

if [ "${CONTAINER_SMOKE_LIBRARY_ONLY:-0}" = "1" ]; then
  return 0 2>/dev/null || exit 0
fi

for command in curl docker node openssl; do
  command -v "$command" >/dev/null 2>&1 || fail "missing required command: $command"
done

image=${1:-}
[ -n "$image" ] || fail "usage: $0 IMAGE"
docker image inspect "$image" >/dev/null 2>&1 || fail "image not found: $image"

temporary_directory=$(mktemp -d)
container_name="video-record-smoke-$$"
volume_name="video-record-smoke-$$"
cookie_jar="$temporary_directory/cookies.txt"
backup_path="$temporary_directory/rehearsal.vrbackup"

cleanup() {
  docker rm -f "$container_name" >/dev/null 2>&1 || true
  docker volume rm "$volume_name" >/dev/null 2>&1 || true
  rm -rf "$temporary_directory"
}
trap cleanup EXIT INT TERM

image_user=$(docker image inspect --format '{{.Config.User}}' "$image")
assert_image_user "$image_user"

exposed_ports=$(docker image inspect --format '{{json .Config.ExposedPorts}}' "$image")
printf '%s' "$exposed_ports" | node -e '
let input = "";
process.stdin.on("data", (chunk) => { input += chunk });
process.stdin.on("end", () => {
  const ports = JSON.parse(input || "null");
  if (!ports || !("8080/tcp" in ports) || Object.keys(ports).length !== 1) process.exit(1);
});
' || fail "image must expose only 8080/tcp"

healthcheck=$(docker image inspect --format '{{json .Config.Healthcheck.Test}}' "$image")
printf '%s' "$healthcheck" | node -e '
let input = "";
process.stdin.on("data", (chunk) => { input += chunk });
process.stdin.on("end", () => {
  const test = JSON.parse(input || "null");
  if (JSON.stringify(test) !== JSON.stringify(["CMD", "/video-record", "healthcheck"])) process.exit(1);
});
' || fail "image healthcheck must use /video-record healthcheck"

encryption_key=$(openssl rand -base64 32 | tr -d '\n')
admin_password=$(printf '%s-%s-%s' Synthetic container password)

start_container() {
  docker run -d \
    --name "$container_name" \
    --read-only \
    --user "$image_user" \
    --cap-drop ALL \
    --security-opt no-new-privileges:true \
    -p 127.0.0.1::8080 \
    -v "$volume_name:/data" \
    -e APP_ENV=production \
    -e APP_PORT=8080 \
    -e APP_COOKIE_SECURE=false \
    -e DATA_DIR=/data \
    -e APP_ENCRYPTION_KEY="$encryption_key" \
    "$image" >/dev/null
}

wait_for_ready() {
  attempts=0
  until docker exec "$container_name" /video-record healthcheck >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    [ "$attempts" -lt 30 ] || {
      docker logs "$container_name" >&2 || true
      fail "container did not become ready"
    }
    sleep 1
  done
}

container_port() {
  docker port "$container_name" 8080/tcp | sed -n '1s/.*://p'
}

json_field() {
  field=$1
  node -e '
let input = "";
process.stdin.on("data", (chunk) => { input += chunk });
process.stdin.on("end", () => {
  const value = JSON.parse(input)[process.argv[1]];
  if (typeof value !== "string" || value.length === 0) process.exit(1);
  process.stdout.write(value);
});
' "$field"
}

assert_json_value() {
  field=$1
  expected=$2
  node -e '
let input = "";
process.stdin.on("data", (chunk) => { input += chunk });
process.stdin.on("end", () => {
  const value = JSON.parse(input)[process.argv[1]];
  if (String(value) !== process.argv[2]) process.exit(1);
});
' "$field" "$expected" || fail "unexpected JSON value for $field"
}

assert_container_security() {
  [ "$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$container_name")" = "true" ] ||
    fail "root filesystem is not read-only"
  [ "$(docker inspect --format '{{.Config.User}}' "$container_name")" = "$image_user" ] ||
    fail "container user differs from image non-root user"
  mounts=$(docker inspect --format '{{json .Mounts}}' "$container_name")
  printf '%s' "$mounts" | node -e '
let input = "";
process.stdin.on("data", (chunk) => { input += chunk });
process.stdin.on("end", () => {
  const mounts = JSON.parse(input);
  if (mounts.length !== 1 || mounts[0].Destination !== "/data" || mounts[0].RW !== true) process.exit(1);
});
' || fail "/data must be the only writable mount"
  cap_drop=$(docker inspect --format '{{json .HostConfig.CapDrop}}' "$container_name")
  [ "$cap_drop" = '["ALL"]' ] || fail "container must drop all capabilities"
  security_options=$(docker inspect --format '{{json .HostConfig.SecurityOpt}}' "$container_name")
  printf '%s' "$security_options" | grep -q 'no-new-privileges' || fail "no-new-privileges is missing"
}

start_container
wait_for_ready
assert_container_security
port=$(container_port)
[ -n "$port" ] || fail "container port 8080 is not published"
base_url="http://127.0.0.1:$port"
origin="$base_url"

frontend_html=$(curl --fail-with-body --silent --show-error "$base_url/")
printf '%s' "$frontend_html" | grep -q '<script type="module"' || fail "production frontend is not embedded"
frontend_asset=$(printf '%s' "$frontend_html" | node -e '
let input = "";
process.stdin.on("data", (chunk) => { input += chunk });
process.stdin.on("end", () => {
  const match = input.match(/<script type="module"[^>]+src="([^"]+\.js)"/);
  if (!match) process.exit(1);
  process.stdout.write(match[1]);
});
') || fail "embedded frontend has no module asset"
curl --fail-with-body --silent --show-error "$base_url$frontend_asset" >/dev/null

setup_status=$(curl --fail-with-body --silent --show-error "$base_url/api/v1/setup/status")
printf '%s' "$setup_status" | assert_json_value initialized false

setup_payload=$(node -e 'process.stdout.write(JSON.stringify({username: process.argv[1], password: process.argv[2]}))' \
  container-admin "$admin_password")
curl --fail-with-body --silent --show-error \
  -c "$cookie_jar" \
  -H "Origin: $origin" \
  -H 'Content-Type: application/json' \
  --data "$setup_payload" \
  "$base_url/api/v1/setup/admin" >/dev/null
login_response=$(curl --fail-with-body --silent --show-error \
  -c "$cookie_jar" \
  -H "Origin: $origin" \
  -H 'Content-Type: application/json' \
  --data "$setup_payload" \
  "$base_url/api/v1/auth/login")
csrf_token=$(printf '%s' "$login_response" | json_field csrfToken) || fail "login did not return a CSRF token"

media_response=$(curl --fail-with-body --silent --show-error \
  -b "$cookie_jar" -c "$cookie_jar" \
  -H "Origin: $origin" \
  -H "X-CSRF-Token: $csrf_token" \
  -H "Idempotency-Key: container-media-$$" \
  -H 'Content-Type: application/json' \
  --data '{"mediaType":"movie","title":"Synthetic Container Movie","year":"2026"}' \
  "$base_url/api/v1/media/custom")
media_id=$(printf '%s' "$media_response" | json_field id) || fail "custom media did not return an ID"

record_response=$(curl --fail-with-body --silent --show-error \
  -X PUT \
  -b "$cookie_jar" -c "$cookie_jar" \
  -H "Origin: $origin" \
  -H "X-CSRF-Token: $csrf_token" \
  -H "Idempotency-Key: container-record-$$" \
  -H 'If-Match: "0"' \
  -H 'Content-Type: application/json' \
  --data '{"status":"wishlist"}' \
  "$base_url/api/v1/records/$media_id")
printf '%s' "$record_response" | assert_json_value status wishlist

docker rm -f "$container_name" >/dev/null
start_container
wait_for_ready
assert_container_security
port=$(container_port)
base_url="http://127.0.0.1:$port"
origin="$base_url"

persisted_record=$(curl --fail-with-body --silent --show-error \
  -b "$cookie_jar" "$base_url/api/v1/records/$media_id")
printf '%s' "$persisted_record" | assert_json_value status wishlist
curl --fail-with-body --silent --show-error \
  -b "$cookie_jar" \
  "$base_url/api/v1/calendar?month=2026-07&timezone=Asia%2FShanghai&filter=all" >/dev/null

backup_response=$(curl --fail-with-body --silent --show-error \
  -X POST \
  -b "$cookie_jar" -c "$cookie_jar" \
  -H "Origin: $origin" \
  -H "X-CSRF-Token: $csrf_token" \
  -H "Idempotency-Key: container-backup-$$" \
  -H 'Content-Type: application/json' \
  --data '{}' \
  "$base_url/api/v1/backups")
backup_filename=$(printf '%s' "$backup_response" | json_field filename) || fail "backup did not return a filename"
curl --fail-with-body --silent --show-error \
  -b "$cookie_jar" \
  -o "$backup_path" \
  "$base_url/api/v1/backups/$backup_filename"
[ -s "$backup_path" ] || fail "downloaded backup is empty"

changed_record=$(curl --fail-with-body --silent --show-error \
  -X PUT \
  -b "$cookie_jar" -c "$cookie_jar" \
  -H "Origin: $origin" \
  -H "X-CSRF-Token: $csrf_token" \
  -H "Idempotency-Key: container-change-$$" \
  -H 'If-Match: "1"' \
  -H 'Content-Type: application/json' \
  --data '{"status":"dropped"}' \
  "$base_url/api/v1/records/$media_id")
printf '%s' "$changed_record" | assert_json_value status dropped

curl --fail-with-body --silent --show-error \
  -X POST \
  -b "$cookie_jar" -c "$cookie_jar" \
  -H "Origin: $origin" \
  -H "X-CSRF-Token: $csrf_token" \
  -H "Idempotency-Key: container-restore-$$" \
  -F "file=@$backup_path;type=application/vnd.video-record.backup" \
  "$base_url/api/v1/restore" >/dev/null

restored_record=$(curl --fail-with-body --silent --show-error \
  -b "$cookie_jar" "$base_url/api/v1/records/$media_id")
printf '%s' "$restored_record" | assert_json_value status wishlist

history=$(docker history --no-trunc --format '{{.CreatedBy}}' "$image")
assert_history_has_no_credentials "$history" "$encryption_key"

echo "container smoke: passed for $image"
