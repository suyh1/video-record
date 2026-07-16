# TMDB Preview And Detail Atmosphere Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make TMDB detail browsing read-only until an explicit record action, keep unrecorded metadata out of local search, and apply a poster-derived multi-color atmosphere to the complete detail page.

**Architecture:** TMDB results navigate to a GET-only preview route that reuses normalized TMDB detail, cast, artwork, and detail presentation components. A deliberate record action imports the media and redirects to its canonical local detail; local search independently requires a current-user profile. Poster sampling returns a deterministic three-color OKLCH palette which is lifted to the page root as CSS variables.

**Tech Stack:** Go 1.26, SQLite, React 19, TypeScript, TanStack Query, React Router, Vitest, Testing Library, Playwright, CSS Color 5 `color-mix()`.

---

### Task 1: Restrict Local Search To The Current User's Library

**Files:**
- Modify: `internal/records/catalog_test.go`
- Modify: `internal/records/repository.go:229`

**Step 1: Write the failing test**

Extend the catalog isolation test to insert or upsert a TMDB-backed `media_items` row with no `user_media_profiles` row. Assert `SearchMedia(userID, title)` returns no match. Then create a round/profile for that user and assert the same query returns exactly that media ID. Preserve the existing assertion that another user's private catalog is not visible.

**Step 2: Run the focused test and verify RED**

Run: `go test ./internal/records -run 'TestCatalog.*Search' -count=1`

Expected: FAIL because `SearchMedia` currently uses a `LEFT JOIN` and returns metadata-only rows.

**Step 3: Implement the minimal query change**

Change `SearchMedia` to begin from `user_media_profiles profile`, join `media_items media` for the current user, and keep the TMDB identity join:

```sql
FROM user_media_profiles profile
JOIN media_items media ON media.id = profile.media_id
LEFT JOIN media_external_ids tmdb ON tmdb.media_id = media.id AND tmdb.source = 'tmdb'
WHERE profile.user_id = ?
  AND (
    COALESCE(media.custom_title, media.external_title) LIKE ? ESCAPE '\'
    OR media.original_title LIKE ? ESCAPE '\'
  )
```

Keep exact-title ordering and the 20-row limit.

**Step 4: Verify GREEN**

Run: `go test ./internal/records -run 'TestCatalog' -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/records/catalog_test.go internal/records/repository.go
git commit -m "fix: exclude unrecorded metadata from local search"
```

### Task 2: Route TMDB Results To A Read-Only Preview

