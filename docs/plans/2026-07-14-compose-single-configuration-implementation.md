# Compose Single Configuration Source Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `docker-compose.yml` the only Compose deployment configuration file and remove all Compose variable interpolation and `.env` setup requirements.

**Architecture:** Keep application environment-variable support unchanged for non-Compose execution, but express every Compose value literally in the tracked YAML. Protect the deployment contract with documentation acceptance checks so `.env.example`, `${...}` interpolation, and stale `.env` instructions cannot return unnoticed.

**Tech Stack:** Docker Compose v2, POSIX shell acceptance tests, Markdown operator documentation, Go application configuration.

---

### Task 1: Add a failing Compose configuration policy check

**Files:**
- Modify: `scripts/docs-acceptance-test.sh`

**Step 1: Write the failing test**

Add checks that fail when `.env.example` exists, `docker-compose.yml` contains the literal text `${`, required Compose values are absent, or operator documentation tells users to maintain `.env`.

```sh
if [ -e .env.example ]; then
  fail '.env.example must not exist; docker-compose.yml is the only Compose configuration source'
fi

if grep -Fq '${' docker-compose.yml; then
  fail 'docker-compose.yml must use literal values instead of variable interpolation'
fi

require_pattern docker-compose.yml 'image:[[:space:]]+video-record:local' 'the explicit local image name'
require_pattern docker-compose.yml '8080:8080' 'the explicit default host port'
require_pattern docker-compose.yml 'APP_ENCRYPTION_KEY:[[:space:]]*""' 'an explicit optional encryption-key value'
```

Also scan `README.md`, `docs/deployment.md`, `docs/security.md`, `docs/integrations.md`, `docs/upgrading.md`, and `docs/release-checklist.md` for stale `.env` instructions.

**Step 2: Run test to verify it fails**

Run: `./scripts/docs-acceptance-test.sh`

Expected: FAIL because `.env.example` exists and `docker-compose.yml` still contains `${...}`.

**Step 3: Commit the red test**

```bash
git add scripts/docs-acceptance-test.sh
git commit -m "test: require compose as the single config source"
```

### Task 2: Make the Compose configuration self-contained

**Files:**
- Delete: `.env.example`
- Modify: `.gitignore`
- Modify: `docker-compose.yml`

**Step 1: Write the minimal configuration**

Use the following literal Compose values:

```yaml
image: video-record:local
ports:
  - "8080:8080"
environment:
  APP_ENV: production
  APP_PORT: "8080"
  APP_COOKIE_SECURE: "false"
  DATA_DIR: /data
  APP_ENCRYPTION_KEY: ""
  TMDB_READ_ACCESS_TOKEN: ""
```

Remove the obsolete `.env.example` allow rule from `.gitignore`, while keeping `.env` ignored as defense against accidentally committed local secrets.

**Step 2: Verify Compose parses without `.env`**

Run: `docker compose config --quiet`

Expected: exit 0 with no interpolation or missing-variable errors.

**Step 3: Commit the Compose change**

```bash
git add .gitignore docker-compose.yml .env.example
git commit -m "build: make compose configuration self-contained"
```

### Task 3: Update operator documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/deployment.md`
- Modify: `docs/security.md`
- Modify: `docs/integrations.md`
- Modify: `docs/upgrading.md`
- Modify: `docs/release-checklist.md`

**Step 1: Document one-file configuration**

Describe `docker-compose.yml` as the only Compose configuration source. Explain that the checked-in defaults support local HTTP on port 8080, and that HTTPS deployments set `APP_COOKIE_SECURE` to `"true"` directly in Compose.

**Step 2: Document optional credentials safely**

Explain that users generate `APP_ENCRYPTION_KEY` with `openssl rand -base64 32` and paste it directly into the Compose environment section before configuring media-server integrations. TMDB remains optional and is configured the same way. Warn users not to commit local secret-bearing Compose edits and to retain the encryption key in an independent secret store.

**Step 3: Replace image and port instructions**

Tell users to edit the literal `image` and `ports` entries. Use literal health-check URLs such as `http://127.0.0.1:8080/healthz`.

**Step 4: Run the policy test to verify it passes**

Run: `./scripts/docs-acceptance-test.sh`

Expected: `documentation acceptance tests: passed`.

**Step 5: Commit the documentation**

```bash
git add README.md docs/deployment.md docs/security.md docs/integrations.md docs/upgrading.md docs/release-checklist.md
git commit -m "docs: configure compose without env files"
```

### Task 4: Run repository verification

**Files:**
- Verify only

**Step 1: Verify formatting and policy**

Run: `git diff --check HEAD~3..HEAD && docker compose config --quiet && ./scripts/docs-acceptance-test.sh`

Expected: exit 0 and documentation acceptance output reports passed.

**Step 2: Verify backend behavior**

Run: `go test ./... -race -count=1 && go vet ./...`

Expected: all Go tests pass and vet exits 0.

**Step 3: Verify frontend behavior and build**

Run: `npm --prefix web test -- --run && npm --prefix web run typecheck && npm --prefix web run build`

Expected: all frontend tests pass, type checking exits 0, and the production build completes.

**Step 4: Inspect final scope**

Run: `git status --short --branch && git log -5 --oneline --decorate`

Expected: clean `main`; commits are limited to the design, policy test, Compose configuration, and aligned documentation.
