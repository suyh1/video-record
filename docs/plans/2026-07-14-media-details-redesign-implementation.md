# Media Details Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a TMDB-enhanced media details page with live cast and season data, sparse local episode progress, a wide cinematic layout, and collapsed household settings.

**Architecture:** Keep TMDB display data behind the existing authenticated server adapter with a six-hour expiring cache. Expose the non-sensitive TMDB identity on local media DTOs, fetch one season at a time, and persist only episode identities the user actually marks. React merges remote episode catalogs with local progress and reuses that merge for both details and Home “next episode” actions.

**Tech Stack:** Go 1.26, chi, SQLite STRICT migrations, React 19, TypeScript 7, TanStack Query, Vitest/MSW, Playwright, OpenAPI 3.1.

---

## Execution Constraints

- Work directly on `main`; do not create a worktree or feature branch.
- Follow @superpowers:test-driven-development for every behavior change.
- Follow @redesign-existing-projects for the page redesign and preserve the existing design tokens.
- Do not add a frontend or Go dependency.
- Do not persist TMDB cast, season catalogs, episode names, overviews, stills, or air dates as business data.
- Keep existing import, sync, calendar, statistics, and watch-event behavior compatible.
- Commit after each task only after the task-specific tests pass.

### Task 1: Add fresh TMDB TV, season, and credits contracts

**Files:**
- Modify: `internal/integrations/tmdb/types.go`
- Modify: `internal/integrations/tmdb/client.go`
- Test: `internal/integrations/tmdb/client_test.go`

**Step 1: Write the failing client tests**

Add tests that prove TV details decode season summaries, season details decode episode still paths, credits filter/retain cast order, and all three cached responses expire after six hours.

```go
func TestClientFetchesLiveTVSeasonAndCredits(t *testing.T) {
    // Synthetic upstream returns TV seasons, one season's episodes, and ordered cast.
    // Assert /tv/1399, /tv/1399/season/1, and /tv/1399/credits paths and zh-CN.
    // Assert SeasonSummary.EpisodeCount, EpisodeDetails.StillPath,
    // and Credits.Cast[0].Character.
}

func TestLiveDetailsCacheExpiresAfterSixHours(t *testing.T) {
    // Use fakeClock and cache; assert one upstream request before 6h and a second after 6h.
}
```

**Step 2: Run tests to verify RED**

Run: `go test ./internal/integrations/tmdb -run 'TestClientFetchesLiveTVSeasonAndCredits|TestLiveDetailsCacheExpiresAfterSixHours' -count=1`

Expected: FAIL because `SeasonSummary`, `Credits`, `StillPath`, and `Credits()` do not exist and details still use a seven-day TTL.

**Step 3: Implement the minimum client contract**

Add these normalized types and fields:

```go
type SeasonSummary struct {
    ID           int    `json:"id"`
    Name         string `json:"name"`
    Overview     string `json:"overview"`
    PosterPath   string `json:"poster_path"`
    AirDate      string `json:"air_date"`
    SeasonNumber int    `json:"season_number"`
    EpisodeCount int    `json:"episode_count"`
}

type CastMember struct {
    ID          int    `json:"id"`
    Name        string `json:"name"`
    Character   string `json:"character"`
    ProfilePath string `json:"profile_path"`
    Order       int    `json:"order"`
}

type Credits struct { Cast []CastMember `json:"cast"` }
```

Add `Seasons []SeasonSummary` and `EpisodeRunTime []int` to `TVDetails`, `PosterPath/AirDate/Overview` to `SeasonDetails`, and `StillPath` to `EpisodeDetails`. Replace `detailsCacheTTL` with a six-hour live metadata TTL and add:

```go
func (client *Client) Credits(ctx context.Context, mediaType string, id int, language string) (Credits, error)
```

Validate `mediaType` is `movie` or `tv` before building `/movie/{id}/credits` or `/tv/{id}/credits`.

