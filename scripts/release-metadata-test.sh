#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
temporary_directory=$(mktemp -d)
trap 'rm -rf "$temporary_directory"' EXIT INT TERM

stable_output="$temporary_directory/stable"
"$project_root/scripts/release-metadata.sh" v1.2.3 example/video-record "$stable_output"
expected_stable='version=1.2.3
stable=true
full_tag=example/video-record:1.2.3
major_minor_tag=example/video-record:1.2
latest_tag=example/video-record:latest'
[ "$(cat "$stable_output")" = "$expected_stable" ] || {
  echo "release metadata: stable tags differ" >&2
  exit 1
}

prerelease_output="$temporary_directory/prerelease"
"$project_root/scripts/release-metadata.sh" v2.0.0-rc.1 example/video-record "$prerelease_output"
expected_prerelease='version=2.0.0-rc.1
stable=false
full_tag=example/video-record:2.0.0-rc.1
major_minor_tag=
latest_tag='
[ "$(cat "$prerelease_output")" = "$expected_prerelease" ] || {
  echo "release metadata: prerelease tags differ" >&2
  exit 1
}

alphanumeric_output="$temporary_directory/alphanumeric"
"$project_root/scripts/release-metadata.sh" v2.0.0-01abc example/video-record "$alphanumeric_output"
grep -q '^full_tag=example/video-record:2.0.0-01abc$' "$alphanumeric_output" || {
  echo "release metadata: rejected a valid alphanumeric prerelease" >&2
  exit 1
}

newline_repository=$(printf 'example/video-record\ninjected=value')
if "$project_root/scripts/release-metadata.sh" v1.2.3 "$newline_repository" \
  "$temporary_directory/newline" >/dev/null 2>&1; then
  echo "release metadata: accepted a repository containing a newline" >&2
  exit 1
fi

for invalid in 'v1.2' 'v01.2.3' 'v1.2.3+build' 'v1.2.3-rc.' '1.2.3' 'v1.2.3|Example/Repo'; do
  tag=${invalid%%|*}
  repository=${invalid#*|}
  if [ "$repository" = "$invalid" ]; then
    repository=example/video-record
  fi
  if "$project_root/scripts/release-metadata.sh" "$tag" "$repository" "$temporary_directory/invalid" >/dev/null 2>&1; then
    echo "release metadata: accepted invalid input: $invalid" >&2
    exit 1
  fi
done

echo "release metadata tests: passed"
