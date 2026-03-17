# Visual Feedback & Microinteractions

## Overview

Add five visual feedback improvements to the dashboard: an upgraded loading indicator, a visual filter badge, task completion celebrations, idea triage transition animations, and styled confirmation dialogs replacing browser `confirm()`. All changes are CSS and JS only -- no Go handler or data model changes required.

## Current State

**Problems Identified:**
- Loading indicator (`<span class="htmx-indicator meta-dim">updating&hellip;</span>`) uses `meta-dim` which is low contrast (`--fg-dim`) and has no animation. Nearly invisible.
- Filter state persists in `localStorage` (via `tracker.js`) but on page reload there is zero visual indication that results are filtered. Users see a subset of items without knowing why.
- Task completion (clicking the checkmark) triggers a full-page SSE refresh. The completed item simply disappears and reappears under the "Done" section with strikethrough + 0.7 opacity. No positive reinforcement.
- Idea triage actions (park, drop, untriage) cause an instant SSE-driven page refresh. Items vanish from one section and appear in another with no transition.
- Seven `confirm()` calls across templates (`tracker.html` x3, `ideas.html` x1, `idea.html` x1, `goals.html` x1, `admin-users.html` x1) use unstyled browser dialogs.

**Technical Context:**
- Single CSS file: `web/static/theme.css` (1126 lines)
- Two JS files: `web/static/tracker.js` (filter/toggle logic, 117 lines), `web/static/upload.js` (image upload, 153 lines)
- Templates use Go `html/template`. SSE via htmx-sse triggers full container swaps (`hx-swap="outerHTML"`).
- Catppuccin Latte/Mocha dual theme with CSS custom properties.
- htmx `afterSwap` event is already used to re-apply filters and re-init upload handlers.

## Requirements

**Functional Requirements:**
1. The loading indicator MUST use a visible animation (pulse or spin) with sufficient contrast in both light and dark themes.
2. A "Filtered" badge MUST appear in the filter bar when any filter is active, and MUST disappear when cleared.
3. Task completion MUST trigger a brief celebratory CSS animation (green flash/pulse) on the item before it moves to the done section.
4. Idea triage state changes MUST animate the affected card (fade out or slide) before the SSE swap replaces the page content.
5. All `confirm()` calls MUST be replaced with a themed modal dialog that matches the dashboard design and includes a "This cannot be undone" warning for destructive actions.
6. All animations MUST respect `prefers-reduced-motion: reduce` by disabling or minimising motion.

**Technical Constraints:**
1. Solution MUST NOT add external dependencies.
2. Solution MUST work with the existing htmx SSE full-swap pattern.
3. Solution MUST NOT modify Go handlers or data models.
4. CSS additions MUST use existing custom properties for colours.
5. JS MUST remain ES5-compatible to match existing conventions.

## Success Criteria

1. Loading indicator is visible as an animated element with contrast meeting WCAG AA against both theme backgrounds.
2. When any tag or priority filter is active and the page is reloaded, a "Filtered" badge is visible in the filter bar.
3. Clicking the complete button on a task triggers a visible green pulse/flash animation before the item transitions to done.
4. Triage actions on ideas produce a fade-out animation on the affected card.
5. All seven `confirm()` calls are replaced with a styled modal. Modal includes a warning line and Cancel/Confirm buttons.
6. All animations are suppressed or reduced when `prefers-reduced-motion: reduce` is active.
7. Linting passes without warnings or errors.
8. All tests pass (`go test ./...`).
9. Build succeeds.

---

## Development Plan

### Phase 1: Loading Indicator & Filter Badge (CSS + JS)

- [ ] Add `@keyframes dash-pulse` animation to `theme.css` that pulses opacity between 0.4 and 1.0 over 1.2s
- [ ] Replace the `.htmx-indicator` styles: set `color: var(--accent)`, `font-weight: 600`, and apply `animation: dash-pulse 1.2s ease-in-out infinite`
- [ ] Add a `prefers-reduced-motion: reduce` media query that sets `animation: none` on `.htmx-indicator` (and all other animations added in later phases)
- [ ] Add a `.filter-active-badge` CSS class styled as a small badge using `--accent` background
- [ ] In `tracker.js`, modify `applyFilter()` to insert or remove a `.filter-active-badge` element in the `.tracker-filters` container when `activeFilterType` is truthy
- [ ] Ensure the badge is re-created after `htmx:afterSwap` events
- [ ] Verify both features in light and dark themes
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 2: Task Completion Celebration (CSS + JS)

- [ ] Add `@keyframes dash-complete-flash` to `theme.css`: brief animation that flashes the item's background to translucent green, holds, then fades back. Duration ~600ms.
- [ ] Add a `.tracker-item-completing` CSS class that applies this animation
- [ ] In `tracker.js`, add a `celebrateComplete(form)` function that finds the parent `.tracker-item`, adds `.tracker-item-completing`, then returns `true` to allow form submission
- [ ] Add an `htmx:beforeSwap` listener that checks for any `.tracker-item-completing` element in the swap target. If found, delay the swap by 400ms (using a promise or setTimeout callback) to let the animation complete before the SSE swap replaces the DOM. Without this, the animation is invisible on fast connections because the SSE swap destroys the animating element immediately.
- [ ] In `tracker.html`, add `onsubmit` handler to the complete form calling `celebrateComplete(this)`
- [ ] Add this animation to the `prefers-reduced-motion` rule
- [ ] Test by completing a task and observing the green flash. Verify the animation is visible even on fast local connections (the beforeSwap delay should prevent instant replacement)
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 3: Idea Triage Transition Animations (CSS + JS)