**Step 4: Run tests to verify GREEN**

Run: `go test ./internal/integrations/tmdb -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/integrations/tmdb
git commit -m "feat: add live TMDB catalog contracts"
```

### Task 2: Expose local TMDB identities without expanding snapshots

**Files:**
- Modify: `internal/media/model.go`
- Modify: `internal/media/repository.go`
- Modify: `internal/media/service_test.go`
- Modify: `internal/records/catalog.go`
- Modify: `internal/records/repository.go`
- Modify: `internal/records/catalog_test.go`
- Modify: `internal/httpapi/media_handlers.go`
- Modify: `internal/httpapi/record_handlers.go`
- Test: `internal/httpapi/media_handlers_test.go`
- Test: `internal/httpapi/record_handlers_test.go`

**Step 1: Write failing identity tests**

Add tests proving a TMDB-backed item returns numeric `tmdbId`, a custom unlinked item returns `null`, and local library/search items carry the same optional identity without exposing other source IDs.

```go
require.Equal(t, 1399, *item.TMDBID)
require.Nil(t, custom.TMDBID)
```

Also assert a newly created TMDB item only stores the approved fallback fields: title, original title, release date/year, and poster path; no new cast/season/episode rows are created.

**Step 2: Run tests to verify RED**

Run: `go test ./internal/media ./internal/records ./internal/httpapi -run 'TMDBIdentity|MediaRead|Library' -count=1`

Expected: FAIL because media and catalog DTOs do not expose `tmdbId`.

**Step 3: Implement optional identity reads**

Add `TMDBID *int` to `media.Item` and `records.CatalogItem`. Read it with a `LEFT JOIN media_external_ids ... source = 'tmdb'`, parse the stored text with `strconv.Atoi`, and return an internal error for malformed positive IDs.

Extend only the media/catalog response DTOs:

```go
TMDBID *int `json:"tmdbId"`
```

Keep existing rich fallback columns for backward compatibility, but stop adding any season, episode, or credits persistence to the TMDB create/link path.

**Step 4: Run tests to verify GREEN**

Run: `go test ./internal/media ./internal/records ./internal/httpapi -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/media internal/records internal/httpapi
git commit -m "feat: expose local TMDB identities"
```

### Task 3: Expand authenticated TMDB HTTP responses

**Files:**
- Modify: `internal/httpapi/tmdb_handlers.go`
- Modify: `internal/httpapi/router.go`
- Test: `internal/httpapi/tmdb_handlers_test.go`

**Step 1: Write failing handler tests**

Extend the synthetic TMDB server and assert:

- `/api/v1/tmdb/tv/1399` returns genres, episode runtime, and season summaries.
- `/api/v1/tmdb/tv/1399/season/1` returns season poster/overview/air date and episode still path.
- `/api/v1/tmdb/tv/1399/credits` returns ordered cast.
- `/api/v1/tmdb/person/1399/credits` is not routed or returns a stable invalid media type error.

**Step 2: Run tests to verify RED**

Run: `go test ./internal/httpapi -run 'TestTMDB.*(TV|Season|Credits)' -count=1`

Expected: FAIL because the response fields and credits route are absent.

**Step 3: Implement normalized responses**

Add response DTOs with camelCase JSON. Limit cast to the first 20 ordered entries after stable sorting by `order`; retain entries without profile images so the UI can render a name fallback.

Register:

```go
protected.Get("/tmdb/{mediaType}/{id}/credits", tmdbAPI.credits)
```

Keep all routes behind the existing authenticated group and reuse `writeTMDBError`.

**Step 4: Run tests to verify GREEN**

Run: `go test ./internal/httpapi -run 'TestTMDB' -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/httpapi/tmdb_handlers.go internal/httpapi/tmdb_handlers_test.go internal/httpapi/router.go
git commit -m "feat: serve live TMDB cast and seasons"
```

### Task 4: Persist only episode identities the user marks