**Files:**
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/app/App.tsx`
- Create: `web/src/features/media/TMDBPreviewPage.tsx`
- Create: `web/src/features/media/TMDBPreviewPage.test.tsx`

**Step 1: Write the failing navigation test**

Add an app test whose search handlers return one TMDB TV result. Track requests to `POST /api/v1/media/tmdb/:mediaType/:externalID`, select the result, and assert:

```ts
expect(window.location.pathname).toBe('/tmdb/tv/12345')
expect(importRequests).toBe(0)
```

**Step 2: Verify RED**

Run: `npm --prefix web test -- --run src/app/App.test.tsx`

Expected: FAIL because selection currently posts and navigates to `/media/:id`.

**Step 3: Add the preview route and navigation**

Remove `createMediaFromTMDB` from `ApplicationShell.selectSearchResult`. For TMDB results, close search and navigate to `/tmdb/${item.mediaType}/${item.externalId}`. Register `/tmdb/:mediaType/:tmdbId` with `TMDBPreviewPage`, and treat that route as immersive in the header logic.

**Step 4: Write the failing preview rendering tests**

Cover movie and TV parameters. Mock the existing GET detail and credits endpoints with signed poster/backdrop/profile URLs. Assert the preview renders title, overview, artwork, cast, a clear preview label, and an explicit record action. Assert no local `GET /media/:id`, record GET, or write request occurs on mount. Cover invalid media type/ID and a retryable TMDB failure.

**Step 5: Verify RED**

Run: `npm --prefix web test -- --run src/features/media/TMDBPreviewPage.test.tsx`

Expected: FAIL because the page does not exist.

**Step 6: Implement the read-only page**

Parse `mediaType` as `movie | tv` and `tmdbId` as a positive integer before enabling queries. Load `getTMDBMovie` or `getTMDBTV` plus `getTMDBCredits`. Normalize the response into the existing `MediaDetails`/`RecordState` presentation shape, use `MediaHero` and `CastStrip`, and render a compact preview record action below the cast. Do not call any mutation during render, query success, navigation, or unmount.

**Step 7: Verify GREEN**

Run: `npm --prefix web test -- --run src/app/App.test.tsx src/features/media/TMDBPreviewPage.test.tsx`

Expected: PASS.

**Step 8: Commit**

```bash
git add web/src/app/App.tsx web/src/app/App.test.tsx web/src/features/media/TMDBPreviewPage.tsx web/src/features/media/TMDBPreviewPage.test.tsx
git commit -m "feat: add read-only TMDB detail previews"
```

### Task 3: Materialize A Preview Only From An Explicit Record Action

**Files:**
- Modify: `web/src/features/media/TMDBPreviewPage.tsx`
- Modify: `web/src/features/media/TMDBPreviewPage.test.tsx`
- Modify: `web/src/api/client.test.ts`

**Step 1: Write the failing record-action tests**

For a movie preview, choose a non-`none` status and submit. Assert the client first posts the TMDB import, then writes the initial current round with `If-Match: "0"`, invalidates media-search/library queries, and replaces the URL with `/media/:returnedId`. For TV, assert the explicit continue-to-record action imports and redirects without running automatically on page load. Add a failure case that preserves the preview and reports an error.

**Step 2: Verify RED**

Run: `npm --prefix web test -- --run src/features/media/TMDBPreviewPage.test.tsx src/api/client.test.ts`

Expected: FAIL because the preview action is not wired.

**Step 3: Implement the mutation flow**

Build the `MediaSearchResult` identity from the route and TMDB response. Movie submission calls `createMediaFromTMDB`, then `updateCurrentRound(returned.id, undefined, 0, payload)`. TV's explicit record action calls `createMediaFromTMDB` and redirects to the local seasonal workspace. In both cases invalidate `['media-search']` and `['library']`, then navigate with `{ replace: true }`. Disable duplicate submissions and retain an accessible error without discarding the selected status.

**Step 4: Verify GREEN**

Run: `npm --prefix web test -- --run src/features/media/TMDBPreviewPage.test.tsx src/api/client.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/features/media/TMDBPreviewPage.tsx web/src/features/media/TMDBPreviewPage.test.tsx web/src/api/client.test.ts
git commit -m "feat: create local media from explicit preview records"
```

### Task 4: Extract A Three-Color Poster Palette

**Files:**
- Modify: `web/src/lib/mediaAccent.test.ts`
- Modify: `web/src/lib/mediaAccent.ts`

**Step 1: Write failing palette tests**

Add `selectMediaPalette` tests with repeated red, blue, and green clusters. Assert the result is deterministic, contains three distinct OKLCH colors, and rejects neutral/transparent/extreme pixels. Add `sampleMediaPalette` canvas tests and retain the existing single-accent API for the home hero by deriving it from the first palette color.

**Step 2: Verify RED**

Run: `npm --prefix web test -- --run src/lib/mediaAccent.test.ts`

Expected: FAIL because the palette API does not exist.

**Step 3: Implement palette clustering**

Convert usable pixels to OKLab, quantize into perceptual buckets, rank buckets by population and chroma, then greedily choose up to three colors separated by a minimum OKLab distance. Normalize output lightness/chroma to stable atmospheric ranges. Fill missing slots by deterministic hue offsets from the strongest color; return `null` only when there is no usable chromatic sample. Keep `sampleMediaAccent(image)` as `sampleMediaPalette(image)?.colors[0] ?? null`.

**Step 4: Verify GREEN**

Run: `npm --prefix web test -- --run src/lib/mediaAccent.test.ts src/features/home/HomeHero.test.tsx`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/lib/mediaAccent.ts web/src/lib/mediaAccent.test.ts
git commit -m "feat: derive multi-color media palettes"
```

