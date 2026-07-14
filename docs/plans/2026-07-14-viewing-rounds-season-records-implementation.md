# Viewing Rounds and Season Records Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace media-wide personal records and the flat watch history with explicit movie/season viewing rounds, editable second-precision episode times, and read-only rewatch archives.

**Architecture:** Add a media-level profile for catalog/share settings and normalized viewing rounds for private record state. Rebase episode progress and watch events on a round, expose scoped round APIs, then migrate all projections and the details UI in layers.

**Tech Stack:** Go 1.26, chi v5, SQLite STRICT migrations, OpenAPI 3.1, React 19, TypeScript 7, TanStack Query, React Hook Form, Zod, Radix Dialog, Vitest/MSW, Playwright.

---

## Execution Constraints

- Work directly on `main`; do not create a branch or worktree.
- Use @superpowers:test-driven-development for every behavior change.
- Use @superpowers:systematic-debugging for any unexpected failure.
- Use @impeccable for the details-page controls and responsive polish.
- Use @superpowers:verification-before-completion before every completion claim.
- Do not add a Go or frontend dependency.
- Preserve users, sessions, media, TMDB identities, tags, collections, integration accounts and configuration.
- Deliberately delete old record facts; do not add a compatibility path for version 1 record data.
- Keep archived rounds private and immutable.
- Commit only after the task-specific RED/GREEN cycle and verification pass.

### Task 1: Add the destructive viewing-round migration

**Files:**
- Create: `internal/storage/migrations/0013_viewing_rounds.sql`
- Modify: `internal/storage/migrate_test.go`
- Test: `internal/storage/migrate_test.go`

**Step 1: Write the failing migration tests**

Add `TestViewingRoundsMigrationClearsRecordFactsAndPreservesConfiguration` and `TestViewingRoundsMigrationEnforcesScopeAndCurrentRoundUniqueness`. Seed users, sessions, media, external IDs, tags, collections, integration accounts, old states/events/progress/sharing and sync candidates before applying migration 13.

Assert the migration:

```go
require.Equal(t, 1, countRows(t, db, "users"))
require.Equal(t, 1, countRows(t, db, "media_items"))
require.Equal(t, 1, countRows(t, db, "collections"))
require.Equal(t, 1, countRows(t, db, "external_accounts"))
require.Zero(t, countRows(t, db, "user_media_tags"))
require.Zero(t, countRows(t, db, "sync_candidates"))
require.Zero(t, countRows(t, db, "watch_rounds"))
require.NoError(t, foreignKeyCheck(db))
```

Then insert duplicate current movie rounds and duplicate current season rounds and require a unique-constraint failure. Insert movie scope with `season_number = NULL` and TV scope with positive season numbers successfully.

**Step 2: Run the migration tests to verify RED**

Run: `go test ./internal/storage -run 'TestViewingRoundsMigration' -count=1 -v`

Expected: FAIL because migration 13 and the new tables do not exist.

**Step 3: Implement the minimum STRICT schema**

Create `user_media_profiles` with `user_id + media_id` primary key, projected `status`, independent `version`, sharing flags/review and timestamps.

Create `watch_rounds` with this contract:

```sql
CREATE TABLE watch_rounds (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    season_number INTEGER CHECK (season_number IS NULL OR season_number > 0),
    round_number INTEGER NOT NULL CHECK (round_number > 0),
    status TEXT NOT NULL CHECK (status IN ('none','wishlist','watching','completed','dropped')),
    rating INTEGER CHECK (rating BETWEEN 0 AND 100),
    note TEXT,
    viewing_method TEXT,
    started_at TEXT,
    completed_at TEXT,
    archived_at TEXT,
    version INTEGER NOT NULL CHECK (version > 0),
    status_source TEXT NOT NULL,
    rating_source TEXT NOT NULL,
    note_source TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX watch_rounds_scope_number
ON watch_rounds(user_id, media_id, COALESCE(season_number, 0), round_number);
CREATE UNIQUE INDEX watch_rounds_current_scope
ON watch_rounds(user_id, media_id, COALESCE(season_number, 0))
WHERE archived_at IS NULL;
```

Recreate `watch_events` with a required `round_id` foreign key, recreate participants, and create `round_episode_progress(round_id, episode_id, watched_at, source, watch_event_id, updated_at)`.

Before dropping old tables, delete `idempotency_keys`, `sync_candidates` and `user_media_tags`. Drop old progress/participant/event/state tables in foreign-key-safe order. Preserve the tables listed in the execution constraints.

**Step 4: Run migration verification to verify GREEN**

