import { readFileSync } from 'node:fs'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

const requireText = (contents, path, values) => {
  for (const value of values) {
    if (!contents.includes(value)) {
      throw new Error(`${path} is missing required policy text: ${value}`)
    }
  }
}

const ciPath = '.github/workflows/ci.yml'
const ci = read(ciPath)
requireText(ci, ciPath, [
  'branches: [main]',
  'fetch-depth: 0',
  'go-version: 1.26.5',
  './scripts/release-metadata-test.sh',
  './scripts/coverage-gate.sh',
  './scripts/coverage-gate-test.sh',
  './scripts/image-secret-scan-test.sh',
  './scripts/verify-manifest-test.sh',
  'gofmt -l',
  'go test ./... -race -count=1',
  'go vet ./...',
  'Test(Migrate',
  'npm ci',
  'npx playwright install --with-deps chromium',
  'npm run lint',
  'npm run typecheck',
  'npm test -- --run',
  'npm run api:check',
  'npm run build',
  'npm run e2e',
  'gitleaks/v8@v8.30.1',
  'govulncheck',
  'npm audit --audit-level=high',
  'docker build',
  './scripts/container-smoke.sh',
  './scripts/image-secret-scan.sh',
  'critical,high',
])

const releasePath = '.github/workflows/release.yml'
const release = read(releasePath)
requireText(release, releasePath, [
  "tags: ['v*.*.*']",
  'group: release-${{ github.repository }}',
  'vars.IMAGE_REPOSITORY',
  'secrets.DOCKERHUB_USERNAME',
  'secrets.DOCKERHUB_TOKEN',
  'docker/setup-qemu-action',
  'docker/setup-buildx-action',
  'linux/amd64,linux/arm64',
  'sbom: true',
  'provenance: mode=max',
  'push: true',
  'git fetch --no-tags origin main:refs/remotes/origin/main',
  './scripts/release-metadata.sh',
  'local/video-record:release-amd64',
  'local/video-record:release-arm64',
  'tags: ${{ steps.release.outputs.full_tag }}',
  'steps.build.outputs.digest',
  'docker buildx imagetools create',
  './scripts/image-secret-scan.sh',
  './scripts/verify-manifest.sh',
])
if (release.includes('group: release-${{ github.ref }}')) {
  throw new Error(`${releasePath} allows concurrent tag releases to race moving aliases`)
}

const releaseJobEnvironment = release.match(/^    env:\n((?:^      .+\n)+)/m)?.[1] || ''
if (releaseJobEnvironment.includes('DOCKERHUB_')) {
  throw new Error(`${releasePath} exposes Docker Hub credentials to the entire release job`)
}

const releaseOrder = [
  './scripts/container-smoke.sh local/video-record:release-amd64',
  'image: local://local/video-record:release-amd64',
  './scripts/image-secret-scan.sh local/video-record:release-amd64',
  './scripts/container-smoke.sh local/video-record:release-arm64',
  'image: local://local/video-record:release-arm64',
  './scripts/image-secret-scan.sh local/video-record:release-arm64',
  'push: true',
  './scripts/verify-manifest.sh',
  'docker buildx imagetools create',
].map((marker) => release.indexOf(marker))
if (releaseOrder.some((position) => position < 0) ||
    releaseOrder.some((position, index) => index > 0 && position <= releaseOrder[index - 1])) {
  throw new Error(`${releasePath} must smoke and scan both platforms before publish and alias promotion`)
}

for (const [path, contents] of [[ciPath, ci], [releasePath, release]]) {
  for (const match of contents.matchAll(/^\s*uses:\s+([^\s#]+)/gm)) {
    const action = match[1]
    if (!/@[0-9a-f]{40}$/.test(action)) {
      throw new Error(`${path} action is not pinned to a full commit SHA: ${action}`)
    }
  }
}

const dependabotPath = '.github/dependabot.yml'
const dependabot = read(dependabotPath)
for (const ecosystem of ['gomod', 'npm', 'docker', 'github-actions']) {
  requireText(dependabot, dependabotPath, [`package-ecosystem: ${ecosystem}`])
}

console.log('workflow policy tests: passed')
