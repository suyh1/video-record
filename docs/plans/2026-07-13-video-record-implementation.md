# video-record Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build and publish a private, family-capable movie and TV viewing diary with TMDB discovery, optional Jellyfin/Emby/Plex history sync, SQLite persistence, and one multi-architecture Docker image.

**Architecture:** A single Go process owns HTTP, auth, domain services, SQLite repositories, scheduled sync, backup/restore, and embedded React assets. The React/Vite client uses a versioned JSON API; external data enters through isolated adapters and can never overwrite higher-priority user data.

**Tech Stack:** Go 1.26, `net/http` + Chi, modernc SQLite, embedded SQL migrations, React 19, Vite 7, TypeScript strict, React Router, TanStack Query, React Hook Form, Zod, Radix primitives, Lucide, Vitest, Playwright, Docker Buildx.

---

## Execution Rules

- Work directly on `main`; this project explicitly does not use feature branches or worktrees.
- Follow `@superpowers:test-driven-development` for every behavior change.
- Follow `@superpowers:systematic-debugging` for any unexpected failure.
- Run `@superpowers:verification-before-completion` before each milestone and release claim.
- Never place a real TMDB or media-server credential in a command, fixture, screenshot, log, commit, build argument, or generated artifact.
- Use `TMDB_READ_ACCESS_TOKEN` only through the local process environment.
- Make the listed commit after each task only when all task-level checks pass.

## Milestone M1: Foundation and Identity

### Task 1: Repository Tooling and Minimal Server

**Files:**
- Create: `.gitignore`
- Create: `.env.example`
- Create: `go.mod`
- Create: `cmd/server/main.go`
- Create: `internal/httpapi/router.go`
- Create: `internal/httpapi/health.go`
- Test: `internal/httpapi/health_test.go`
- Create: `Makefile`

**Step 1: Write the failing health-handler test**

```go
func TestHealthz(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    rec := httptest.NewRecorder()
    NewRouter(Dependencies{}).ServeHTTP(rec, req)
    require.Equal(t, http.StatusOK, rec.Code)
    require.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}
```

**Step 2: Run the test and verify failure**

Run: `go test ./internal/httpapi -run TestHealthz -v`

Expected: FAIL because `NewRouter` does not exist.

**Step 3: Implement the minimal server**

- Initialize module `video-record` with Go 1.26.
- Add Chi v5 and Testify.
- Implement `/healthz` and a server with read-header, read, write, and idle timeouts.
- `.env.example` lists variable names with empty or synthetic values only.
- `.gitignore` covers `.env`, `/data`, `/dist`, `web/node_modules`, Playwright output, coverage, and local binaries.

**Step 4: Verify**

Run: `go test ./...`

Expected: PASS.

Run: `go vet ./...`

Expected: no findings.

**Step 5: Commit**

```bash
git add .gitignore .env.example go.mod go.sum Makefile cmd internal
git commit -m "chore: initialize go application"
```

### Task 2: Configuration, Logging, and Request IDs

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Create: `internal/httpapi/middleware.go`
- Test: `internal/httpapi/middleware_test.go`
- Modify: `cmd/server/main.go`

**Step 1: Write failing configuration tests**

Test that defaults produce port `8080`, data directory `/data`, a development-safe cookie mode, and an error when `APP_ENCRYPTION_KEY` is present but malformed. Test that logging redacts `Authorization`, cookies, TMDB token values, and media-server tokens.

**Step 2: Verify failure**

Run: `go test ./internal/config ./internal/httpapi -v`

Expected: FAIL because configuration and middleware are missing.

**Step 3: Implement**

- Parse environment without reading `.env` in production code.
- Use `log/slog` JSON in production and text locally.
- Add request IDs to context, response headers, structured logs, and Problem Details.
- Add panic recovery that returns a generic `500` without stack traces or secrets.

**Step 4: Verify**

Run: `go test ./internal/config ./internal/httpapi -race`

Expected: PASS and no races.

**Step 5: Commit**

```bash
git add internal/config internal/httpapi cmd/server/main.go
git commit -m "feat: add configuration and safe request logging"
```

### Task 3: SQLite Connection and Embedded Migrations

**Files:**
- Create: `internal/storage/db.go`
- Create: `internal/storage/migrate.go`
- Create: `internal/storage/migrations/0001_core.sql`
- Test: `internal/storage/db_test.go`
- Test: `internal/storage/migrate_test.go`
- Modify: `internal/httpapi/health.go`

