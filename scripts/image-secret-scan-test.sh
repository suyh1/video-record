#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
temporary_directory=$(mktemp -d)
trap 'rm -rf "$temporary_directory"' EXIT INT TERM

layer_root="$temporary_directory/layer-root"
mkdir "$layer_root"
printf '%s\n' 'synthetic image content' >"$layer_root/content.txt"

legacy_root="$temporary_directory/legacy-root"
mkdir -p "$legacy_root/layer-one"
tar -cf "$legacy_root/layer-one/layer.tar" -C "$layer_root" .
printf '%s\n' '{"architecture":"amd64","os":"linux"}' >"$legacy_root/config.json"
printf '%s\n' '[{"Config":"config.json","RepoTags":["test/legacy:latest"],"Layers":["layer-one/layer.tar"]}]' >"$legacy_root/manifest.json"
legacy_archive="$temporary_directory/legacy.tar"
tar -cf "$legacy_archive" -C "$legacy_root" .

oci_root="$temporary_directory/oci-root"
manifest_digest=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
layer_digest=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
mkdir -p "$oci_root/blobs/sha256"
tar -cf "$oci_root/blobs/sha256/$layer_digest" -C "$layer_root" .
printf '%s\n' "{\"schemaVersion\":2,\"layers\":[{\"digest\":\"sha256:$layer_digest\"}]}" >"$oci_root/blobs/sha256/$manifest_digest"
printf '%s\n' "{\"schemaVersion\":2,\"manifests\":[{\"digest\":\"sha256:$manifest_digest\"}]}" >"$oci_root/index.json"
printf '%s\n' '{"imageLayoutVersion":"1.0.0"}' >"$oci_root/oci-layout"
oci_archive="$temporary_directory/oci.tar"
tar -cf "$oci_archive" -C "$oci_root" .

fake_bin="$temporary_directory/bin"
mkdir "$fake_bin"
cat >"$fake_bin/docker" <<'EOF'
#!/bin/sh
set -eu
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  exit 0
fi
if [ "$1" = "image" ] && [ "$2" = "save" ] && [ "$3" = "--output" ]; then
  case "$5" in
    test/legacy) cp "$LEGACY_ARCHIVE" "$4" ;;
    test/oci) cp "$OCI_ARCHIVE" "$4" ;;
    *) exit 64 ;;
  esac
  exit 0
fi
exit 64
EOF
cat >"$fake_bin/go" <<'EOF'
#!/bin/sh
set -eu
[ "$1" = "run" ]
exit 0
EOF
chmod +x "$fake_bin/docker" "$fake_bin/go"

export LEGACY_ARCHIVE="$legacy_archive"
export OCI_ARCHIVE="$oci_archive"
for image in test/legacy test/oci; do
  PATH="$fake_bin:$PATH" "$project_root/scripts/image-secret-scan.sh" "$image" |
    grep -q "image secret scan: passed for $image"
done

echo "image secret scan tests: passed"
