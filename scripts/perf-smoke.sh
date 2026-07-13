#!/bin/sh
set -eu

export VIDEO_RECORD_PERF_SMOKE=1
go test ./internal/testutil -run '^TestPerformanceSmoke$' -count=1 -v
