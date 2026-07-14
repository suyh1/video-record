#!/bin/sh
set -eu

fail() {
  echo "coverage gate: $*" >&2
  exit 1
}

for command in awk go mktemp tr; do
  command -v "$command" >/dev/null 2>&1 || fail "missing required command: $command"
done

coverage_packages=${COVERAGE_PACKAGES:-'./internal/records ./internal/media ./internal/stats ./internal/household ./internal/storage ./internal/integrations ./internal/sync'}
threshold=85.0
temporary_directory=$(mktemp -d)
trap 'rm -rf "$temporary_directory"' EXIT INT TERM

for package in $coverage_packages; do
  profile_name=$(printf '%s' "$package" | tr '/.' '__')
  profile="$temporary_directory/$profile_name.out"
  go test "$package" -covermode=atomic -coverprofile="$profile" -count=1
  coverage=$(awk 'NR > 1 { total += $2; if ($3 > 0) covered += $2 }
    END { if (total == 0) exit 1; printf "%.10f", covered * 100 / total }' "$profile")
  [ -n "$coverage" ] || fail "missing total coverage for $package"
  awk -v coverage="$coverage" -v threshold="$threshold" \
    'BEGIN { exit !(coverage + 0 >= threshold + 0) }' ||
    fail "$package coverage $coverage% is below $threshold%"
  echo "coverage gate: $package $coverage%"
done