**Step 1: Write failing database tests**

Test that a temporary database enables `foreign_keys`, `journal_mode=WAL`, and `busy_timeout`; opening and migrating twice is idempotent. Test that `/readyz` fails before storage initialization and passes after a writable migrated database exists.

**Step 2: Verify failure**

Run: `go test ./internal/storage ./internal/httpapi -run 'Test(Open|Migrate|Ready)' -v`

Expected: FAIL because storage is absent.

**Step 3: Implement**

- Use `modernc.org/sqlite`.
- Configure one writer and bounded readers.
- Embed ordered SQL migration files and record applied versions.
- Create initial tables for schema metadata only; domain tables arrive with their domain tasks.
- Refuse startup when migration checksum differs from an applied migration.

**Step 4: Verify**

Run: `go test ./internal/storage ./internal/httpapi -race`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/storage internal/httpapi/health.go
git commit -m "feat: add sqlite storage and migrations"
```

### Task 4: Users, Passwords, Sessions, and Closed Initialization

**Files:**
- Create: `internal/auth/model.go`
- Create: `internal/auth/password.go`
- Create: `internal/auth/service.go`
- Create: `internal/auth/repository.go`
- Create: `internal/storage/migrations/0002_auth.sql`
- Create: `internal/httpapi/auth_handlers.go`
- Create: `internal/httpapi/csrf.go`
- Test: `internal/auth/service_test.go`
- Test: `internal/httpapi/auth_handlers_test.go`

**Step 1: Write failing auth tests**

Cover:

- First user becomes administrator.
- A second initialization request returns `409 initialization_closed`.
- Argon2id hashes verify and malformed hashes fail safely.
- Login rotates sessions and rate-limits repeated failures.
- Non-GET requests without valid Origin and CSRF token return `403`.
- Session cookie is opaque, HttpOnly, SameSite=Lax, and conditionally Secure.

**Step 2: Verify failure**

Run: `go test ./internal/auth ./internal/httpapi -run 'Test(Initialize|Login|CSRF|Session)' -v`

Expected: FAIL.

**Step 3: Implement**

- Store only a SHA-256 hash of each random session token.
- Use Argon2id with parameters recorded in the encoded hash.
- Add session expiry, revocation, last-seen update throttling, and login-attempt buckets.
- Expose `/api/v1/setup/status`, `/api/v1/setup/admin`, `/api/v1/auth/login`, `/api/v1/auth/logout`, and `/api/v1/auth/me`.

**Step 4: Verify**

Run: `go test ./internal/auth ./internal/httpapi -race`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/auth internal/httpapi internal/storage/migrations/0002_auth.sql
git commit -m "feat: add secure initialization and sessions"
```

### Task 5: Frontend Foundation and Design Tokens

**Files:**
- Create: `web/package.json`
- Create: `web/package-lock.json`
- Create: `web/vite.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/index.html`
- Create: `web/src/main.tsx`
- Create: `web/src/app/App.tsx`
- Create: `web/src/styles/tokens.css`
- Create: `web/src/styles/global.css`
- Create: `web/src/app/App.test.tsx`
- Create: `web/src/test/setup.ts`

**Step 1: Write a failing shell test**

```tsx
render(<App />)
expect(screen.getByRole('navigation', { name: '主导航' })).toBeVisible()
expect(screen.getByRole('searchbox', { name: '搜索影视' })).toBeVisible()
```

**Step 2: Verify failure**

Run: `npm --prefix web test -- --run src/app/App.test.tsx`

Expected: FAIL because the app shell does not exist.

**Step 3: Implement**

- Pin React 19, Vite 7, TypeScript, Vitest, Testing Library, React Router, TanStack Query, React Hook Form, Zod, Radix primitives, and Lucide.
- Implement the exact OKLCH tokens, typography scale, radii, spacing, z-index, light/dark themes, and reduced-motion rules from the design document.
- Create responsive desktop/tablet/mobile navigation shells with stable dimensions.
- Add local font assets only after license files are committed.

**Step 4: Verify**

Run: `npm --prefix web run typecheck`

Run: `npm --prefix web test -- --run`

Run: `npm --prefix web run build`

Expected: each command PASS.

**Step 5: Commit**

```bash
git add web
git commit -m "feat: add responsive frontend shell and design tokens"
```

## Milestone M2: TMDB and Manual Records

### Task 6: TMDB Client, Attribution, and Safe Cache