**Files:**
- Create: `internal/storage/migrations/0012_episode_identity.sql`
- Modify: `internal/storage/migrate_test.go`
- Modify: `internal/records/progress.go`
- Modify: `internal/records/repository.go`
- Test: `internal/records/progress_test.go`
- Test: `internal/records/calendar_test.go`
- Test: `internal/records/import_export_test.go`

**Step 1: Write failing sparse-progress tests**

Create a TMDB-linked TV item with zero season/episode rows, then submit two references out of a 12-episode remote catalog:

```go
EpisodeRefs: []EpisodeReference{
    {SourceID: "63056", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 1},
    {SourceID: "63057", SeasonNumber: 1, EpisodeNumber: 2, AbsoluteNumber: 2},
},
TotalEpisodes: 12,
```

Assert exactly one season identity and two episode identity rows exist, rich TMDB fields are empty, two watch events exist, the state is `watching` rather than `completed`, calendar absolute numbers are 1 and 2, replay is a no-op, and undo removes only the selected progress/event.

Add a migration test proving existing episode rows receive stable absolute numbers and existing progress survives.

**Step 2: Run tests to verify RED**

Run: `go test ./internal/storage ./internal/records -run 'Sparse|EpisodeIdentity|EpisodeProgress|Calendar' -count=1`

Expected: FAIL because external references and `absolute_number` do not exist.

**Step 3: Add the identity migration**

Add nullable `absolute_number` to `episodes` and backfill existing rows by media/season/episode order. Do not add any rich TMDB columns.

**Step 4: Implement reference-based progress writes**

Add:

```go
type EpisodeReference struct {
    SourceID      string
    SeasonNumber  int
    EpisodeNumber int
    AbsoluteNumber int
}

type EpisodeProgressInput struct {
    // existing fields...
    EpisodeRefs  []EpisodeReference
    TotalEpisodes int
}
```

Validate positive, unique references and a total not smaller than the highest submitted absolute number. In the same SQLite transaction:

1. Find or insert the season identity with empty display fields.
2. Find or insert the episode identity with `source_id`, numbers, empty display fields, and `absolute_number`.
3. Apply the existing watch event/progress mutation.
4. Project status using `max(local identity count, input.TotalEpisodes)`.

Retain the old local-ID selector path when `EpisodeRefs` is empty so import, sync, and legacy callers continue to work.

**Step 5: Run tests to verify GREEN**

Run: `go test ./internal/storage ./internal/records -count=1`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/storage/migrations/0012_episode_identity.sql internal/storage/migrate_test.go internal/records
git commit -m "feat: store sparse external episode progress"
```

### Task 5: Publish the sparse progress and live metadata OpenAPI contract

**Files:**
- Modify: `internal/httpapi/progress_handlers.go`
- Test: `internal/httpapi/progress_handlers_test.go`
- Modify: `api/openapi.yaml`
- Modify: `web/src/api/generated.ts`
- Test: `internal/httpapi/contract_test.go`

**Step 1: Write failing HTTP contract tests**

Post `episodeRefs` to a media item with no catalog rows and assert the response exposes `sourceId`, season/episode/absolute numbers, correct watched count, ETag, CSRF enforcement, and idempotent replay. Assert malformed/duplicate references return `invalid_episode_progress` without inserting rows.

**Step 2: Run tests to verify RED**

Run: `go test ./internal/httpapi -run 'EpisodeProgress|OpenAPI' -count=1`

Expected: FAIL because the request/response/OpenAPI schemas do not include the new fields.

**Step 3: Implement handler DTOs and OpenAPI schemas**

Add:

```yaml
EpisodeReference:
  required: [sourceId, seasonNumber, episodeNumber, absoluteNumber]

UpdateEpisodeProgressRequest:
  properties:
    episodeRefs:
      type: array
      items: { $ref: '#/components/schemas/EpisodeReference' }
    totalEpisodes: { type: integer, minimum: 0 }
