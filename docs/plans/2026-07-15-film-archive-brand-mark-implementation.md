# Film Archive Brand Mark Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the default browser icon and every clapperboard brand mark with one responsive film-archive symbol.

**Architecture:** Define the in-app mark once as a presentational React SVG component, then reuse it in the application shell and authentication pages. Keep a standalone filled SVG in Vite's public directory for the favicon because browser chrome needs stronger contrast and cannot inherit application theme tokens.

**Tech Stack:** React 19, TypeScript, Vite, inline SVG, Vitest, Testing Library, Playwright browser verification

---

### Task 1: Shared Brand Mark Component

**Files:**
- Create: `web/src/app/BrandMark.test.tsx`
- Create: `web/src/app/BrandMark.tsx`

**Step 1: Write the failing test**

Render `<BrandMark />` and assert that the shared symbol exposes the stable `data-brand-mark="film-archive"` hook, is hidden from assistive technology by default, and accepts the requested size.

**Step 2: Run the focused test to verify it fails**

Run: `npm --prefix web test -- --run src/app/BrandMark.test.tsx`

Expected: FAIL because `BrandMark.tsx` does not exist.

**Step 3: Implement the minimal component**

Create a 24 by 24 inline SVG using the approved film-frame/archive-drawer geometry. Use `currentColor`, a fixed `viewBox`, rounded joins, and no generated IDs so repeated instances remain valid.

**Step 4: Run the focused test to verify it passes**

Run: `npm --prefix web test -- --run src/app/BrandMark.test.tsx`

Expected: PASS.

### Task 2: Browser Favicon

**Files:**
- Create: `web/public/favicon.svg`
- Modify: `web/index.html`

**Step 1: Add the standalone icon**

Create the approved filled variant with a deep wine background and warm-white glyph. Keep the same geometry as the component and optimize it for 16px rendering.

**Step 2: Reference the favicon explicitly**

Add `<link rel="icon" type="image/svg+xml" href="/favicon.svg" />` in the document head.

**Step 3: Verify the production asset path**

Run: `npm --prefix web run build`

Expected: PASS and `web/dist/favicon.svg` exists with the reference preserved in `web/dist/index.html`.

### Task 3: Replace In-App Brand Marks

**Files:**
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/features/auth/AuthGate.tsx`
- Test: `web/src/app/App.test.tsx`
- Test: `web/src/features/auth/AuthGate.test.tsx`

**Step 1: Extend the existing tests**

Assert that the authenticated shell and unauthenticated page contain the shared film-archive mark. Run both test files and confirm the assertions fail before integration.

**Step 2: Replace the old imports and instances**

Remove `Clapperboard` from both Lucide import lists, import `BrandMark`, and render it in the sidebar, mobile header, setup, and login brand positions. Preserve existing accessible labels and decorative SVG behavior.

**Step 3: Run the focused tests**

Run: `npm --prefix web test -- --run src/app/App.test.tsx src/features/auth/AuthGate.test.tsx`

Expected: PASS.

**Step 4: Confirm no stale brand glyph remains**

Run: `rg -n "Clapperboard" web/src`

Expected: no matches.

### Task 4: Full Verification

**Files:**
- Verify only; no planned source changes

**Step 1: Run automated checks**

Run: `npm --prefix web test -- --run`

Expected: PASS.

Run: `npm --prefix web run typecheck`

Expected: PASS.

Run: `npm --prefix web run build`

Expected: PASS.

**Step 2: Inspect browser rendering**

Start the Vite development server, open the project in the in-app browser, and inspect desktop and mobile viewports. Confirm the favicon request succeeds, the icon is visible in browser chrome where supported, the sidebar and mobile marks remain centered, and authentication uses the same geometry.

**Step 3: Review the final diff**

Run: `git diff --check` and `git status --short`.

Expected: no whitespace errors and only the planned brand asset, component, integration, tests, and plan changes are present.