**Files:**
- Create: `internal/integrations/tmdb/client.go`
- Create: `internal/integrations/tmdb/types.go`
- Create: `internal/integrations/tmdb/cache.go`
- Test: `internal/integrations/tmdb/client_test.go`
- Create: `internal/storage/migrations/0003_tmdb_cache.sql`
- Create: `internal/httpapi/tmdb_handlers.go`
- Create: `web/src/features/settings/TmdbStatus.tsx`
- Test: `web/src/features/settings/TmdbStatus.test.tsx`

**Step 1: Write failing adapter tests**

Use `httptest.Server` to verify Bearer auth is sent upstream but absent from errors/logs; search cache lasts six hours; details last seven days; `429 Retry-After` and the eight-second deadline map to stable application errors.

**Step 2: Verify failure**

Run: `go test ./internal/integrations/tmdb ./internal/httpapi -v`

Expected: FAIL.

**Step 3: Implement**

- Read only `TMDB_READ_ACCESS_TOKEN` from server configuration.
- Implement `/search/multi`, movie details, TV details, seasons, and episodes.
- Store normalized response snapshots and expiry timestamps.
- Expose configured/unconfigured status without returning token material.
- Add TMDB attribution to the About/Settings surface.

**Step 4: Verify**

Run: `go test ./internal/integrations/tmdb ./internal/httpapi -race`

Expected: PASS.

Run: `gitleaks git --redact --no-banner`

Expected: no leaks found.

**Step 5: Commit**

```bash
git add internal/integrations/tmdb internal/httpapi internal/storage/migrations/0003_tmdb_cache.sql web/src/features/settings
git commit -m "feat: add tmdb search and metadata cache"
```

### Task 7: Media Catalog and External Identity

**Files:**
- Create: `internal/media/model.go`
- Create: `internal/media/repository.go`
- Create: `internal/media/service.go`
- Create: `internal/storage/migrations/0004_media.sql`
- Test: `internal/media/service_test.go`
- Test: `internal/media/repository_test.go`
- Create: `internal/httpapi/media_handlers.go`

**Step 1: Write failing media tests**

Test local UUID identity, uniqueness of `(source, source_id, media_type)`, merge of refreshed external fields, and preservation of local/custom fields.

**Step 2: Verify failure**

Run: `go test ./internal/media -v`

Expected: FAIL.

**Step 3: Implement**

- Add `media_items`, `media_external_ids`, `seasons`, `episodes`, and normalized genre/credit snapshot tables.
- Implement find-or-create from TMDB and stale-while-revalidate reads.
- Add custom-item creation and later TMDB linking in one transaction.

**Step 4: Verify**

Run: `go test ./internal/media ./internal/storage -race`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/media internal/httpapi/media_handlers.go internal/storage/migrations/0004_media.sql
git commit -m "feat: add local media catalog"
```

### Task 8: Personal States, Ratings, Tags, and Collections

**Files:**
- Create: `internal/records/state.go`
- Create: `internal/records/service.go`
- Create: `internal/records/repository.go`
- Create: `internal/storage/migrations/0005_user_records.sql`
- Test: `internal/records/state_test.go`
- Test: `internal/records/service_test.go`
- Create: `internal/httpapi/record_handlers.go`

**Step 1: Write failing domain tests**

Cover all five states, rating conversion to `0–100`, private tags/collections, manual-source priority, and optimistic version conflicts.

**Step 2: Verify failure**

Run: `go test ./internal/records -run 'Test(State|Rating|Priority|Version)' -v`

Expected: FAIL.

**Step 3: Implement**

- Add `user_media_states`, `tags`, `user_media_tags`, `collections`, and `collection_items`.
- Use integer ratings and explicit nullable fields.
- Add ETag/version checks and RFC 9457 conflict responses.

**Step 4: Verify**

Run: `go test ./internal/records ./internal/httpapi -race`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/records internal/httpapi/record_handlers.go internal/storage/migrations/0005_user_records.sql
git commit -m "feat: add personal media states and collections"
```

### Task 9: Immutable Watch Events and Idempotency

**Files:**
- Create: `internal/records/events.go`
- Create: `internal/storage/migrations/0006_watch_events.sql`
- Test: `internal/records/events_test.go`
- Modify: `internal/httpapi/record_handlers.go`
- Create: `internal/httpapi/idempotency.go`
- Test: `internal/httpapi/idempotency_test.go`

**Step 1: Write failing event tests**

