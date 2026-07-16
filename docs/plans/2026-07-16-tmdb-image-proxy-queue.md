# TMDB Image Proxy Queue Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make a normal detail-page burst load every signed TMDB image while keeping upstream image concurrency and total in-flight proxy work bounded.

**Architecture:** Add a 32-request admission semaphore in front of the existing four-slot upstream semaphore. Admitted requests wait for an upstream slot or their request context; only admission saturation returns the existing 429 contract. A delayed synthetic image upstream gives the real browser enough overlap to reproduce and permanently cover the former random failure.

**Tech Stack:** Go 1.24, Chi, `net/http`, Testify, React/Vite, Playwright, Node synthetic HTTP server.

---

### Task 1: Browser-Level Slow Image Burst Regression

**Files:**
- Modify: `web/scripts/e2e-environment.mjs`
- Modify: `web/e2e/support.ts`
- Modify: `web/e2e/image-proxy.spec.ts`

**Step 1: Add controllable synthetic image latency**

In `startSyntheticTMDB`, hold an `imageDelayMs` value beside `failingIDs`. Let `/__control` update it only when the JSON body contains a finite, non-negative value. Before writing an `/images/` response, await that delay.

```js
let imageDelayMs = 0

if (Number.isFinite(body.imageDelayMs) && body.imageDelayMs >= 0) {
  imageDelayMs = Math.floor(body.imageDelayMs)
}

if (imageDelayMs > 0) {
  await new Promise((resolve) => setTimeout(resolve, imageDelayMs))
}
```

Add a focused support helper without changing existing `controlSyntheticTMDB` callers:

```ts
export async function setSyntheticTMDBImageDelay(page: Page, imageDelayMs: number) {
  const response = await page.request.post(`${syntheticTMDBOrigin}/__control`, {
    data: { imageDelayMs },
  })
  expect(response.ok()).toBeTruthy()
}
```

**Step 2: Write the failing browser regression**

In `image-proxy.spec.ts`, set a 250 ms image delay, log in, then attach a response listener and open `/media/e2e-series`. The page has one backdrop, one poster, and three cast portraits. Poll `.media-details-page img` until all five elements are complete with non-zero natural dimensions, and assert every matching signed image response is 200. Reset the delay in `finally`.

```ts
test('queues a slow detail image burst without dropping poster or cast portraits', async ({ page }) => {
  await setSyntheticTMDBImageDelay(page, 250)
  try {
    await login(page)
    const imageStatuses: number[] = []
    page.on('response', (response) => {
      if (new URL(response.url()).pathname.startsWith('/api/v1/public/tmdb/images/')) {
        imageStatuses.push(response.status())
      }
    })
    await page.goto('/media/e2e-series')
    await expect(page.getByRole('heading', { level: 1, name: '潮汐档案' })).toBeVisible()
    await expect.poll(() => page.locator('.media-details-page img').evaluateAll((images) => ({
      loaded: images.filter((image) => {
        const target = image as HTMLImageElement
        return target.complete && target.naturalWidth > 0 && target.naturalHeight > 0
      }).length,
      total: images.length,
    }))).toEqual({ loaded: 5, total: 5 })
    expect(imageStatuses).toHaveLength(5)
    expect(imageStatuses.every((status) => status === 200)).toBe(true)
  } finally {
    await setSyntheticTMDBImageDelay(page, 0)
  }
})
```

**Step 3: Run the browser test to verify RED**

Run: `npm --prefix web run e2e -- --project=chromium image-proxy.spec.ts`

Expected: FAIL because at least one of the five overlapping images receives 429 and becomes a placeholder.

**Step 4: Commit the test harness and RED regression**

Do not commit a failing tree yet; keep these test-only changes for the backend GREEN commit.

### Task 2: Backend Queue Semantics Tests

**Files:**
- Modify: `internal/httpapi/public_tmdb_handlers_test.go`

**Step 1: Replace the immediate-rejection expectation**

Rename the old concurrency test to describe waiting. Give its handlers `imageSlots` capacity 1 and `imageRequests` capacity 2. Hold the first upstream request, start the second handler in a goroutine, wait until both requests are admitted, and assert the second handler has not returned. Release the first request, then require both responses to be 200 and both channels to be empty.