Run: `go test ./internal/storage -run 'TestViewingRoundsMigration|TestMigrations' -count=1 -v`

Expected: PASS with `PRAGMA foreign_key_check` returning no rows.

**Step 5: Commit**

```bash
git add internal/storage/migrations/0013_viewing_rounds.sql internal/storage/migrate_test.go
git commit -m "feat: add viewing round schema"
```

### Task 2: Build the viewing-round domain and repository

**Files:**
- Create: `internal/records/rounds.go`
- Create: `internal/records/round_repository.go`
- Modify: `internal/records/repository.go`
- Modify: `internal/records/service.go`
- Create: `internal/records/rounds_test.go`
- Test: `internal/records/rounds_test.go`

**Step 1: Write the failing domain tests**

Cover an empty version-0 read, first movie write, independent season 1/2 writes, user isolation, invalid movie/season scopes, rating bounds, source priority, archived-write rejection and future completion time.

Use a fixed service clock:

```go
now := time.Date(2026, 7, 14, 12, 30, 45, 0, time.UTC)
service := NewService(repository, ServiceOptions{Now: func() time.Time { return now }})
round, err := service.CurrentRound(ctx, RoundScope{UserID: userID, MediaID: movieID})
require.NoError(t, err)
require.Zero(t, round.Version)
require.Equal(t, StatusNone, round.Status)
```

Require `ErrInvalidRoundScope` for a TV scope without a season, a movie scope with a season, or a non-positive season. Require `ErrInvalidWatchedAt` for a completion time after `now`.

**Step 2: Run the tests to verify RED**

Run: `go test ./internal/records -run 'Test(CurrentRound|UpdateRound|RoundScope)' -count=1 -v`

Expected: FAIL because `RoundScope`, `WatchRound`, `CurrentRound` and `UpdateRound` do not exist.

**Step 3: Implement the minimum round API**

Add:

```go
type RoundScope struct {
    UserID       string
    MediaID      string
    SeasonNumber *int
}

type WatchRound struct {
    ID, UserID, MediaID string
    SeasonNumber        *int
    RoundNumber         int
    Status              Status
    Rating              *int
    Note                *string
    ViewingMethod       *string
    StartedAt           *time.Time
    CompletedAt         *time.Time
    ArchivedAt          *time.Time
    Version             int
    StatusSource        Source
    RatingSource        Source
    NoteSource          Source
}
```

Add optional `ServiceOptions{Now func() time.Time}` without breaking existing `NewService(repository)` callers. Validate media type by reading `media_items.media_type` before accepting the scope. Return a synthetic empty round with version 0 on no rows; create round 1 only in the first write transaction.

Keep query parsing and nullable time helpers centralized in `round_repository.go`. Do not duplicate ad hoc time layouts.

**Step 4: Run the tests to verify GREEN**

Run: `go test ./internal/records -run 'Test(CurrentRound|UpdateRound|RoundScope)' -count=1 -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/records/rounds.go internal/records/round_repository.go internal/records/repository.go internal/records/service.go internal/records/rounds_test.go
git commit -m "feat: add viewing round domain"
```

### Task 3: Project current rounds into media profiles

**Files:**
- Create: `internal/records/profile.go`
- Modify: `internal/records/repository.go`
- Modify: `internal/records/catalog.go`
- Modify: `internal/records/catalog_test.go`
- Modify: `internal/records/service_test.go`
- Test: `internal/records/catalog_test.go`
- Test: `internal/records/service_test.go`

**Step 1: Write the failing projection tests**

Prove:

- A movie profile mirrors its current round.
- Any watching season makes the TV profile `watching`.
- Otherwise the most recently updated current season supplies the TV status.
- Archived rounds never affect the profile.
- Tags can create/update a versioned profile even before a round exists.
- Search and library return the projected profile without duplicate TV rows.

```go
items, err := service.Library(ctx, userID, StatusWatching)
require.NoError(t, err)
require.Equal(t, []string{seriesID}, catalogIDs(items))
```

**Step 2: Run the tests to verify RED**

Run: `go test ./internal/records -run 'Test(MediaProfile|Library.*Round|Tags.*Profile)' -count=1 -v`

Expected: FAIL because catalog and tag versioning still query `user_media_states`.

**Step 3: Implement the projection**

Add repository helpers that upsert `user_media_profiles` in the same transaction as round changes. Move catalog/search/tag version queries from `user_media_states` to profiles. Profile version changes only for profile-owned settings; round edits use round version.

Return both versions where the details page needs them:

```go
type CurrentRecord struct {
    Round          WatchRound
    ProfileVersion int
}
```