Test that completed and rewatch create distinct events, wishlist creates none, duplicate idempotency keys replay the original result, deleting an event recomputes dates but preserves personal fields.

**Step 2: Verify failure**

Run: `go test ./internal/records ./internal/httpapi -run 'Test(Event|Idempotency|Rewatch)' -v`

Expected: FAIL.

**Step 3: Implement**

- Add `watch_events`, `watch_event_participants`, and `idempotency_keys`.
- Record event source and external event ID.
- Keep idempotency responses for a bounded retention period.

**Step 4: Verify**

Run: `go test ./internal/records ./internal/httpapi -race`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/records internal/httpapi internal/storage/migrations/0006_watch_events.sql
git commit -m "feat: add idempotent viewing events"
```

### Task 10: Search, Details, Library, and Quick Record UI

**Files:**
- Create: `web/src/api/client.ts`
- Create: `web/src/api/types.ts`
- Create: `web/src/features/search/SearchDialog.tsx`
- Create: `web/src/features/media/MediaPoster.tsx`
- Create: `web/src/features/media/MediaDetailsPage.tsx`
- Create: `web/src/features/records/QuickRecordForm.tsx`
- Create: `web/src/features/library/LibraryPage.tsx`
- Test: matching `*.test.tsx` files

**Step 1: Write failing UI tests**

Cover 300ms debounce, local-first results, media type/year/original title/status labels, two-action wishlist flow, completed default date, expanded optional fields, input preservation on network failure, and conflict resolution.

**Step 2: Verify failure**

Run: `npm --prefix web test -- --run src/features`

Expected: FAIL.

**Step 3: Implement**

- Use MSW for API tests.
- Use TanStack Query for remote state; do not duplicate it in a global store.
- Implement stable `2:3` poster boxes and neutral missing-image states.
- Provide undo toasts for ordinary changes and dialogs only for destructive actions.

**Step 4: Verify**

Run: `npm --prefix web run typecheck`

Run: `npm --prefix web test -- --run`

Run: `npm --prefix web run build`

Expected: each command PASS.

**Step 5: Commit**

```bash
git add web/src
git commit -m "feat: add search library and quick recording ui"
```

## Milestone M3: Episodes, Calendar, and Statistics

### Task 11: Episode Progress and Series State Projection

**Files:**
- Create: `internal/records/progress.go`
- Create: `internal/storage/migrations/0007_episode_progress.sql`
- Test: `internal/records/progress_test.go`
- Create: `internal/httpapi/progress_handlers.go`
- Create: `web/src/features/episodes/EpisodeProgress.tsx`
- Test: `web/src/features/episodes/EpisodeProgress.test.tsx`

**Step 1: Write failing tests**

Cover single episode, contiguous range, whole season, next episode, undo, `S02E03` plus absolute count, completed projection, and protection of explicit dropped state.

**Step 2: Verify failure**

Run: `go test ./internal/records -run TestEpisode -v`

Expected: FAIL.

Run: `npm --prefix web test -- --run EpisodeProgress`

Expected: FAIL.

**Step 3: Implement**

Add `episode_progress` and batch APIs with one transaction and one idempotency key per user action.

**Step 4: Verify**

Run: `go test ./internal/records ./internal/httpapi -race`

Run: `npm --prefix web test -- --run EpisodeProgress`

Expected: both commands PASS.

**Step 5: Commit**

```bash
git add internal/records internal/httpapi/progress_handlers.go internal/storage/migrations/0007_episode_progress.sql web/src/features/episodes
git commit -m "feat: add episode progress tracking"
```

### Task 12: Calendar and Timeline Queries

**Files:**
- Create: `internal/records/calendar.go`
- Test: `internal/records/calendar_test.go`
- Create: `internal/httpapi/calendar_handlers.go`
- Create: `web/src/features/calendar/CalendarPage.tsx`
- Create: `web/src/features/calendar/MonthGrid.tsx`
- Create: `web/src/features/calendar/AgendaList.tsx`
- Test: matching `*.test.tsx`

**Step 1: Write failing tests**

Test timezone boundaries, repeated watches on one day, shared participants, desktop month grouping, mobile agenda order, and filters for completed/in-progress.

**Step 2: Verify failure**

Run: `go test ./internal/records -run TestCalendar -v`

Expected: FAIL.

**Step 3: Implement and verify**

Run: `go test ./internal/records ./internal/httpapi -race`

Run: `npm --prefix web test -- --run Calendar`

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/records internal/httpapi/calendar_handlers.go web/src/features/calendar
git commit -m "feat: add viewing calendar and timeline"
```