```

Extend `MediaItem` and local catalog results with nullable `tmdbId`; extend TMDB TV/season/episode schemas and add `TMDBCredits`. Keep `additionalProperties: false` throughout.

**Step 4: Regenerate TypeScript API types**

Run: `npm --prefix web run api:generate`

Expected: `web/src/api/generated.ts` updates without generator errors.

**Step 5: Run tests to verify GREEN**

Run: `go test ./internal/httpapi -count=1 && npm --prefix web run api:check`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/httpapi api/openapi.yaml web/src/api/generated.ts
git commit -m "feat: publish live episode progress contract"
```

### Task 6: Add typed frontend TMDB queries and episode merge helpers

**Files:**
- Modify: `web/src/api/types.ts`
- Modify: `web/src/api/client.ts`
- Create: `web/src/features/episodes/episodeCatalog.ts`
- Create: `web/src/features/episodes/episodeCatalog.test.ts`
- Test: `web/src/api/client.test.ts`

**Step 1: Write failing helper and client tests**

Test typed TV, season, and credits calls, request cancellation, image-path null handling, merging remote episodes with sparse local progress, defaulting to the last watched or first incomplete season, and locating the next unwatched episode.

```ts
expect(mergeSeason(remoteSeason, progress).episodes[1]?.watched).toBe(true)
expect(selectDefaultSeason(summaries, progress)).toBe(2)
```

**Step 2: Run tests to verify RED**

Run: `npm --prefix web test -- --run src/api/client.test.ts src/features/episodes/episodeCatalog.test.ts`

Expected: FAIL because the live TMDB APIs and merge helpers do not exist.

**Step 3: Implement typed APIs and pure helpers**

Add `TMDBTVDetails`, `TMDBSeasonDetails`, `TMDBCastMember`, and `EpisodeReference` application types. Add `getTMDBMovie`, `getTMDBTV`, `getTMDBSeason`, and `getTMDBCredits` using `requestJSON` and optional `AbortSignal`.

Keep all season-selection and merge behavior in pure functions. Compute watched state by `sourceId`, then season/episode fallback for legacy imports. Compute total episodes from regular season summaries and ignore season 0 specials for progress totals.

**Step 4: Run tests to verify GREEN**

Run: `npm --prefix web test -- --run src/api/client.test.ts src/features/episodes/episodeCatalog.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/api web/src/features/episodes/episodeCatalog.ts web/src/features/episodes/episodeCatalog.test.ts
git commit -m "feat: add live episode catalog client"
```

### Task 7: Rebuild the episode progress interaction around one live season

**Files:**
- Modify: `web/src/features/episodes/EpisodeProgress.tsx`
- Modify: `web/src/features/episodes/EpisodeProgress.test.tsx`
- Modify: `web/src/styles/global.css`

**Step 1: Write failing component tests**

Mock sparse local progress plus TMDB TV/season responses. Assert automatic first-season loading, defaulting to the first incomplete season, switching seasons, direct episode toggles, next episode, range, whole season, retry after a local season fetch error, and no TMDB request for an unlinked custom series.

Assert update bodies contain only selected `episodeRefs`, `totalEpisodes`, version, action, and watched time; they must not contain names, overviews, stills, or actor data.

**Step 2: Run tests to verify RED**

Run: `npm --prefix web test -- --run src/features/episodes/EpisodeProgress.test.tsx`

Expected: FAIL against the existing all-local episode list.

**Step 3: Implement the live-season UI**

Change props to include `tmdbId`. Query TV season summaries and one selected season through stable TanStack Query keys. Render:

- watched/total summary and a semantic progress element;
- a season selector with stable dimensions;
- one season's episode rows with explicit watched text/icon;
- next episode action;
- native `details` for range/whole-season actions;
- per-region loading, empty, error, and retry states.

For `tmdbId === null`, retain a legacy numeric progress fallback from locally stored episodes.

**Step 4: Run tests to verify GREEN**