Do not copy rating or private notes into the profile.

**Step 4: Run the tests to verify GREEN**

Run: `go test ./internal/records -run 'Test(MediaProfile|Library.*Round|Tags.*Profile)' -count=1 -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/records/profile.go internal/records/repository.go internal/records/catalog.go internal/records/catalog_test.go internal/records/service_test.go
git commit -m "feat: project viewing rounds into library"
```

### Task 4: Expose scoped current-round HTTP and OpenAPI contracts

**Files:**
- Create: `internal/httpapi/round_handlers.go`
- Create: `internal/httpapi/round_handlers_test.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/record_handlers.go`
- Modify: `internal/httpapi/contract_test.go`
- Modify: `api/openapi.yaml`
- Modify: `web/src/api/generated.ts`

**Step 1: Write the failing HTTP contract tests**

Test authenticated movie GET without `seasonNumber`, TV GET/PUT with `seasonNumber=2`, wrong scope errors, version-0 creation, `If-Match` conflicts, CSRF/origin/idempotency and future movie completion rejection.

Assert the response includes:

```json
{
  "roundId": "",
  "mediaId": "series-1",
  "seasonNumber": 2,
  "roundNumber": 1,
  "status": "none",
  "rating": null,
  "note": null,
  "viewingMethod": null,
  "watchedAt": null,
  "version": 0,
  "profileVersion": 0
}
```

**Step 2: Run tests to verify RED**

Run: `go test ./internal/httpapi -run 'Test(CurrentRound|UpdateCurrentRound)' -count=1 -v`

Expected: FAIL with 404 because round routes do not exist.

**Step 3: Implement handlers and schemas**

Register:

```go
protected.Route("/records/{mediaID}/rounds", func(rounds chi.Router) {
    rounds.Get("/current", recordAPI.currentRound)
    rounds.With(origin, csrf, idempotency.Required).Put("/current", recordAPI.updateCurrentRound)
})
```

Use stable errors `invalid_round_scope`, `invalid_watched_at`, `round_archived` and existing `version_conflict`. Update OpenAPI schemas for `CurrentRound` and `UpdateCurrentRoundRequest`, generate TypeScript, and ensure generated code is committed.

**Step 4: Run contract verification to verify GREEN**

Run: `go test ./internal/httpapi -run 'Test(CurrentRound|UpdateCurrentRound|OpenAPI)' -count=1 -v`

Run: `npm --prefix web run api:generate`

Run: `npm --prefix web run api:check`

Expected: all commands PASS and generated API is clean.

**Step 5: Commit**

```bash
git add internal/httpapi/round_handlers.go internal/httpapi/round_handlers_test.go internal/httpapi/router.go internal/httpapi/record_handlers.go internal/httpapi/contract_test.go api/openapi.yaml web/src/api/generated.ts
git commit -m "feat: expose scoped current rounds"
```

### Task 5: Rebase episode progress on the current season round

**Files:**
- Modify: `internal/records/progress.go`
- Modify: `internal/records/round_repository.go`
- Modify: `internal/records/repository.go`
- Modify: `internal/records/progress_test.go`
- Modify: `internal/httpapi/progress_handlers.go`
- Modify: `internal/httpapi/progress_handlers_test.go`
- Modify: `api/openapi.yaml`
- Modify: `web/src/api/generated.ts`

**Step 1: Write the failing progress tests**

Replace media-wide fixtures with explicit season scopes. Cover:

- Season 1 and season 2 maintain independent active rounds.
- Marking the first episode creates round 1 and uses the submitted time.
- `set_time` updates an already watched episode and its linked event atomically.
- `set_time` on an unwatched episode creates the progress/event and marks it watched.
- A future time returns `ErrInvalidWatchedAt` without changing either table.
- Partial/all/undo project the season round to `watching`/`completed`/`none`.
- Archived progress remains queryable through round detail but never appears in current progress.

```go
updated, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
    UserID: userID, MediaID: seriesID, SeasonNumber: 2,
    Action: EpisodeProgressSetTime, WatchedAt: editedAt,
    EpisodeRefs: []EpisodeReference{episode}, TotalEpisodes: 2,
    ExpectedVersion: current.Version, Source: SourceManual,
})
require.NoError(t, err)
require.Equal(t, editedAt, *updated.Episodes[0].WatchedAt)
require.Equal(t, editedAt, eventTimeForEpisode(t, db, episode.SourceID))
```

**Step 2: Run tests to verify RED**

Run: `go test ./internal/records ./internal/httpapi -run 'Test.*(SeasonRoundProgress|EpisodeTime|FutureEpisode)' -count=1 -v`

