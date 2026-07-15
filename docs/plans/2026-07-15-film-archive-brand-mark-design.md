# Film Archive Brand Mark Design

## Goal

Replace the browser's default favicon and the existing clapperboard marks with one distinctive symbol for video-record. The mark should communicate a private film and television archive rather than playback, recording, or media-server administration.

## Concept

The selected concept is the "film archive frame": a compact geometric mark that combines a film frame with an archive drawer. Short vertical perforations at the sides provide the film cue; a centered horizontal slot provides the archive cue. The silhouette stays recognizable without text at 16 pixels.

## Visual System

- The favicon uses a restrained square with 6px-equivalent corner rounding, a deep wine background derived from the existing accent color, and a warm-white archive glyph.
- In-app marks use the same geometry as an inline SVG component. They inherit the surrounding text color so they remain legible in light and dark themes.
- The symbol uses flat fills and strokes only. It has no gradients, shadows, fine texture, lettering, or animation.
- The existing `video-record` wordmark remains unchanged.

## Integration

- Add `web/public/favicon.svg` as the browser icon.
- Reference the icon explicitly from `web/index.html`.
- Add a reusable brand-mark React component so all in-app instances share one SVG definition.
- Replace the sidebar, mobile header, setup, and login clapperboards with the new component.
- Keep the existing 36px bordered container in the sidebar and mobile header; only the glyph changes.

## Accessibility And Failure Behavior

Decorative in-app instances remain hidden from assistive technology, while their surrounding brand containers retain the existing accessible `video-record` label. Browsers that cannot load the SVG favicon will simply omit it; the page title and application remain unaffected.

## Verification

- Add a component test for the shared mark and update existing brand expectations if needed.
- Run frontend tests, type checking, and the production build.
- Inspect the favicon and the in-app mark at desktop and mobile sizes in both the built output and a browser.
- Confirm the 16px favicon remains distinguishable and that no stale clapperboard brand instances remain.
