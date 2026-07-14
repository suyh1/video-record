#!/bin/sh
set -eu

fail() {
  echo "release metadata: $*" >&2
  exit 1
}

tag=${1:-}
image_repository=${2:-}
output_file=${3:-}
[ -n "$tag" ] && [ -n "$image_repository" ] && [ -n "$output_file" ] ||
  fail "usage: $0 TAG IMAGE_REPOSITORY OUTPUT_FILE"
LC_ALL=C
export LC_ALL
case "$tag" in
  *[![:print:]]*) fail "release tag contains control or non-ASCII characters" ;;
esac
case "$image_repository" in
  *[![:print:]]*) fail "IMAGE_REPOSITORY contains control or non-ASCII characters" ;;
esac

printf '%s' "$image_repository" |
  grep -Eq '^[a-z0-9]+([._-][a-z0-9]+)*/[a-z0-9]+([._-][a-z0-9]+)*$' ||
  fail "IMAGE_REPOSITORY must be a lower-case Docker Hub namespace/repository"
printf '%s' "$tag" |
  grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$' ||
  fail "release tag is not valid SemVer"

version=${tag#v}
case "$version" in
  *-*)
    prerelease=${version#*-}
    case "$prerelease" in
      .*|*.|*..*) fail "release tag has an empty prerelease identifier" ;;
    esac
    old_ifs=$IFS
    IFS=.
    set -- $prerelease
    IFS=$old_ifs
    [ "$#" -gt 0 ] || fail "release tag has an empty prerelease"
    for identifier in "$@"; do
      [ -n "$identifier" ] || fail "release tag has an empty prerelease identifier"
      printf '%s' "$identifier" | grep -Eq '^[0-9A-Za-z-]+$' ||
        fail "release tag has an invalid prerelease identifier"
      case "$identifier" in
        0) ;;
        0*)
          if printf '%s' "$identifier" | grep -Eq '^[0-9]+$'; then
            fail "numeric prerelease identifiers cannot have leading zeroes"
          fi
          ;;
      esac
    done
    stable=false
    ;;
  *)
    stable=true
    ;;
esac

core_version=${version%%-*}
major_minor=${core_version%.*}
{
  printf 'version=%s\n' "$version"
  printf 'stable=%s\n' "$stable"
  printf 'full_tag=%s:%s\n' "$image_repository" "$version"
  if [ "$stable" = "true" ]; then
    printf 'major_minor_tag=%s:%s\n' "$image_repository" "$major_minor"
    printf 'latest_tag=%s:latest\n' "$image_repository"
  else
    printf 'major_minor_tag=\n'
    printf 'latest_tag=\n'
  fi
} >>"$output_file"