Expected: FAIL because progress is still media-wide and cannot update watched times.

**Step 3: Implement round-scoped progress**

Add `SeasonNumber int` to `EpisodeProgressInput` and `EpisodeProgressSetTime`. Query only the selected season and current round. Replace all writes to `episode_progress` with `round_episode_progress`.

For `set_time`:

```sql
UPDATE round_episode_progress
SET watched_at = ?, updated_at = strftime('%s','now') * 1000
WHERE round_id = ? AND episode_id = ?;

UPDATE watch_events
SET watched_at = ?
WHERE id = ? AND round_id = ?;
```

Both statements and the round status/version projection must be in one transaction. GET/POST progress handlers require a positive `seasonNumber` query parameter for TV. Return `roundId` and current round `version`.

Update OpenAPI and generated TypeScript for the season query, `set_time` action and round fields.

**Step 4: Run progress verification to verify GREEN**

Run: `go test ./internal/records ./internal/httpapi -run 'Test.*(EpisodeProgress|SeasonRoundProgress|EpisodeTime|FutureEpisode)' -count=1 -v`

Run: `npm --prefix web run api:generate`

Run: `npm --prefix web run api:check`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/records/progress.go internal/records/round_repository.go internal/records/repository.go internal/records/progress_test.go internal/httpapi/progress_handlers.go internal/httpapi/progress_handlers_test.go api/openapi.yaml web/src/api/generated.ts
git commit -m "feat: scope episode progress to viewing rounds"
```

### Task 6: Add atomic rewatch archives and read-only history

**Files:**
- Modify: `internal/records/rounds.go`
- Modify: `internal/records/round_repository.go`
- Modify: `internal/records/rounds_test.go`
- Modify: `internal/httpapi/round_handlers.go`
- Modify: `internal/httpapi/round_handlers_test.go`
- Modify: `internal/httpapi/router.go`
- Modify: `api/openapi.yaml`
- Modify: `web/src/api/generated.ts`

**Step 1: Write the failing archive tests**

Test movie and season rewatch separately. Require:

- Incomplete rounds return `ErrRoundNotCompleted`.
- A completed round becomes archived and immutable.
- The new round number increments and starts at `watching` with empty private fields.
- Season 2 rewatch leaves season 1 unchanged.
- Archived TV detail includes every episode time.
- Repeated idempotency keys return the same new round.
- Injected failure between archive and insert rolls the whole transaction back.

```go
result, err := service.StartRewatch(ctx, RewatchInput{
    Scope: RoundScope{UserID: userID, MediaID: seriesID, SeasonNumber: intPtr(2)},
    ExpectedVersion: completed.Version,
})
require.NoError(t, err)
require.Equal(t, 1, result.Archived.RoundNumber)
require.Equal(t, 2, result.Current.RoundNumber)
require.Equal(t, StatusWatching, result.Current.Status)
require.Nil(t, result.Current.Rating)
```

**Step 2: Run tests to verify RED**

Run: `go test ./internal/records ./internal/httpapi -run 'Test.*(RewatchRound|RoundHistory|ArchivedRound)' -count=1 -v`

Expected: FAIL because archive/list/detail operations do not exist.

**Step 3: Implement archive/list/detail**

Add:

```go
type RoundSummary struct {
    ID, MediaID string
    SeasonNumber *int
    RoundNumber int
    CompletedAt *time.Time
    Rating *int
}

type RoundDetail struct {
    Round WatchRound
    Episodes []Episode
}
```

Implement `StartRewatch`, `RoundHistory` and `RoundDetail`. Archive and insert the next round inside one write transaction. Reject any mutation whose `archived_at` is non-null.

Register:

- `GET /api/v1/records/{mediaID}/rounds?seasonNumber=N`
- `GET /api/v1/records/{mediaID}/rounds/{roundID}`
- `POST /api/v1/records/{mediaID}/rounds/current/rewatch?seasonNumber=N`

The POST requires `If-Match`, CSRF, Origin and idempotency. Return `round_not_completed` and `round_archived` with stable status codes.

**Step 4: Run archive verification to verify GREEN**

Run: `go test ./internal/records ./internal/httpapi -run 'Test.*(RewatchRound|RoundHistory|ArchivedRound)' -count=1 -v`

Run: `npm --prefix web run api:generate`

Run: `npm --prefix web run api:check`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/records/rounds.go internal/records/round_repository.go internal/records/rounds_test.go internal/httpapi/round_handlers.go internal/httpapi/round_handlers_test.go internal/httpapi/router.go api/openapi.yaml web/src/api/generated.ts
git commit -m "feat: archive completed viewing rounds"
```