### Task 13: Accessible Statistics

**Files:**
- Create: `internal/stats/service.go`
- Test: `internal/stats/service_test.go`
- Create: `internal/httpapi/stats_handlers.go`
- Create: `web/src/features/stats/StatsPage.tsx`
- Create: `web/src/features/stats/AccessibleChart.tsx`
- Test: matching `*.test.tsx`

**Step 1: Write failing tests**

Cover monthly/yearly counts, genres, ratings, duration, tags, viewing methods, repeat views, and user isolation. UI tests require a textual table equivalent for every chart.

**Step 2: Verify failure**

Run: `go test ./internal/stats -v`

Expected: FAIL.

**Step 3: Implement and verify**

Run: `go test ./internal/stats ./internal/httpapi -race`

Run: `npm --prefix web test -- --run Stats`

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/stats internal/httpapi/stats_handlers.go web/src/features/stats
git commit -m "feat: add private viewing statistics"
```

## Milestone M4: Household, Portability, and Recovery

### Task 14: Household Members and Privacy Boundaries

**Files:**
- Create: `internal/household/service.go`
- Create: `internal/household/policy.go`
- Test: `internal/household/policy_test.go`
- Create: `internal/httpapi/household_handlers.go`
- Create: `web/src/features/household/MemberSettings.tsx`
- Test: `web/src/features/household/MemberSettings.test.tsx`

**Step 1: Write failing policy tests**

Prove that administrators cannot read private notes, users cannot mutate another member's state, shared events expose only approved fields, and public-to-household rating/short-review flags work explicitly.

**Step 2: Verify failure**

Run: `go test ./internal/household ./internal/httpapi -run TestPolicy -v`

Expected: FAIL.

**Step 3: Implement and verify**

Run: `go test ./internal/household ./internal/httpapi -race`

Run: `npm --prefix web test -- --run MemberSettings`

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/household internal/httpapi/household_handlers.go web/src/features/household
git commit -m "feat: enforce household privacy boundaries"
```

### Task 15: JSON/CSV Export and Safe Import

**Files:**
- Create: `internal/records/export.go`
- Create: `internal/records/import.go`
- Test: `internal/records/import_export_test.go`
- Create: `internal/httpapi/import_export_handlers.go`
- Create: `web/src/features/settings/DataTransfer.tsx`
- Test: `web/src/features/settings/DataTransfer.test.tsx`

**Step 1: Write failing round-trip and hostile-input tests**

Test JSON round-trip equality, CSV formula injection neutralization, size limits, invalid UTF-8, duplicate external IDs, path-like filenames, partial failure reports, and absence of credentials/private data outside the selected user scope.

**Step 2: Verify failure**

Run: `go test ./internal/records -run 'Test(Import|Export)' -v`

Expected: FAIL.

**Step 3: Implement and verify**

Run: `go test ./internal/records ./internal/httpapi -race`

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/records internal/httpapi/import_export_handlers.go web/src/features/settings/DataTransfer*
git commit -m "feat: add safe data import and export"
```

### Task 16: Consistent Backup and Atomic Restore

**Files:**
- Create: `internal/storage/backup.go`
- Create: `internal/storage/restore.go`
- Test: `internal/storage/backup_restore_test.go`
- Create: `internal/httpapi/backup_handlers.go`
- Create: `web/src/features/settings/BackupRestore.tsx`
- Test: `web/src/features/settings/BackupRestore.test.tsx`

**Step 1: Write failing recovery tests**

Cover Online Backup consistency during writes, manifest/checksum verification, version incompatibility, insufficient space, path traversal, pre-restore snapshot, atomic replacement, forced failure rollback, and missing encryption key behavior.

**Step 2: Verify failure**

Run: `go test ./internal/storage -run 'Test(Backup|Restore)' -v`

Expected: FAIL.

**Step 3: Implement**

- Put the API into maintenance mode for the replacement window.
- Fsync the replacement and parent directory before success.
- Never package environment variables or encryption keys.

**Step 4: Verify**

Run: `go test ./internal/storage ./internal/httpapi -race`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/storage internal/httpapi/backup_handlers.go web/src/features/settings/BackupRestore*
git commit -m "feat: add consistent backup and atomic restore"
```

## Milestone M5: Media Server Synchronization

### Task 17: Provider Contract, Encrypted Accounts, and Scheduler

