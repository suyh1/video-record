#!/bin/sh
set -eu

fail() {
  echo "image secret scan: $*" >&2
  exit 1
}

for command in docker find go node strings tar; do
  command -v "$command" >/dev/null 2>&1 || fail "missing required command: $command"
done

image=${1:-}
[ -n "$image" ] || fail "usage: $0 IMAGE"
docker image inspect "$image" >/dev/null 2>&1 || fail "image not found: $image"

scan_directory=$(mktemp -d)
trap 'rm -rf "$scan_directory"' EXIT INT TERM

docker image save --output "$scan_directory/image.tar" "$image"
mkdir "$scan_directory/archive"
tar -xf "$scan_directory/image.tar" -C "$scan_directory/archive"
node - "$scan_directory/archive" <<'NODE' >"$scan_directory/layers.txt"
const fs = require('fs')
const path = require('path')
const root = process.argv[2]
const safeRelativePath = (value) => {
  const normalized = path.posix.normalize(String(value || '').replaceAll('\\', '/'))
  if (!normalized || normalized === '.' || normalized === '..' ||
      normalized.startsWith('../') || path.posix.isAbsolute(normalized)) process.exit(1)
  return normalized
}
const indexPath = path.join(root, 'index.json')
const legacyManifestPath = path.join(root, 'manifest.json')
if (fs.existsSync(indexPath)) {
  const index = JSON.parse(fs.readFileSync(indexPath, 'utf8'))
  const visitedDescriptors = new Set()
  const emittedLayers = new Set()
  const digestValue = (descriptor) => {
    const digest = String(descriptor?.digest || '').replace(/^sha256:/, '')
    if (!/^[0-9a-f]{64}$/.test(digest)) process.exit(1)
    return digest
  }
  const isArchiveLayer = (layer) => {
    const mediaType = String(layer?.mediaType || '')
    return mediaType === '' ||
      (mediaType.includes('image.layer') && mediaType.includes('tar')) ||
      mediaType.includes('rootfs.diff.tar')
  }
  const visitDescriptor = (descriptor) => {
    const digest = digestValue(descriptor)
    if (visitedDescriptors.has(digest)) return
    visitedDescriptors.add(digest)
    const document = JSON.parse(fs.readFileSync(path.join(root, 'blobs', 'sha256', digest), 'utf8'))
    if (Array.isArray(document.manifests)) {
      for (const child of document.manifests) visitDescriptor(child)
      return
    }
    if (!Array.isArray(document.layers)) process.exit(1)
    for (const layer of document.layers) {
      if (!isArchiveLayer(layer)) continue
      const layerDigest = digestValue(layer)
      if (emittedLayers.has(layerDigest)) continue
      emittedLayers.add(layerDigest)
      process.stdout.write('blobs/sha256/' + layerDigest + '\n')
    }
  }
  if (!Array.isArray(index.manifests)) process.exit(1)
  for (const descriptor of index.manifests) visitDescriptor(descriptor)
} else if (fs.existsSync(legacyManifestPath)) {
  const manifests = JSON.parse(fs.readFileSync(legacyManifestPath, 'utf8'))
  if (!Array.isArray(manifests)) process.exit(1)
  for (const manifest of manifests) {
    if (!Array.isArray(manifest.Layers)) process.exit(1)
    for (const layer of manifest.Layers) process.stdout.write(safeRelativePath(layer) + '\n')
  }
} else {
  process.exit(1)
}
NODE

mkdir "$scan_directory/layers"
layer_index=0
while IFS= read -r layer_path; do
  layer_index=$((layer_index + 1))
  target="$scan_directory/layers/$layer_index"
  mkdir "$target"
  tar -xf "$scan_directory/archive/$layer_path" -C "$target"
done <"$scan_directory/layers.txt"
[ "$layer_index" -gt 0 ] || fail "image contains no runtime layers"

find "$scan_directory/layers" -type f -exec strings {} + >"$scan_directory/image-strings.txt"
go run github.com/zricethezav/gitleaks/v8@v8.30.1 dir "$scan_directory/archive" --redact --no-banner
go run github.com/zricethezav/gitleaks/v8@v8.30.1 dir "$scan_directory/layers" --redact --no-banner
go run github.com/zricethezav/gitleaks/v8@v8.30.1 dir "$scan_directory/image-strings.txt" --redact --no-banner

file_count=$(find "$scan_directory/layers" -type f | wc -l | tr -d ' ')
echo "image secret scan: passed for $image ($layer_index layers, $file_count files)"
