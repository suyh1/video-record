#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
CONTAINER_SMOKE_LIBRARY_ONLY=1
export CONTAINER_SMOKE_LIBRARY_ONLY
. "$project_root/scripts/container-smoke.sh"

expect_user_rejected() {
  candidate=$1
  if (assert_image_user "$candidate") >/dev/null 2>&1; then
    echo "container smoke policy: accepted invalid image user: $candidate" >&2
    exit 1
  fi
}

expect_history_rejected() {
  candidate=$1
  if (assert_history_has_no_credentials "$candidate" synthetic-runtime-key) >/dev/null 2>&1; then
    echo "container smoke policy: accepted credential-like history" >&2
    exit 1
  fi
}

(assert_image_user "65532:65532")
expect_user_rejected ""
expect_user_rejected "root"
expect_user_rejected "0"
expect_user_rejected "root:65532"
expect_user_rejected "0:65532"
expect_user_rejected "65532:0"

(assert_history_has_no_credentials "ENV APP_ENV=production APP_PORT=8080 DATA_DIR=/data" synthetic-runtime-key)
expect_history_rejected "ENV MEDIA_SERVER_TOKEN=synthetic-value"
expect_history_rejected "ARG SERVICE_CREDENTIAL=synthetic-value"
expect_history_rejected "Authorization: Bearer synthetic-value"
expect_history_rejected "RUN token=synthetic-value"
jwt_value="eyJ""synthetic.header.payload"
expect_history_rejected "RUN printf %s $jwt_value"
expect_history_rejected "RUN printf %s synthetic-runtime-key"

echo "container smoke policy: passed"