**Files:**
- Create: `internal/integrations/provider.go`
- Create: `internal/integrations/credentials.go`
- Create: `internal/sync/service.go`
- Create: `internal/sync/scheduler.go`
- Create: `internal/storage/migrations/0008_integrations.sql`
- Test: `internal/integrations/provider_contract_test.go`
- Test: `internal/integrations/credentials_test.go`
- Test: `internal/sync/scheduler_test.go`

**Step 1: Write failing contract tests**

Define one conformance suite for authentication check, paginated history, stable event IDs, item identity hints, cancellation, redaction, and retry classification. Test AEAD round-trip, tamper detection, versioning, missing key lock, persisted due jobs, restart catch-up, and single-run leases.

**Step 2: Verify failure**

Run: `go test ./internal/integrations ./internal/sync -v`

Expected: FAIL.

**Step 3: Implement**

- Persist only ciphertext, nonce, version, and fingerprint.
- Schedule 15-minute increments and daily compensation.
- Keep HTTP serving while jobs run.

**Step 4: Verify**

Run: `go test ./internal/integrations ./internal/sync -race`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/integrations internal/sync internal/storage/migrations/0008_integrations.sql
git commit -m "feat: add encrypted integration scheduler"
```

### Task 18: Jellyfin Provider

**Files:**
- Create: `internal/integrations/jellyfin/client.go`
- Create: `internal/integrations/jellyfin/mapper.go`
- Test: `internal/integrations/jellyfin/client_test.go`
- Add fixtures: `internal/integrations/jellyfin/testdata/*.json`

**Step 1: Write failing provider tests**

Use synthetic fixtures for movies, episodes, repeat plays, pagination, deleted users, malformed provider responses, and rate/server failures. Run the shared Provider conformance suite.

**Step 2: Verify failure**

Run: `go test ./internal/integrations/jellyfin -v`

Expected: FAIL.

**Step 3: Implement and verify**

Run: `go test ./internal/integrations/jellyfin ./internal/integrations -race`

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/integrations/jellyfin
git commit -m "feat: add jellyfin history provider"
```

### Task 19: Emby Provider

**Files:**
- Create: `internal/integrations/emby/client.go`
- Create: `internal/integrations/emby/mapper.go`
- Test: `internal/integrations/emby/client_test.go`
- Add fixtures: `internal/integrations/emby/testdata/*.json`

Repeat Task 18's redaction, pagination, event-ID, mapping, error-classification, and Provider conformance steps using Emby-specific synthetic responses.

Run: `go test ./internal/integrations/emby ./internal/integrations -race`

Expected: PASS.

Commit:

```bash
git add internal/integrations/emby
git commit -m "feat: add emby history provider"
```

### Task 20: Plex Provider

**Files:**
- Create: `internal/integrations/plex/client.go`
- Create: `internal/integrations/plex/mapper.go`
- Test: `internal/integrations/plex/client_test.go`
- Add fixtures: `internal/integrations/plex/testdata/*`

Repeat the shared Provider conformance workflow, including Plex XML/JSON parsing boundaries, stable rating keys, pagination, redaction, cancellation, and duplicate event prevention.

Run: `go test ./internal/integrations/plex ./internal/integrations -race`

Expected: PASS.

Commit:

```bash
git add internal/integrations/plex
git commit -m "feat: add plex history provider"
```

### Task 21: Candidate Matching, Conflict Resolution, and Sync UI

**Files:**
- Create: `internal/sync/matcher.go`
- Create: `internal/sync/candidates.go`
- Test: `internal/sync/matcher_test.go`
- Create: `internal/httpapi/sync_handlers.go`
- Create: `web/src/features/sync/SyncStatus.tsx`
- Create: `web/src/features/sync/CandidateReviewPage.tsx`
- Test: matching `*.test.tsx`

**Step 1: Write failing matching tests**

Cover exact external ID, TMDB ID, title/year ambiguity, movie/TV mismatch, conflicts with manual data, ignored candidates, remapping, custom item creation, and repeated external event IDs.

**Step 2: Verify failure**

Run: `go test ./internal/sync -run 'Test(Match|Candidate|Conflict)' -v`

Expected: FAIL.

**Step 3: Implement**

- Auto-apply only exact, conflict-free candidates.
- Return human-readable evidence for every proposed match.
- Provide confirm, rematch, ignore, and custom-item actions.

**Step 4: Verify**

Run: `go test ./internal/sync ./internal/httpapi -race`

Run: `npm --prefix web test -- --run Sync`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/sync internal/httpapi/sync_handlers.go web/src/features/sync
git commit -m "feat: add safe sync candidate review"
```

## Milestone M6: Hardening, Container, and Release

### Task 22: OpenAPI Contract and Full HTTP Security Tests

**Files:**
- Create: `api/openapi.yaml`
- Create: `internal/httpapi/contract_test.go`
- Create: `internal/httpapi/security_test.go`
- Create: `web/src/api/generated.ts`
- Modify: `web/package.json`

**Step 1: Write failing contract tests**

Validate every registered `/api/v1` route against OpenAPI; assert RFC 9457 media type, cursor shape, ETag behavior, idempotency replay, CSRF, object-level authorization, session revocation, and redacted logs.

**Step 2: Verify failure**

Run: `go test ./internal/httpapi -run 'Test(Contract|Security)' -v`

Expected: FAIL until the specification and route behavior agree.

**Step 3: Implement and verify**

- Generate TypeScript types with `openapi-typescript`.
- Add CI check that regenerated output is clean.

Run: `go test ./internal/httpapi -race`

Run: `npm --prefix web run api:check`

Run: `npm --prefix web run typecheck`

Expected: both commands PASS.

**Step 4: Commit**

```bash
git add api internal/httpapi web/src/api/generated.ts web/package*.json
git commit -m "test: enforce api and security contracts"
```

### Task 23: E2E, Accessibility, and Visual Regression

**Files:**
- Create: `web/playwright.config.ts`
- Create: `web/e2e/setup.spec.ts`
- Create: `web/e2e/recording.spec.ts`
- Create: `web/e2e/episodes.spec.ts`
- Create: `web/e2e/household.spec.ts`
- Create: `web/e2e/sync.spec.ts`
- Create: `web/e2e/backup.spec.ts`
- Create: `web/e2e/accessibility.spec.ts`
- Create: `web/e2e/visual.spec.ts`

**Step 1: Write failing E2E tests**

Cover the complete approved journeys and run axe after each major page. Capture light/dark snapshots at `375x812`, `768x1024`, and `1440x900`; assert no horizontal overflow and test reduced motion.

**Step 2: Verify failure**

Run: `npm --prefix web run e2e`

Expected: FAIL until all routes and test harness hooks exist.

**Step 3: Implement test harness and fix product defects**

Use seeded synthetic data and mock external services. Never store real credentials in Playwright traces.

**Step 4: Verify**

Run: `npm --prefix web run e2e`

Expected: PASS with zero blocking WCAG 2.2 AA findings.

**Step 5: Commit**

```bash
git add web/playwright.config.ts web/e2e web/package*.json
git commit -m "test: add end to end accessibility coverage"
```

### Task 24: Performance and Failure-Recovery Harness

**Files:**
- Create: `internal/testutil/seed.go`
- Create: `scripts/perf-smoke.sh`
- Create: `scripts/recovery-smoke.sh`
- Create: `docs/performance.md`

**Step 1: Create failing threshold checks**

Seed 10,000 media items, 50,000 events, and five users. Fail when local API p95 exceeds 200ms, library filter p95 exceeds 300ms, initial 10,000-item sync exceeds five minutes, or incremental 100-item sync exceeds 60 seconds. Recovery script kills the process during write, migration simulation, sync, and backup.

**Step 2: Run and record baseline failures**

Run: `./scripts/perf-smoke.sh`

Run: `./scripts/recovery-smoke.sh`

Expected: threshold or missing-harness failures before tuning.

**Step 3: Optimize only measured bottlenecks**

Add indexes/query changes with regression tests. Do not add Redis or PostgreSQL.

**Step 4: Verify**

Run both scripts three times.

Expected: all thresholds pass and recovery preserves consistent data.

**Step 5: Commit**

```bash
git add internal/testutil scripts docs/performance.md
git commit -m "test: enforce performance and recovery targets"
```

### Task 25: Production Docker Image and Compose

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`
- Create: `docker-compose.yml`
- Create: `internal/assets/embed.go`
- Create: `cmd/server/healthcheck.go`
- Create: `scripts/container-smoke.sh`
- Modify: `cmd/server/main.go`
- Modify: `web/vite.config.ts`

**Step 1: Write the failing smoke script**

The script must assert: non-root UID, read-only root filesystem, only `/data` writable, port 8080, healthcheck binary command, initialization, record persistence after restart, backup/restore, and no credential-like strings in image history.

**Step 2: Verify failure**

Run: `./scripts/container-smoke.sh local/video-record:test`

Expected: FAIL because the image is absent.

**Step 3: Implement**

- Multi-stage Node and Go build.
- `CGO_ENABLED=0`; embed `web/dist`.
- Minimal non-root runtime with CA certificates and timezone data.
- Compose has one app service and one named volume.
- Secrets have empty/required examples, never real defaults.

**Step 4: Verify**

Run: `docker build -t local/video-record:test .`

Run: `./scripts/container-smoke.sh local/video-record:test`

Expected: PASS.

**Step 5: Commit**

```bash
git add Dockerfile .dockerignore docker-compose.yml internal/assets cmd/server scripts/container-smoke.sh web/vite.config.ts
git commit -m "build: add hardened production container"
```

### Task 26: CI, Multi-Architecture Release, and Supply Chain Metadata

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`
- Create: `.github/dependabot.yml`
- Create: `scripts/verify-manifest.sh`

**Step 1: Add CI checks that initially fail on missing commands**

CI must run Go format/test/race/vet, frontend lint/typecheck/test/build, migration tests, API generation check, E2E, secret scan, dependency scan, and container smoke.

**Step 2: Implement release workflow**

- Trigger only on `v*.*.*` tags.
- Require `IMAGE_REPOSITORY` repository variable and Docker Hub credentials as secrets.
- Build `linux/amd64,linux/arm64` with Buildx/QEMU.
- Push full, major.minor, and `latest` tags only for stable SemVer.
- Generate SBOM and provenance attestations.
- Verify the manifest contains both platforms.

**Step 3: Validate workflow syntax locally**

Run: `git diff --check`

Run: `./scripts/verify-manifest.sh local-or-test-reference` when a test registry reference is available.
Expected: workflow parses and manifest verifier rejects single-platform images.

**Step 4: Commit**

```bash
git add .github scripts/verify-manifest.sh
git commit -m "ci: add validation and multi architecture release"
```

### Task 27: Operator Documentation and v1 Release Gate

**Files:**
- Create: `README.md`
- Create: `docs/deployment.md`
- Create: `docs/backup-restore.md`
- Create: `docs/upgrading.md`
- Create: `docs/security.md`
- Create: `docs/integrations.md`
- Create: `docs/release-checklist.md`

**Step 1: Write documentation acceptance checks**

Create a checklist that requires fresh-machine Compose install, synthetic secret generation, TMDB attribution, port changes, backup and restore rehearsal, encryption-key retention warning, amd64/arm64 smoke results, upgrade rollback, and secret scan.

**Step 2: Draft documentation with no real credentials**

All examples use placeholders such as `${TMDB_READ_ACCESS_TOKEN}` and generated random values. Explicitly warn that losing `APP_ENCRYPTION_KEY` locks integrations but not viewing records.

**Step 3: Run the complete release gate**

Run:

```bash
go test ./... -race
go vet ./...
npm --prefix web ci
npm --prefix web run lint
npm --prefix web run typecheck
npm --prefix web test -- --run
npm --prefix web run build
npm --prefix web run e2e
docker build -t local/video-record:release-candidate .
./scripts/container-smoke.sh local/video-record:release-candidate
./scripts/perf-smoke.sh
./scripts/recovery-smoke.sh
git diff --check
```

Expected: every command PASS, zero blocking accessibility findings, zero high/critical vulnerabilities, and no secret matches.

**Step 4: Commit documentation**

```bash
git add README.md docs
git commit -m "docs: add deployment and release operations"
```

**Step 5: Tag only after external checklist completion**

Do not create or push `v1.0.0` automatically. After Docker Hub repository variables and credentials are configured and `docs/release-checklist.md` is signed off:

```bash
git tag -s v1.0.0 -m "video-record v1.0.0"
git push origin main v1.0.0
```

Expected: release workflow publishes a two-platform manifest with SBOM and provenance. This final external push requires explicit user authorization at execution time.

## Final Verification

Before declaring the implementation complete:

1. Re-read `docs/plans/2026-07-13-video-record-design.md` and map every MUST to a passing test or documented operator check.
2. Run all commands from Task 27 on a clean checkout.
3. Inspect image history and exported artifacts for credentials.
4. Verify amd64 and arm64 images independently.
5. Perform one backup/restore rehearsal with realistic synthetic data.
6. Record results in `docs/release-checklist.md` with command output summaries and artifact digests.