Run: `npm --prefix web test -- --run src/features/episodes/EpisodeProgress.test.tsx`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/features/episodes web/src/styles/global.css
git commit -m "feat: record progress from live TMDB seasons"
```

### Task 8: Build the cinematic hero, cast strip, and wide details layout

**Files:**
- Create: `web/src/features/media/MediaHero.tsx`
- Create: `web/src/features/media/CastStrip.tsx`
- Modify: `web/src/features/media/MediaDetailsPage.tsx`
- Modify: `web/src/features/media/MediaDetailsPage.test.tsx`
- Modify: `web/src/styles/global.css`

**Step 1: Write failing page tests**

Assert the page:

- enhances local fallback with live backdrop/overview/runtime/genres;
- shows ordered cast names and roles with image/name fallbacks;
- still shows and saves personal records when all TMDB requests fail;
- places episode progress and history in the main column;
- places personal record in the secondary column;
- keeps “家庭与整理” closed initially and reveals tags, collections, sharing, and household ratings after expansion.

**Step 2: Run tests to verify RED**

Run: `npm --prefix web test -- --run src/features/media/MediaDetailsPage.test.tsx`

Expected: FAIL because the hero, cast, two-column layout, and folded settings do not exist.

**Step 3: Implement semantic components**

`MediaHero` uses a real backdrop when present, the local poster, title metadata, personal rating, and overview. It must render as a full-width band rather than a nested card.

`CastStrip` renders up to 20 fixed-aspect repeated cast items in a horizontal scroller. Use `https://image.tmdb.org/t/p/w300{profilePath}` and a text fallback; never use random placeholder imagery.

Recompose `MediaDetailsPage` into:

```tsx
<MediaHero />
<CastStrip />
<div className="media-details-layout">
  <main className="media-details-primary">...</main>
  <aside className="personal-record-panel">...</aside>
</div>
```

Place tags, collections, `RecordSharingEditor`, and `HouseholdSharedRecords` inside one default-closed native details region. Keep the main personal record actions outside it.

**Step 4: Implement responsive CSS**

Use the existing 1440px shell, a `minmax(0, 1fr) minmax(340px, 400px)` desktop grid, stable 2:3 media dimensions, maximum 8px radii, existing OKLCH tokens, and a readable neutral backdrop overlay. At `max-width: 900px`, switch to one column and put personal record before history. Do not add viewport-scaled type, decorative orbs, or a new palette.

**Step 5: Run tests to verify GREEN**

Run: `npm --prefix web test -- --run src/features/media/MediaDetailsPage.test.tsx`

Expected: PASS.

**Step 6: Commit**

```bash
git add web/src/features/media web/src/styles/global.css
git commit -m "feat: redesign the media details page"
```

### Task 9: Preserve Home next-episode actions with live catalogs

**Files:**
- Modify: `web/src/features/home/HomePage.tsx`
- Modify: `web/src/features/home/HomePage.test.tsx`
- Reuse: `web/src/features/episodes/episodeCatalog.ts`

**Step 1: Write the failing Home regression test**

Return a watching series with `tmdbId`, sparse local progress, TV season summaries, and one live season. Assert Home identifies the next unwatched episode, posts its external reference, shows undo, and keeps the local fallback message when TMDB is unavailable.

**Step 2: Run test to verify RED**

Run: `npm --prefix web test -- --run src/features/home/HomePage.test.tsx`

Expected: FAIL because Home still expects the entire local episode catalog.

**Step 3: Reuse the episode catalog helpers**

Load only the selected incomplete season for each visible continuing item, cap the existing strip at eight items, and send the same sparse progress payload as the details page. Do not duplicate merge/selection logic in `HomePage.tsx`.

When TMDB is unavailable, keep the poster/link and replace the direct next button with a quiet “打开详情继续记录” link; never report the series as completed from an incomplete local identity set.

**Step 4: Run tests to verify GREEN**

