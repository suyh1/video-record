#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
temporary_directory=$(mktemp -d)
trap 'rm -rf "$temporary_directory"' EXIT INT TERM

cat >"$temporary_directory/go" <<'EOF'
#!/bin/sh
set -eu
case "$1" in
  test)
    profile=
    for argument in "$@"; do
      case "$argument" in
        -coverprofile=*) profile=${argument#-coverprofile=} ;;
      esac
    done
    [ -n "$profile" ]
    case "$profile" in
      *below.out) covered=849; uncovered=151 ;;
      *rounded.out) covered=1699; uncovered=301 ;;
      *) covered=85; uncovered=15 ;;
    esac
    printf 'mode: set\nfixture.go:1.1,1.2 %s 1\nfixture.go:2.1,2.2 %s 0\n' \
      "$covered" "$uncovered" >"$profile"
    ;;
  tool)
    [ "$2" = "cover" ]
    profile=${3#-func=}
    case "$profile" in
      *below.out) coverage=84.9 ;;
      *rounded.out) coverage=85.0 ;;
      *) coverage=85.0 ;;
    esac
    printf 'total:\t(statements)\t%s%%\n' "$coverage"
    ;;
  *)
    exit 64
    ;;
esac
EOF
chmod +x "$temporary_directory/go"

PATH="$temporary_directory:$PATH" COVERAGE_PACKAGES='./internal/exact' \
  "$project_root/scripts/coverage-gate.sh"
if PATH="$temporary_directory:$PATH" COVERAGE_PACKAGES='./internal/below' \
  "$project_root/scripts/coverage-gate.sh" >/dev/null 2>&1; then
  echo "coverage gate accepted a package below 85 percent" >&2
  exit 1
fi
if PATH="$temporary_directory:$PATH" COVERAGE_PACKAGES='./internal/rounded' \
  "$project_root/scripts/coverage-gate.sh" >/dev/null 2>&1; then
  echo "coverage gate accepted 84.95 percent after display rounding" >&2
  exit 1
fi

echo "coverage gate tests: passed"