### Task 7: Migrate catalog, home, calendar, stats and household consumers

**Files:**
- Modify: `internal/records/catalog.go`
- Modify: `internal/records/repository.go`
- Modify: `internal/records/calendar.go`
- Modify: `internal/records/catalog_test.go`
- Modify: `internal/records/calendar_test.go`
- Modify: `internal/stats/repository.go`
- Modify: `internal/stats/service_test.go`
- Modify: `internal/household/repository.go`
- Modify: `internal/household/service.go`
- Modify: `internal/household/service_test.go`
- Modify: `internal/httpapi/record_handlers.go`
- Modify: `internal/httpapi/record_handlers_test.go`
- Modify: `internal/httpapi/household_handlers_test.go`
- Modify: `internal/httpapi/router.go`
- Modify: `api/openapi.yaml`

**Step 1: Write failing consumer tests**

Add tests proving:

- Library/search expose one projected status per media.
- Home/current next episode ignores archived rounds.
- Calendar and stats include events from current and archived rounds exactly once.
- Shared events still include participants.
- Sharing flags are media-level and do not reset on rewatch.
- Visible household rating/review comes from the most recently updated current round only.
- Archived private notes are never returned to another user.
- Flat detail watch-event list/create/delete routes are absent after replacement by rounds.

**Step 2: Run tests to verify RED**

Run: `go test ./internal/records ./internal/stats ./internal/household ./internal/httpapi -run 'Test.*(Projection|ArchivedStats|RoundPrivacy|WatchEventRoutes)' -count=1 -v`

Expected: FAIL because consumers still query `user_media_states` and old event endpoints remain routed.

**Step 3: Migrate all consumers**

Use `user_media_profiles` for catalog and sharing configuration. Use `watch_events.round_id` for calendar/stats. Select current-round private content only when household sharing is enabled.

Remove detail-facing event list/create/delete routes and their OpenAPI operations. Keep internal event repository helpers needed by calendar, stats, sync and round progress. Update record error mapping without exposing storage details.

**Step 4: Run consumer verification to verify GREEN**

Run: `go test ./internal/records ./internal/stats ./internal/household ./internal/httpapi -run 'Test.*(Projection|Calendar|Stats|RoundPrivacy|WatchEventRoutes)' -count=1 -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/records internal/stats internal/household internal/httpapi api/openapi.yaml
git commit -m "feat: consume viewing round projections"
```

### Task 8: Upgrade sync and import/export to round-aware facts

**Files:**
- Modify: `internal/records/export.go`
- Modify: `internal/records/import.go`
- Modify: `internal/records/import_export_test.go`
- Modify: `internal/sync/candidates.go`
- Modify: `internal/sync/candidates_test.go`
- Modify: `internal/sync/provider_runner_test.go`
- Modify: `internal/httpapi/import_export_handlers_test.go`
- Modify: `api/openapi.yaml`
- Modify: `web/src/api/generated.ts`

**Step 1: Write failing round-trip and sync tests**

Define export version 2 and prove a JSON round trip preserves:

- Movie and season round numbers.
- Current versus archived state.
- Rating, note, viewing method and completion time.
- Per-round episode times and event links.
- Media-level tags, collections and sharing settings.

Reject version 1 explicitly because old debug compatibility is out of scope.

For sync, prove movie repeats create a new round after a completed one and episode events enter the correct season current round. Duplicate external event IDs remain idempotent.

**Step 2: Run tests to verify RED**

Run: `go test ./internal/records ./internal/sync ./internal/httpapi -run 'Test.*(RoundTrip|ImportVersion|SyncRound)' -count=1 -v`

Expected: FAIL because export version 1 has state/events/progress without round IDs.

**Step 3: Implement version 2 and sync transaction helpers**

Use this document shape:

```go
type exportRound struct {
    ID string
    SeasonNumber *int
    RoundNumber int
    Status Status
    Rating *int
    Note *string
    ViewingMethod *string
    StartedAt *time.Time
    CompletedAt *time.Time
    ArchivedAt *time.Time
    Version int
    Events []exportEvent
    Episodes []exportRoundEpisode
}
```

Update strict JSON validation and CSV columns (`round_number`, `season_number`). Reuse transaction-level round helpers from sync; do not duplicate raw state/event inserts in `internal/sync`.

Update OpenAPI `ExportDocument.version` to const 2 and regenerate the frontend API.

**Step 4: Run integration verification to verify GREEN**

