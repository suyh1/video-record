#!/bin/sh
set -eu

fail() {
  echo "manifest verifier: $*" >&2
  exit 1
}

for command in docker node; do
  command -v "$command" >/dev/null 2>&1 || fail "missing required command: $command"
done

image_reference=${1:-}
[ -n "$image_reference" ] || fail "usage: $0 IMAGE_REFERENCE"

manifest=$(docker buildx imagetools inspect --raw "$image_reference") ||
  fail "unable to inspect image reference"

printf '%s' "$manifest" | node -e '
let input = "";
process.stdin.on("data", (chunk) => { input += chunk });
process.stdin.on("end", () => {
  let manifest;
  try {
    manifest = JSON.parse(input);
  } catch {
    console.error("manifest verifier: inspect output is not valid JSON");
    process.exit(1);
  }
  const descriptors = Array.isArray(manifest.manifests) ? manifest.manifests : [];
  const required = ["linux/amd64", "linux/arm64"];
  const digests = [];
  for (const requiredPlatform of required) {
    const matches = descriptors.filter((entry) => {
      const platform = entry && entry.platform;
      return platform && String(platform.os) + "/" + String(platform.architecture) === requiredPlatform;
    });
    if (matches.length !== 1) {
      console.error("manifest verifier: expected exactly one descriptor for " + requiredPlatform);
      process.exit(1);
    }
    const digest = String(matches[0].digest || "");
    if (!/^sha256:[0-9a-f]{64}$/.test(digest)) {
      console.error("manifest verifier: invalid descriptor digest for " + requiredPlatform);
      process.exit(1);
    }
    digests.push(digest);
  }
  if (new Set(digests).size !== digests.length) {
    console.error("manifest verifier: required platforms share a descriptor digest");
    process.exit(1);
  }
  console.log("manifest verifier: found linux/amd64 and linux/arm64");
});
'
