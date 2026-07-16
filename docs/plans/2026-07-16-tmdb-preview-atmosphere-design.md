# TMDB Preview And Detail Atmosphere Design

## Goal

Opening a TMDB search result must be a read-only preview. A local media item and user record are created only after the user explicitly submits a record action. Media detail pages should use a soft, multi-color gradient derived from the poster across the complete page.

## Current Problem

`ApplicationShell.selectSearchResult` calls `createMediaFromTMDB` before navigating to every TMDB result. That endpoint upserts a row in `media_items`, while local search queries all matching media rows, including rows with no user profile. The next search therefore reports a merely viewed title as a local-library result and removes its deduplicated TMDB result.

The detail hero currently samples one backdrop color and keeps the CSS variable on `MediaHero`. Content below the hero continues to use the global page background, so the atmosphere stops at the hero boundary.

## Chosen Architecture

Add a read-only route at `/tmdb/:mediaType/:tmdbId`. The search dialog navigates TMDB results to this route without a write request. The preview fetches normalized TMDB details and credits through the existing authenticated TMDB endpoints and renders the same metadata, artwork, cast, and atmospheric page shell as a local detail.

The preview presents an explicit record action. The first committed record action imports or refreshes the TMDB metadata, saves the user's chosen status, and then replaces the preview URL with the canonical `/media/:mediaId` route. Merely opening or leaving the preview performs no POST, PUT, or database write. Local search is also constrained to media associated with the current user's profile, so incomplete or failed import attempts cannot appear as library results.

The existing signed same-origin image proxy remains the only path for TMDB images. Preview and local detail requests both consume newly signed proxy URLs returned by the server. Tests cover the preview-to-local transition and ensure a second search still shows TMDB before a record is saved.

## Data Flow

1. Search runs local and TMDB GET requests.
2. Selecting a local result navigates to `/media/:mediaId`.
3. Selecting a TMDB result navigates to `/tmdb/:mediaType/:tmdbId`; no mutation runs.
4. The preview loads TMDB details and credits with GET requests.
5. The user chooses a record status and submits.
6. The client imports the TMDB item, saves the initial record against the returned local ID, invalidates library/search queries, and replaces the route with `/media/:mediaId`.
7. If either write fails, the preview remains visible with the user's selection preserved and an actionable error. A metadata-only row is never exposed by local search.

## Atmospheric Color System

Sample the displayed poster through the existing same-origin image proxy. Cluster usable pixels in OKLab/OKLCH space, discard transparent, near-neutral, near-black, and near-white samples, then select up to three visually distinct colors. Normalize them into restrained page-background tokens while retaining their hue relationships.

`MediaDetailsPage` and `TMDBPreviewPage` own the resulting palette and expose it as CSS custom properties on the page root. The background uses several broad, low-contrast gradients that continue behind the hero, cast, episode, and record sections. Light mode mixes the sampled colors toward the page background; dark mode mixes them toward the dark background. Surfaces remain translucent enough to inherit the atmosphere while text and controls retain existing contrast tokens. If sampling fails or no colorful pixels exist, a neutral brand-derived fallback palette is used without layout changes.

## Error Handling

- Invalid preview route parameters render the existing detail error state without issuing TMDB requests.
- TMDB metadata failure renders a retryable preview error and does not create local state.
- Poster or backdrop failure falls back to the existing placeholders and fallback palette.
- Record-import or record-save failure keeps the preview and form state in place.
- Local search only exposes user-profile-backed media, preventing orphan metadata from becoming visible.

## Verification

- Unit test that selecting a TMDB result navigates to a preview route and sends no import request.
- Unit test that preview metadata and signed images render without local-media requests.
- Unit test that submitting a record imports metadata, saves status, and replaces the route.
- Backend regression test that local search excludes TMDB metadata without a user profile and includes it after a record exists.
- Palette unit tests for three distinct colors, deterministic fallbacks, and blocked canvas access.
- Component tests that palette properties live on the entire detail page, not only the hero.
- Browser verification at desktop and mobile widths in light and dark themes, including image load checks and the search-preview-record sequence.