### Task 5: Apply The Palette Across The Entire Detail Page

**Files:**
- Modify: `web/src/features/media/MediaPoster.tsx`
- Modify: `web/src/features/media/MediaHero.tsx`
- Modify: `web/src/features/media/MediaDetailsPage.tsx`
- Modify: `web/src/features/media/TMDBPreviewPage.tsx`
- Modify: `web/src/features/media/MediaImages.test.tsx`
- Modify: `web/src/features/media/MediaDetailsPage.test.tsx`
- Modify: `web/src/styles/global.css`

**Step 1: Write failing component tests**

Mock `sampleMediaPalette`, fire the poster image load event, and assert the root `.media-atmosphere-page` receives `--media-accent`, `--media-atmosphere-1`, `--media-atmosphere-2`, and `--media-atmosphere-3`. Assert the hero inherits the accent rather than owning the only color variable. Cover poster failure and route changes resetting to fallback variables.

**Step 2: Verify RED**

Run: `npm --prefix web test -- --run src/features/media/MediaImages.test.tsx src/features/media/MediaDetailsPage.test.tsx src/features/media/TMDBPreviewPage.test.tsx`

Expected: FAIL because sampling currently happens from the backdrop and remains local to the hero.

**Step 3: Lift poster sampling to the page**

Allow `MediaPoster` to report successful artwork loads. `MediaHero` samples that poster with `sampleMediaPalette` and reports palette changes through a callback. Both local and preview pages own palette state, apply the four custom properties to their root, and reset on media identity changes or poster failure. Keep backdrop readiness independent from palette readiness.

**Step 4: Add full-page atmospheric CSS**

Use `.media-atmosphere-page` for both routes. Expand it through the immersive main-content padding and add broad layered gradients using the three sampled variables mixed toward `var(--bg)`. Define dark-theme mixes separately, keep section backgrounds transparent, and make raised forms use translucent existing surfaces. Preserve text tokens and focus contrast. Add mobile margin/padding adjustments matching the existing hero breakpoints.

**Step 5: Verify GREEN**

Run: `npm --prefix web test -- --run src/features/media src/lib/mediaAccent.test.ts`

Expected: PASS.

**Step 6: Commit**

```bash
git add web/src/features/media web/src/styles/global.css
git commit -m "style: extend poster atmosphere across media details"
```

### Task 6: Contract, Regression, And Browser Verification

**Files:**
- Modify if needed: `web/e2e/visual.spec.ts`
- Modify if needed: `web/e2e/image-proxy.spec.ts`
- Modify: `docs/plans/2026-07-16-tmdb-preview-atmosphere-design.md` only if implementation details changed materially

**Step 1: Run focused suites**

```bash
go test ./internal/records ./internal/httpapi -count=1
npm --prefix web test -- --run src/app/App.test.tsx src/features/search/SearchDialog.test.tsx src/features/media src/lib/mediaAccent.test.ts
```

Expected: PASS.

**Step 2: Run complete static and unit verification**

```bash
go test ./... -count=1
go vet ./...
npm --prefix web run lint
npm --prefix web run typecheck
npm --prefix web run build
```

Expected: every command exits 0 with no warnings promoted by lint.

**Step 3: Verify in the browser**

Start the backend and Vite server with the existing local configuration. In the in-app browser, verify desktop `1440x900` and mobile `375x812`, light and dark themes:

- Search a TMDB-only title and open it.
- Confirm the route is `/tmdb/...`, artwork loads, and no local result appears after closing and searching again.
- Return to preview, submit a record action, and confirm redirect to `/media/...` plus a local search result.
- Confirm poster, backdrop, and cast proxy requests return image bytes rather than placeholders.
- Confirm the three-color atmosphere continues behind cast and record/season content to the bottom of the page without text overlap.

Capture screenshots and inspect console/network errors.

**Step 4: Run end-to-end tests**

Run: `npm --prefix web run e2e`

Expected: PASS.

**Step 5: Commit final regression coverage**

```bash
git add web/e2e docs/plans/2026-07-16-tmdb-preview-atmosphere-design.md
git commit -m "test: cover TMDB preview and page atmosphere"
```
