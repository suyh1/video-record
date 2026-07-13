#!/bin/sh
set -eu

export VIDEO_RECORD_RECOVERY_SMOKE=1
go test -p 1 ./internal/storage ./internal/testutil \
  -run '^(TestMigrationRecoverySmoke|TestRecoverySmoke)$' -count=1 -v