Run: `go test ./internal/records ./internal/sync ./internal/httpapi -run 'Test.*(RoundTrip|ImportVersion|SyncRound|Import|Export)' -count=1 -v`

Run: `npm --prefix web run api:generate`

Run: `npm --prefix web run api:check`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/records/export.go internal/records/import.go internal/records/import_export_test.go internal/sync internal/httpapi/import_export_handlers_test.go api/openapi.yaml web/src/api/generated.ts
git commit -m "feat: import and sync viewing rounds"
```

### Task 9: Add typed frontend round clients and time conversion helpers

**Files:**
- Modify: `web/src/api/types.ts`
- Modify: `web/src/api/client.ts`
- Modify: `web/src/api/client.test.ts`
- Create: `web/src/lib/dateTime.ts`
- Create: `web/src/lib/dateTime.test.ts`

**Step 1: Write failing client and time tests**

Test movie URLs omit `seasonNumber`, TV URLs include it, writes send `If-Match`/CSRF/idempotency, and list/detail/rewatch decode typed responses.

Test local seconds conversion without hardcoding UTC noon:

```ts
expect(formatLocalSeconds('2026-07-14T12:30:45Z', 'UTC')).toBe('2026-07-14 12:30:45')
expect(toDateTimeLocalValue(new Date('2026-07-14T12:30:45Z'))).toMatch(/T\\d{2}:\\d{2}:\\d{2}$/)
expect(fromDateTimeLocalValue('')).toBeNull()
```

**Step 2: Run tests to verify RED**

Run: `npm --prefix web test -- --run src/api/client.test.ts src/lib/dateTime.test.ts`

Expected: FAIL because round clients and helpers do not exist.

**Step 3: Implement types and clients**

Add `CurrentRound`, `RoundSummary`, `RoundDetail` and `RoundEpisode` types. Add `getCurrentRound`, `updateCurrentRound`, `getRoundHistory`, `getRoundDetail` and `startRewatch`.

Change `getEpisodeProgress` and `updateEpisodeProgress` to require season number. Remove `getWatchEvents`, `createRewatch` and `deleteWatchEvent` from the detail client.

Centralize local/UTC conversion in `dateTime.ts`. Use `Intl.DateTimeFormat` for display and component-level native values for editing; never slice a UTC ISO string to fake local time.

**Step 4: Run tests to verify GREEN**

Run: `npm --prefix web test -- --run src/api/client.test.ts src/lib/dateTime.test.ts`

Run: `npm --prefix web run typecheck`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/api/client.test.ts web/src/lib/dateTime.ts web/src/lib/dateTime.test.ts
git commit -m "feat: add frontend viewing round clients"
```

### Task 10: Replace the media-wide form with a current-round form

**Files:**
- Create: `web/src/features/records/RoundRecordForm.tsx`
- Create: `web/src/features/records/RoundRecordForm.test.tsx`
- Delete: `web/src/features/records/QuickRecordForm.tsx`
- Delete: `web/src/features/records/QuickRecordForm.test.tsx`
- Modify: `web/src/styles/global.css`

**Step 1: Write the failing form tests**

Port the useful save/conflict/draft tests and change the contract to `CurrentRound`. Add:

- Title context is movie “个人记录” or “第 N 季个人记录”.
- Completed movie requires a second-precision local datetime.
- Future local datetime is rejected and retained.
- Rating/note/viewing method are saved to the current round.
- Successful external prop change after rewatch resets the form.
- There is no “再看一次” button inside the form.
- Profile-owned tags/sharing never use the round version.

**Step 2: Run tests to verify RED**

Run: `npm --prefix web test -- --run src/features/records/RoundRecordForm.test.tsx`

Expected: FAIL because `RoundRecordForm` does not exist.

**Step 3: Implement the minimum form**

Reuse React Hook Form and Zod. Accept:

```ts
type RoundRecordFormProps = {
  round: CurrentRound
  now: Date
  participants?: HouseholdMember[]
  onSaved: (round: CurrentRound) => void
}
```

Use `datetime-local` with `step="1"` and a dynamic local `max`. Convert the value through `dateTime.ts` before sending. Keep the existing conflict retry and draft-preservation behavior. Remove the old event-only rewatch mutation and toast.

Keep controls, radii, focus states and 44px targets aligned with the existing product design.

**Step 4: Run form verification to verify GREEN**

Run: `npm --prefix web test -- --run src/features/records/RoundRecordForm.test.tsx`