Run: `npm --prefix web test -- --run src/features/home/HomePage.test.tsx`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/features/home
git commit -m "fix: preserve live next episode actions"
```

### Task 10: Add end-to-end, accessibility, and visual regression coverage

**Files:**
- Modify: `web/scripts/e2e-environment.mjs`
- Modify: `web/e2e/episodes.spec.ts`
- Modify: `web/e2e/recording.spec.ts`
- Modify: `web/e2e/accessibility.spec.ts`
- Modify: `web/e2e/visual.spec.ts`
- Update: `web/e2e/visual.spec.ts-snapshots/*`
- Modify: `progress.md`
- Modify: `findings.md`
- Modify: `task_plan.md`

**Step 1: Extend the synthetic TMDB upstream**

Serve deterministic movie/TV details, credits, two season summaries, and per-season episodes. Record upstream request counts so the test can prove repeat visits within six hours use the cache.

**Step 2: Write failing journeys**

Cover:

- opening an existing TMDB series with no local catalog rows;
- backdrop, cast, season selector, and episode titles appearing;
- marking one episode creates one identity/progress row;
- switching seasons and whole-season recording;
- household settings closed by default;
- TMDB failure leaves personal record saving functional;
- desktop, tablet, and mobile have no horizontal overflow or fixed-element overlap.

**Step 3: Run E2E to verify RED**

Run: `npm --prefix web run e2e`

Expected: FAIL on the newly asserted layout and sparse progress behavior.

**Step 4: Finish minimal integration fixes and update snapshots**

Only change code required by the failing journeys. Record light/dark detail snapshots at 1440x900, 768x1024, and 375x812. Inspect every new snapshot before accepting it.

**Step 5: Run complete frontend verification**

Run:

```bash
npm --prefix web test -- --run
npm --prefix web run typecheck
npm --prefix web run lint
npm --prefix web run api:check
npm --prefix web run build
npm --prefix web run e2e
```

Expected: all commands exit 0 with no warnings or failed tests.

**Step 6: Run complete backend verification**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./... -race -count=1
GOTOOLCHAIN=go1.26.5 go vet ./...
git diff --check
```

Expected: all commands exit 0.

**Step 7: Perform live browser inspection**

Start the development API and Vite server on unused loopback ports. Use the in-app browser at desktop and mobile sizes to verify nonblank images, season switching, progress writes, collapsed settings, no overlap, no horizontal overflow, and no console errors. Stop all temporary processes afterward.

**Step 8: Update tracking files and commit**

Record exact test counts, commands, viewport results, and any resolved failures. Mark phases complete only after fresh evidence exists.

```bash
git add web/e2e web/scripts/e2e-environment.mjs web/e2e/visual.spec.ts-snapshots task_plan.md findings.md progress.md
git commit -m "test: verify rich media details workflow"
```

### Task 11: Final requirement audit

**Files:**
- Review: `docs/plans/2026-07-14-media-details-redesign-design.md`
- Review: `docs/plans/2026-07-14-media-details-redesign-implementation.md`
- Review: all files changed since commit `afd6714`

**Step 1: Audit every accepted requirement**

Confirm with direct code/test evidence:

- full-width backdrop hero and improved poster treatment;
- live cast without business persistence;
- live season/episode catalogs with six-hour freshness;
- only marked episode identities persisted;
- season selection and per-episode progress;
- household controls folded;
- wide desktop and responsive mobile use;
- local records remain usable during TMDB failures.

**Step 2: Run fresh final gates**

Repeat the complete Go, frontend, E2E, OpenAPI, build, lint, and diff checks from Task 10. Do not rely on an earlier run.

**Step 3: Inspect Git state**

Run: `git status --short && git log --oneline -12`

Expected: clean worktree and the task commits on `main`.

**Step 4: Report completion**

Provide the local development URL, important file references, test counts, and any residual external dependency risk. Do not push, tag, publish images, or create a PR unless explicitly requested.