- [ ] Add `@keyframes dash-fade-out` to `theme.css`: opacity 1 to 0 plus slight vertical slide over 300ms
- [ ] Add a `.idea-transitioning` CSS class that applies the animation with `pointer-events: none`
- [ ] In `tracker.js`, add a `triageAnimate(form)` function that: captures the form's `action` URL and `method`, adds `.idea-transitioning` to the parent `.tracker-item`, then after 250ms submits via `fetch(action, {method, credentials: 'same-origin'})` followed by a page reload. Using `fetch` instead of `form.submit()` avoids a race condition where the SSE swap could destroy the form element before the delayed submit fires. Return `false` from the `onsubmit` to prevent immediate submission.
- [ ] In `ideas.html`, add `onsubmit="return triageAnimate(this)"` to triage action forms (park, drop, untriage) but NOT the delete button
- [ ] Add this animation to the `prefers-reduced-motion` rule
- [ ] Test triage transitions in both themes
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 4: Styled Confirmation Dialogs (CSS + JS + Templates)

- [ ] Add modal CSS to `theme.css`:
  - `.confirm-overlay`: fixed fullscreen overlay with semi-transparent background, z-index above nav, flex centering
  - `.confirm-dialog`: card with `--bg-card` background, max-width 400px
  - `.confirm-warning`: `--red` colour for "This cannot be undone" line
  - `.confirm-actions`: flex row with Cancel and Confirm buttons
  - Entry animation: fade-in + slight scale, respecting `prefers-reduced-motion`
- [ ] Add a reusable confirm modal HTML template to `web/templates/layout.html`: a hidden `div.modal-overlay#confirm-modal` containing `.modal-content` with a title, warning text, and Cancel/Confirm buttons. This is the **shared modal infrastructure** that Plan 06 (Power User Features) will also reuse for the search overlay and shortcut help modal. Use `.modal-overlay` and `.modal-content` as the base class names (not `.confirm-overlay`/`.confirm-dialog`) so that Plan 06 can extend rather than duplicate.
- [ ] Update the modal CSS to use `.modal-overlay` and `.modal-content` as base classes. Add `.confirm-warning` for the destructive action warning text and `.confirm-actions` for the button row. These extend the base modal, not replace it.
- [ ] Create `web/static/dialog.js` with a `confirmAction(form, message)` function that:
  - Shows the `#confirm-modal` overlay and populates its title/warning text dynamically
  - Cancel button hides the modal and returns focus to the triggering button
  - Confirm button hides the modal and submits the form
  - Traps focus within the dialog (tab cycling between Cancel and Confirm)
  - Closes on Escape key press
  - Uses ARIA: `role="alertdialog"`, `aria-modal="true"`, `aria-labelledby` pointing to the title
- [ ] In `layout.html`, add `<script src="/static/dialog.js"></script>`
- [ ] In all seven templates, replace `onclick="return confirm('...')"` with `onsubmit="return confirmAction(this, '...')"`:
  - `tracker.html`: 3 instances (delete item, move item, delete done item)
  - `ideas.html`: 1 instance (delete idea)
  - `idea.html`: 1 instance (delete idea)
  - `goals.html`: 1 instance (delete goal)
  - `admin-users.html`: 1 instance (delete user)
- [ ] Add "This cannot be undone" warning to all delete confirmations. Omit for "Move to..." (reversible).
- [ ] Test all seven confirmation points
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 5: Final Review

- [ ] Run full test suite and verify all tests pass
- [ ] Build application and verify no errors or warnings
- [ ] Verify all animations respect `prefers-reduced-motion: reduce`
- [ ] Check CSS contrast for `.htmx-indicator` in both themes
- [ ] Review JS for ES5 compliance
- [ ] Verify all success criteria are met

---

## Cross-Plan Dependencies

- **Plan 01 MUST run before this plan.** Plan 01 Phase 2 modifies `applyFilter()` in `tracker.js` to add `.filter-empty` show/hide. This plan's Phase 1 also modifies `applyFilter()` to add the filter badge. Running 01 first keeps changes minimal and avoids merge conflicts.
- **Plan 06 reuses this plan's modal infrastructure.** Phase 4 establishes `.modal-overlay`/`.modal-content` as shared CSS classes and puts the confirm modal HTML in `layout.html`. Plan 06 extends this with `#search-overlay` and `#shortcut-help` modals using the same base classes.
- **`ideas.html` is touched by Plans 01, 02, 04, and 05.** This plan modifies: triage animation `onsubmit` handlers and the delete `confirm()` replacement. Different sections from other plans' changes.

## Notes

- The celebration animation uses an `htmx:beforeSwap` delay to ensure visibility. Without this, the SSE swap replaces the DOM before the animation completes on fast connections.
- `dialog.js` is a new file to keep responsibilities clear. Both files stay short.
- The triage animation uses `fetch()` for delayed submission instead of `form.submit()` to avoid a race condition where the SSE swap could destroy the form element before the timeout fires.
- The modal CSS uses `.modal-overlay`/`.modal-content` as shared base classes. Plan 06 will add `#search-overlay` and `#shortcut-help` using the same base. Do NOT create a parallel modal system with different class names.

## Critical Files

- `web/static/theme.css` - All CSS additions (keyframes, modal base classes, filter badge)
- `web/static/tracker.js` - Filter badge, completion celebration, triage animation, beforeSwap delay
- `web/templates/tracker.html` - Replace 3 confirm() calls, add completion handler
- `web/templates/ideas.html` - Replace 1 confirm(), add triage animation handlers
- `web/templates/layout.html` - Add dialog.js script tag, confirm modal HTML template