Run: `npm --prefix web run typecheck`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/features/records/RoundRecordForm.tsx web/src/features/records/RoundRecordForm.test.tsx web/src/features/records/QuickRecordForm.tsx web/src/features/records/QuickRecordForm.test.tsx web/src/styles/global.css
git commit -m "feat: add current round record form"
```

### Task 11: Build the season workspace and editable episode times

**Files:**
- Create: `web/src/features/episodes/SeasonRecordWorkspace.tsx`
- Create: `web/src/features/episodes/SeasonRecordWorkspace.test.tsx`
- Create: `web/src/features/episodes/EpisodeTimeEditor.tsx`
- Create: `web/src/features/episodes/EpisodeTimeEditor.test.tsx`
- Modify: `web/src/features/episodes/EpisodeProgress.tsx`
- Modify: `web/src/features/episodes/EpisodeProgress.test.tsx`
- Modify: `web/src/features/episodes/episodeCatalog.ts`
- Modify: `web/src/features/home/HomePage.tsx`
- Modify: `web/src/features/home/HomePage.test.tsx`
- Modify: `web/src/styles/global.css`

**Step 1: Write failing workspace and episode tests**

Prove:

- The top-level season selector controls progress and “第 N 季个人记录”.
- Switching from season 1 to 2 changes all query keys and preserves season 1 server data.
- Watched rows show `YYYY-MM-DD HH:mm:ss`.
- Clicking an unwatched circle submits the current ISO time.
- Clicking a watched circle submits undo.
- “设置观看时间” on an unwatched episode opens the editor and `set_time` marks it watched.
- Editing a watched episode updates the displayed time.
- Future values show an inline alert and do not send a request.
- Cancel restores the prior value and focus.
- Home next episode uses only the selected/current active season round.

**Step 2: Run tests to verify RED**

Run: `npm --prefix web test -- --run src/features/episodes/SeasonRecordWorkspace.test.tsx src/features/episodes/EpisodeTimeEditor.test.tsx src/features/episodes/EpisodeProgress.test.tsx src/features/home/HomePage.test.tsx`

Expected: FAIL because season ownership is inside `EpisodeProgress` and no time editor exists.

**Step 3: Implement the workspace and stable row layout**

Move season selection to `SeasonRecordWorkspace`. Pass a required `seasonNumber` into `EpisodeProgress` and current-round queries.

Render each episode row with separate controls:

```tsx
<button aria-label={toggleLabel} aria-pressed={episode.watched} />
<span className="episode-code">{label}</span>
<strong>{episode.name}</strong>
<button className="episode-time-button">{timeLabel}</button>
```

Do not keep the entire row as one button. Use tabular numerals and a fixed responsive grid so the time, pending label and editor do not resize adjacent rows. On mobile, place time on a second grid row; preserve 44px targets.

Only disable the target row during a mutation. On success, update the season-scoped progress and current-round query caches.

**Step 4: Run workspace verification to verify GREEN**

Run: `npm --prefix web test -- --run src/features/episodes/SeasonRecordWorkspace.test.tsx src/features/episodes/EpisodeTimeEditor.test.tsx src/features/episodes/EpisodeProgress.test.tsx src/features/home/HomePage.test.tsx`

Run: `npm --prefix web run typecheck`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/features/episodes web/src/features/home web/src/styles/global.css
git commit -m "feat: edit season episode watch times"
```

### Task 12: Replace watch history with the rewatch archive UI

**Files:**
- Create: `web/src/features/records/RewatchSection.tsx`
- Create: `web/src/features/records/RewatchSection.test.tsx`
- Delete: `web/src/features/records/WatchHistory.tsx`
- Delete: `web/src/features/records/WatchHistory.test.tsx`
- Modify: `web/src/features/media/MediaDetailsPage.tsx`
- Modify: `web/src/features/media/MediaDetailsPage.test.tsx`
- Modify: `web/src/styles/global.css`

**Step 1: Write failing archive and details-page tests**

Test:

- Empty text explains that prior rounds will appear after “再刷”.
- The button is named “再刷” and disabled while current round is incomplete.
- Successful rewatch replaces the current round and adds the archived summary.
- Failed rewatch leaves form/progress/history unchanged and shows an inline error.
- History rows show “第 N 刷”, completion seconds and rating summary.
- “查看” opens a Radix dialog with completion time, rating, note and viewing method.
- TV detail shows archived episode labels/times; movie detail does not.
- The details page makes no watch-event request and renders no “观看历史”.
- TV mobile DOM order is season selector, episodes, private record, rewatch section.

**Step 2: Run tests to verify RED**

Run: `npm --prefix web test -- --run src/features/records/RewatchSection.test.tsx src/features/media/MediaDetailsPage.test.tsx`