**Step 2: Add admission saturation coverage**

Construct handlers with both channels at capacity 1. Hold the first request active, issue a second request, and retain the old assertions:

```go
require.Equal(t, http.StatusTooManyRequests, secondResponse.Code)
require.Equal(t, "1", secondResponse.Header().Get("Retry-After"))
require.Contains(t, secondResponse.Body.String(), `"code":"tmdb_image_busy"`)
```

**Step 3: Add queued cancellation coverage**

Fill the active slot, start one admitted request with a cancelable context, wait until `len(handlers.imageRequests) == 1`, cancel it, and require the handler to return without an upstream request. Verify the admission channel is empty after cancellation.

**Step 4: Run backend tests to verify RED**

Run: `go test ./internal/httpapi -run 'TestPublicTMDBImage.*(Wait|Queue|Admission)' -count=1`

Expected: FAIL because `publicTMDBHandlers` has no `imageRequests` field and the second request still returns immediately.

### Task 3: Bounded Server Queue Implementation

**Files:**
- Modify: `internal/httpapi/public_tmdb_handlers.go`
- Modify: `internal/httpapi/router.go`
- Test: `internal/httpapi/public_tmdb_handlers_test.go`
- Test: `web/e2e/image-proxy.spec.ts`

**Step 1: Add the total in-flight limit**

```go
const (
	publicTMDBImageConcurrency = 4
	publicTMDBImageRequestLimit = 32
)

type publicTMDBHandlers struct {
	client        *tmdb.Client
	now           func() time.Time
	imageSlots    chan struct{}
	imageRequests chan struct{}
}
```

Initialize both channels in `NewRouter`.

**Step 2: Admit, wait, and release in the handler**

First admit non-blockingly to `imageRequests`; return the existing 429 only if it is full. Then block on `imageSlots` or request cancellation. If cancellation wins while queued, release `imageRequests` before returning. After successful acquisition, defer release of both semaphores around `client.Image`.

```go
if !handlers.acquireImageSlot(w, r) {
	return
}
defer func() {
	<-handlers.imageSlots
	<-handlers.imageRequests
}()
```

**Step 3: Run the backend queue tests to verify GREEN**

Run: `go test ./internal/httpapi -run 'TestPublicTMDBImage' -count=1`

Expected: PASS.

**Step 4: Run the delayed browser regression to verify GREEN**

Run: `npm --prefix web run e2e -- --project=chromium image-proxy.spec.ts`

Expected: PASS with five signed image responses, all status 200.

**Step 5: Commit**

```bash
git add internal/httpapi/public_tmdb_handlers.go internal/httpapi/public_tmdb_handlers_test.go internal/httpapi/router.go web/scripts/e2e-environment.mjs web/e2e/support.ts web/e2e/image-proxy.spec.ts
git commit -m "fix: queue concurrent TMDB image requests"
```

### Task 4: Full Verification and Real Browser Refreshes

**Files:**
- Modify: `task_plan.md`
- Modify: `findings.md`
- Modify: `progress.md`

**Step 1: Run static and unit gates**

Run in parallel:

- `go test ./... -race -count=1`
- `go vet ./...`
- `npm --prefix web test -- --run`
- `npm --prefix web run lint`
- `npm --prefix web run build`
- `npm --prefix web run api:check`

Expected: all exit 0.

**Step 2: Run the standard isolated browser suite**

Run: `npm --prefix web run e2e`

Expected: all Playwright tests pass with no snapshot changes.

**Step 3: Verify the running app repeatedly**

Start a fresh backend and Vite server against the synthetic TMDB environment. In the in-app browser, open the series detail page and perform at least five reloads. After each reload assert:

- exactly five detail images are present and decoded;
- poster and three cast portraits do not become placeholders;
- signed image responses have no 429;
- console warning/error list is empty.

**Step 4: Update tracking files and commit**

Mark the 2026-07-16 image queue phases complete and record the RED/GREEN and browser evidence.

```bash
git add task_plan.md findings.md progress.md docs/plans/2026-07-16-tmdb-image-proxy-queue.md
git commit -m "docs: complete TMDB image queue verification"
```
