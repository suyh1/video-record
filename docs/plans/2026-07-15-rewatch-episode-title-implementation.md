# Rewatch Episode Title Fix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show the active TMDB season episode title in archived rewatch details when the sparse local episode name is empty.

**Architecture:** Keep archived times and private round data unchanged. Pass the active season's already queried TMDB episode catalog from `SeasonRecordWorkspace` into `RewatchSection`, then resolve display names by season and episode number with the local archived name and “未命名” as fallbacks.

**Tech Stack:** React 19, TypeScript, TanStack Query, Vitest, Testing Library, MSW.

---

### Task 1: Merge catalog titles in the archive dialog

**Files:**
- Modify: `web/src/features/records/RewatchSection.test.tsx:79`
- Modify: `web/src/features/records/RewatchSection.tsx:15`

**Step 1: Write the failing component test**

Change the season archive fixture so the API returns `name: ''`, pass a matching catalog entry, and assert the dialog shows `重逢` without `未命名`:

```tsx
renderWithQueryClient(
  <RewatchSection
    round={seasonRound}
    episodeCatalog={[{ seasonNumber: 2, episodeNumber: 1, name: '重逢' }]}
  />,
)

expect(dialog).toHaveTextContent('重逢')
expect(dialog).not.toHaveTextContent('未命名')
```

**Step 2: Run the test to verify RED**

Run:

```bash
npm --prefix web test -- --run src/features/records/RewatchSection.test.tsx
```

Expected: the archive dialog still renders `未命名`, so the new title assertion fails.

**Step 3: Implement the minimal display merge**

Add a narrow catalog prop using the existing TMDB episode type:

```tsx
type EpisodeTitle = Pick<TMDBEpisodeDetails, 'seasonNumber' | 'episodeNumber' | 'name'>

type RewatchSectionProps = {
  round: CurrentRound
  episodeCatalog?: EpisodeTitle[]
  onRewatched?: (round: CurrentRound) => void
}
```

Build a lookup keyed by season and episode number, then render:

```tsx
const catalogName = episodeCatalog.find((item) =>
  item.seasonNumber === episode.seasonNumber &&
  item.episodeNumber === episode.episodeNumber,
)?.name

<span>{catalogName || episode.name || '未命名'}</span>
```

Keep movie behavior unchanged by defaulting `episodeCatalog` to an empty array.

**Step 4: Run the component test to verify GREEN**

Run the same Vitest command. Expected: all `RewatchSection` tests pass.

### Task 2: Wire the active season catalog into rewatch details

**Files:**
- Modify: `web/src/features/episodes/SeasonRecordWorkspace.test.tsx`
- Modify: `web/src/features/episodes/SeasonRecordWorkspace.tsx:5`

**Step 1: Write the failing workspace integration test**

Add archived round handlers for season 1 whose detail returns an empty local episode name. Reuse the existing TMDB season handler, open “查看第 1 刷”, and assert the dialog shows `第 1 季第一集` instead of `未命名`.

**Step 2: Run the test to verify RED**

Run:

```bash
npm --prefix web test -- --run src/features/episodes/SeasonRecordWorkspace.test.tsx
```

Expected: the current list has the TMDB title but the archive dialog still shows `未命名`.

**Step 3: Pass the active season catalog**

In `SeasonRecordWorkspace`, query `getTMDBSeason` with the same query key and stale time already used by `EpisodeProgress`:

```tsx
const season = useQuery({
  queryKey: ['tmdb-season', tmdbId, activeSeason],
  queryFn: ({ signal }) => getTMDBSeason(tmdbId ?? 0, activeSeason ?? 0, signal),
  enabled: tmdbId !== null && activeSeason !== null,
  staleTime: 30_000,
})
```

TanStack Query shares this request and cached data with `EpisodeProgress`. Pass the catalog into the active season's rewatch section:

```tsx
<RewatchSection
  round={activeRound.data}
  episodeCatalog={season.data?.episodes ?? []}
  onRewatched={...}
/>
```

**Step 4: Run focused tests to verify GREEN**

Run:

```bash
npm --prefix web test -- --run src/features/episodes/SeasonRecordWorkspace.test.tsx src/features/records/RewatchSection.test.tsx
```

Expected: both test files pass.

### Task 3: Verify and commit

**Files:**
- Modify: `task_plan.md`
- Modify: `findings.md`
- Modify: `progress.md`

**Step 1: Run the frontend gates**

```bash
npm --prefix web test -- --run
npm --prefix web run typecheck
npm --prefix web run lint
npm --prefix web run build
git diff --check
```

Expected: every command exits 0 with no failed tests, type errors, lint warnings, build errors, or whitespace errors.

**Step 2: Verify the original browser symptom**

Open a TV season with one archived round, click “查看”, and confirm every archived row uses the same episode title as the current season list while retaining its archived second-precision watched time.

**Step 3: Update tracking files**

Mark all five phases complete and record the red/green evidence and verification results.

**Step 4: Commit the implementation**

```bash
git add web/src/features/records/RewatchSection.tsx \
  web/src/features/records/RewatchSection.test.tsx \
  web/src/features/episodes/SeasonRecordWorkspace.tsx \
  web/src/features/episodes/SeasonRecordWorkspace.test.tsx \
  task_plan.md findings.md progress.md
git commit -m "fix: show episode titles in rewatch details"
```