Expected: FAIL because the page still renders `WatchHistory` and rewatch is inside the old form.

**Step 3: Implement the archive section and page integration**

Use existing Radix Dialog primitives and semantic z-index tokens. Fetch summaries by media/season scope; fetch full detail only when “查看” opens.

On successful rewatch:

```ts
queryClient.setQueryData(currentRoundKey(scope), result.current)
queryClient.setQueryData(roundHistoryKey(scope), result.history)
queryClient.setQueryData(progressKey(scope), emptyProgressFrom(result.current))
```

For movie, reset only the round form. For TV, reset only the selected season caches. Leave profile/tag/collection/sharing queries intact.

Place the season selector above the details grid, keep the desktop sticky private panel, and render rewatch as an unframed full-width section below the grid. Do not introduce nested cards.

**Step 4: Run details verification to verify GREEN**

Run: `npm --prefix web test -- --run src/features/records/RewatchSection.test.tsx src/features/media/MediaDetailsPage.test.tsx`

Run: `npm --prefix web run typecheck`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/features/records web/src/features/media/MediaDetailsPage.tsx web/src/features/media/MediaDetailsPage.test.tsx web/src/styles/global.css
git commit -m "feat: add rewatch archive details"
```

### Task 13: Update E2E data, journeys, accessibility and visual baselines

**Files:**
- Modify: `web/e2e/support.ts`
- Modify: `web/e2e/recording.spec.ts`
- Modify: `web/e2e/episodes.spec.ts`
- Modify: `web/e2e/accessibility.spec.ts`
- Modify: `web/e2e/visual.spec.ts`
- Modify: `web/e2e/visual.spec.ts-snapshots/details-375x812-light.png`
- Modify: `web/e2e/visual.spec.ts-snapshots/details-375x812-dark.png`
- Modify: `web/e2e/visual.spec.ts-snapshots/details-768x1024-light.png`
- Modify: `web/e2e/visual.spec.ts-snapshots/details-768x1024-dark.png`
- Modify: `web/e2e/visual.spec.ts-snapshots/details-1440x900-light.png`
- Modify: `web/e2e/visual.spec.ts-snapshots/details-1440x900-dark.png`
- Modify: `progress.md`
- Modify: `findings.md`
- Modify: `task_plan.md`

**Step 1: Update the E2E seed and write failing journeys**

Change the synthetic import document to version 2 with profiles and empty/current rounds.

Add journeys that:

- Complete a movie at an exact second, click “再刷”, verify the form resets to watching, and open the archived private record.
- Mark every season 2 episode with controlled seconds, save season 2 rating/note, click “再刷”, and verify season 1 remains unchanged.
- Set an unwatched episode time directly and verify it becomes watched.
- Attempt a future time and verify the request is blocked/rejected without data change.
- Exercise time controls and archive dialog entirely by keyboard.

**Step 2: Run journeys to verify RED**

Run: `npm --prefix web run e2e -- --project=chromium --grep 'rewatch|episode time|season record'`

Expected: FAIL because the seed and UI still lack the completed feature until all previous tasks are integrated.

**Step 3: Finish responsive/a11y integration**

Update axe coverage for movie and TV archive dialogs. Assert no horizontal overflow and no sticky/fixed overlap at 375x812, 768x1024 and 1440x900. Verify light/dark themes and reduced motion.

Start the local E2E server through the existing runner; do not reuse an unknown process on an occupied port. Force-update all six details screenshots only after structural assertions pass, then inspect every image for time overflow, modal clipping and mobile order.

**Step 4: Run the complete verification matrix**

Run each command separately and require exit 0:

```bash
gofmt -w internal/records internal/httpapi internal/household internal/stats internal/sync
go test ./internal/storage ./internal/records ./internal/httpapi ./internal/household ./internal/stats ./internal/sync -race -count=1
go test ./... -race -count=1
go vet ./...
npm --prefix web run lint
npm --prefix web run typecheck
npm --prefix web test -- --run
npm --prefix web run api:check
npm --prefix web run build
npm --prefix web audit --audit-level=high
npm --prefix web run e2e
git diff --check
```

Run the repository coverage gate and credential scans using the existing Makefile/scripts. Record exact pass counts, coverage, E2E counts and any tool limitation in `progress.md`.

Expected: all commands PASS, axe has zero blocking violations, details snapshots are current, and no warning/error appears in the browser console.

**Step 5: Commit**

```bash
git add web/e2e web/src api internal findings.md progress.md task_plan.md
git commit -m "test: verify viewing round journeys"
```
