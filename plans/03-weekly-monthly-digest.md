# Weekly/Monthly Digest View

## Overview

New page at `/digest` showing activity summaries for selectable periods (this week, last week, this month). Displays counts, horizontal bar charts, and per-tag breakdowns. Extends `internal/insights/`.

## Design Decisions

- New page at `/digest` (not a homepage tab) -- separate concern, own route
- Period switching via query param (`?period=this-week|last-week|this-month`), default `this-week`
- Invalid period parameter falls back to `this-week` silently (no error flash)
- Monday-start ISO weeks, consistent with existing `WeeklyVelocity`
- Pure HTML/CSS bar charts (no JS charting library)
- Computed on each request from in-memory caches (no new DB or caching)
- Uses `time.Now()` without timezone conversion, consistent with existing insights code
- Ideas have `Added` dates so period-filtered 'added' counts are possible. Triage/conversion dates are unavailable; those counts are shown as all-time totals with a qualifier label
- Family tracker data is included alongside personal data (same as homepage)

## Success Criteria

1. `/digest` renders with period-specific counts
2. Period toggle works via query parameter
3. Bar widths are proportional to the maximum value in the dataset
4. Tag breakdown shows per-tag completions within period
5. Navigation includes "digest" link
6. Layout stacks to single column below 768px
7. Invalid period parameter falls back to `this-week` without error
8. New `insights.Digest` function has table-driven tests; handler has 3+ tests

---

## Development Plan

### Phase 1: Insights Package -- Digest Computation

- [ ] Add `DigestPeriod` type and `DigestResult` struct. Design digest input types to extend or reuse existing `CompletedItem` and `TagInfo` structs where possible, avoiding redundant parallel types. Add `Added` date field if not already present
- [ ] Add `Digest(items, ideas, period, now)` function computing: completed tasks, added tasks, added ideas, per-tag completion counts
- [ ] Extract a shared `weekStart(now)` helper from the existing `WeeklyVelocity` week-start logic (lines 91-98 of insights.go). Use it in both `WeeklyVelocity` and the new `periodBounds`. Two copies of week-start logic will drift
- [ ] Add `periodBounds(period, now)` helper using `weekStart` (Monday-start weeks, calendar months)
- [ ] `Digest()` returns a `MaxValue int` field in `DigestResult` so templates can compute percentage bar widths without Go template arithmetic
- [ ] Table-driven tests: period boundaries, empty data, tag aggregation, Sunday edge case
- [ ] STOP and wait for human review

### Phase 2: Digest Handler and Template

- [ ] Add digest handler to the home package supporting both auth-enabled and single-user modes (matching the existing `HomePage`/`HomePageSingle` pattern)
- [ ] Shared render function fetches personal, family, and ideas data, converts to digest input types, calls `insights.Digest`. Merge personal and family tracker items before calling `Digest()` -- the handler merges, not the insights function (same pattern as homepage velocity)
- [ ] Compute period-filtered 'added ideas' counts (ideas have `Added` dates). Compute all-time converted/triaged idea totals separately (dates unavailable for period filtering)
- [ ] Create `web/templates/digest.html`: period toggles, summary stat cards, horizontal bar chart, tag breakdown, empty state
- [ ] Add `"digest.html"` to the `pages` slice in `parseTemplates()`
- [ ] STOP and wait for human review

### Phase 3: Routing, Navigation, and CSS

- [ ] Expand `mountAppRoutes` signature to accept digest handler parameter and register `GET /digest`
- [ ] Wire handler in both auth-enabled and single-user branches of `main.go`
- [ ] Validate the `period` query parameter; fall back to `this-week` for unrecognised values
- [ ] Add "digest" nav link in `layout.html`
- [ ] Add `g d` keyboard shortcut in `shortcuts.js` and update the help modal
- [ ] CSS: digest layout, bar chart rows, single-column stacking below 768px
  - Reuse `.filter-tag`/`.filter-tag.active` pattern for period toggle (do not invent new toggle)
  - Reuse `.summary-stats`/`.stat-item`/`.stat-value` for stat cards
  - Bar fills must NOT have `transition: width` (server-rendered, no dynamic updates). If entry animation is desired, gate behind `prefers-reduced-motion`
  - Non-zero bar fills need `min-width: 2px` so small values remain visible
  - Bar colours: `--green` for completed, `--accent` for added tasks, `--teal` for ideas. Each row must have a text label (colour alone fails WCAG 1.4.1)
- [ ] STOP and wait for human review

### Phase 4: Tests and Final Review

- [ ] Digest handler tests (renders OK, period switching, invalid period fallback, empty state)
- [ ] Full test suite, linter, build
- [ ] Self-review: verify `prefers-reduced-motion` on any CSS animations, ES5 JS compliance for shortcuts, flash message conventions (read-only page likely needs none)
- [ ] Verify all success criteria met
- [ ] STOP and wait for human review

## Critical Files

- `internal/insights/insights.go` -- `Digest()`, `DigestResult`, `periodBounds`
- `internal/home/digest.go` -- New file: handler and rendering logic
- `cmd/dashboard/main.go` -- Route registration, `mountAppRoutes` signature, template parsing
- `web/templates/digest.html` -- New template
- `web/static/theme.css` -- Digest layout, bar chart, summary card CSS
- `web/static/shortcuts.js` -- Keyboard shortcut registration
