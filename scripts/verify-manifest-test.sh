#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
temporary_directory=$(mktemp -d)
trap 'rm -rf "$temporary_directory"' EXIT INT TERM

cat >"$temporary_directory/docker" <<'EOF'
#!/bin/sh
set -eu

[ "$1" = "buildx" ]
[ "$2" = "imagetools" ]
[ "$3" = "inspect" ]
[ "$4" = "--raw" ]

case "$5" in
  test/both)
    printf '%s\n' '{"manifests":[{"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","platform":{"os":"linux","architecture":"arm64"}}]}'
    ;;
  test/single)
    printf '%s\n' '{"manifests":[{"platform":{"os":"linux","architecture":"amd64"}}]}'
    ;;
  test/wrong-os)
    printf '%s\n' '{"manifests":[{"platform":{"os":"linux","architecture":"amd64"}},{"platform":{"os":"darwin","architecture":"arm64"}}]}'
    ;;
  test/shared-digest)
    printf '%s\n' '{"manifests":[{"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","platform":{"os":"linux","architecture":"arm64"}}]}'
    ;;
  test/invalid)
    printf '%s\n' 'not-json'
    ;;
  *)
    exit 64
    ;;
esac
EOF
chmod +x "$temporary_directory/docker"

PATH="$temporary_directory:$PATH" "$project_root/scripts/verify-manifest.sh" test/both

for invalid_reference in test/single test/wrong-os test/shared-digest test/invalid; do
  if PATH="$temporary_directory:$PATH" "$project_root/scripts/verify-manifest.sh" "$invalid_reference" >/dev/null 2>&1; then
    echo "manifest verifier accepted invalid reference: $invalid_reference" >&2
    exit 1
  fi
done

echo "manifest verifier tests: passed"
