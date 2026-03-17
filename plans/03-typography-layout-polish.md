# Typography & Layout Polish

## Overview

Four related improvements to readability and mobile usability: increase the base font size from 14px to 16px, darken dim/muted text colours for WCAG AA compliance at small sizes, allow homepage task titles to wrap instead of truncate, and consolidate tracker action buttons behind a disclosure menu on mobile screens.

## Current State

**Problems Identified:**
- Body font is 14px with a monospace stack, making extended reading uncomfortable on desktop. Metadata text at `0.7rem` (9.8px at 14px base) falls below the WCAG minimum of 12px for readable text.
- `--fg-dim` (`#6c6f85` light / `#a6adc8` dark) and `--fg-muted` (`#7c7f93` light / `#8e92a8` dark) have marginal contrast ratios against their backgrounds, particularly for small text where WCAG AA requires 4.5:1.
- Homepage task titles use `white-space: nowrap` with `text-overflow: ellipsis`, hiding important context.
- On mobile, each tracker item's action area renders priority select + apply, delete, and move buttons in a wrapping row, creating a dense wall of controls.

**Technical Context:**
- Single CSS file: `web/static/theme.css` (1126 lines). Catppuccin Latte (light) and Mocha (dark) colour themes.
- Templates: Go `html/template` with htmx. Relevant: `tracker.html`, `goals.html`, `homepage.html`.
- Mobile breakpoint is `768px`. Touch targets already enforced at `2.75rem` (44px).

## Requirements

**Functional Requirements:**
1. Body font size MUST increase from 14px to 16px.
2. All text rendered with `--fg-dim` or `--fg-muted` MUST meet WCAG AA contrast ratio (4.5:1) against `--bg` in both light and dark themes.
3. Homepage task titles MUST allow wrapping to a maximum of 2 lines, with ellipsis on overflow beyond that.
4. Tracker item actions MUST collapse into a `<details>` disclosure element on screens <= 768px.

**Technical Constraints:**
1. Solution MUST NOT require any Go code changes or new template functions.
2. Solution MUST NOT introduce JavaScript dependencies; the mobile menu MUST use native HTML `<details>`.
3. Both light and dark themes MUST be updated together for any colour change.
4. Solution MUST NOT break the existing 44px minimum touch targets on mobile.

## Success Criteria

1. `body { font-size: 16px }` is set and all relative sizes scale proportionally.
2. Contrast ratio of `--fg-dim` against `--bg` is >= 4.5:1 in both themes.
3. Homepage task titles wrap to at most 2 lines with ellipsis beyond that.
4. On viewports <= 768px, tracker item actions are hidden behind a "more" disclosure element. On desktop, actions render inline as today.
5. All tests pass. Build succeeds.

---

## Development Plan

### Phase 1: Font Size and Contrast Adjustments

- [ ] Change `body { font-size: 14px }` to `body { font-size: 16px }` in `web/static/theme.css`
- [ ] In the light theme, darken `--fg-dim` from `var(--subtext0)` (`#6c6f85`) to `var(--subtext1)` (`#5c5f77`) and darken `--fg-muted` from `#7c7f93` to approximately `#6c6f85`. Verify contrast >= 4.5:1 against `--bg` (`#eff1f5`)
- [ ] In the dark theme, verify `--fg-dim` (`#a6adc8` on `#1e1e2e`) meets 4.5:1. If not, shift to `--subtext1` (`#bac2de`). Darken `--fg-muted` from `#8e92a8` to approximately `#9ea2b8` and verify >= 4.5:1 against `#1e1e2e`
- [ ] Audit all elements using `font-size: 0.7rem`. At 16px base, `0.7rem` = 11.2px. Confirm acceptable; bump to `0.75rem` if any element feels too small
- [ ] Check the iOS zoom prevention rule (`font-size: 16px` on `.form-input` at 768px breakpoint). Now redundant but harmless -- leave for safety
- [ ] Review nav at 16px: verify `.nav-brand`, `.nav-links`, and `.nav-user` still fit on 375px viewport
- [ ] Visually verify dim metadata text is legible in both themes
- [ ] Run `go test ./...`
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 2: Homepage Card Truncation

- [ ] In `theme.css`, replace the `.homepage-task` truncation rules with a 2-line clamp:
  - Remove `overflow: hidden; text-overflow: ellipsis; white-space: nowrap`
  - Add `display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden`
- [ ] Verify links inside the clamped container still receive clicks
- [ ] Confirm the homepage grid layout still looks balanced with multi-line titles
- [ ] Test on both desktop and mobile (375px)
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 3: Mobile Action Button Consolidation

- [ ] In `web/templates/tracker.html`, wrap `.tracker-item-actions` content in a `<details class="item-actions-menu" open>` / `<summary class="action-btn item-actions-toggle">more</summary>` element. Note the `open` attribute is set by default so the actions are visible.
- [ ] Apply the same pattern in `web/templates/goals.html` for goal action buttons (but NOT +1/-1/set progress forms which are primary actions)
- [ ] Add CSS. **Do NOT use `display: contents`** on `<details>` -- it has known accessibility issues in Safari where it strips semantic meaning from the details/summary pair, breaking assistive technology. Instead use this approach:
  - Desktop (above 768px): `.item-actions-toggle { display: none; }` hides the summary button. `.item-actions-menu[open] .item-actions-panel` renders children inline as a flex row (same as current `.tracker-item-actions` layout). The `open` attribute in HTML ensures the panel is always visible on desktop.
  - Mobile (<= 768px): Remove the `open` attribute via a small JS snippet that runs on load and on `htmx:afterSwap`: `document.querySelectorAll('.item-actions-menu').forEach(function(el) { if (window.innerWidth <= 768) el.removeAttribute('open'); })`. The `<summary>` is styled as a 44px touch-target button. `.item-actions-panel` renders as a vertical flex stack when the `<details>` is opened.
- [ ] Ensure priority select, delete, and move confirmations still function inside the `<details>` panel
- [ ] Verify desktop rendering is unchanged -- actions MUST render inline exactly as they do today
- [ ] Goal progress buttons (+1, -1, set) MUST remain visible outside the `<details>` menu on mobile
- [ ] Review `.container` max-width (currently 1100px). With the font size increase to 16px, content may feel tighter. Consider bumping to 1200px if the nav or card grids feel cramped.
- [ ] Run `go test ./...`
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 4: Final Review

- [ ] Run full test suite, `go vet ./...`, and build
- [ ] Verify all success criteria met
- [ ] Perform critical self-review of all changes

---

## Notes

- The `--fg-dim` and `--fg-muted` colour values are estimates. The executing agent MUST calculate actual contrast ratios (formula: `(L1 + 0.05) / (L2 + 0.05)`) or use a web-based contrast checker. Do NOT trust the suggested hex values blindly.
- `-webkit-line-clamp` is well-supported across modern browsers including Firefox 68+.
- The `<details>` approach is intentionally low-tech: accessible, works with htmx SSE reswaps, degrades gracefully. `display: contents` was considered but rejected due to Safari accessibility bugs that strip semantic meaning from the details/summary pair.
- The mobile JS snippet to remove the `open` attribute is a small progressive enhancement. If JS fails, actions are visible (the `open` attribute remains), which is the safer degradation path.
- The font size change from 14px to 16px affects all `rem`-based measurements. The `.container` max-width may need increasing from 1100px to 1200px to prevent content from feeling cramped.

## Critical Files

- `web/static/theme.css` - All CSS changes
- `web/templates/tracker.html` - Wrap action buttons in `<details>`
- `web/templates/goals.html` - Same `<details>` pattern for goal actions
