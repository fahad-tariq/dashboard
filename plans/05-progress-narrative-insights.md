# Progress Narrative & Insights

## Overview

Add progress context, age visibility, velocity insights, streaks, tag aggregation, pace indicators, and idea-to-task conversion tracking. These features transform raw date fields already present in the data model into actionable narratives, giving users a sense of momentum and revealing hidden patterns.

## Current State

**Problems Identified:**
- Goal progress bars show a flat green fill regardless of whether the goal is on track. 25% complete looks identical whether 3 days or 3 months remain.
- Item age is invisible. Open tasks carry `Added` dates but users never see how long items have been sitting.
- Homepage shows counts only ("5 open") with no velocity, trends, or streaks.
- Tags filter within a single section but provide no cross-section narrative.
- Idea-to-task conversion deletes the idea with no linkage preserved.

**Technical Context:**
- `tracker.Item` has `Added` and `Completed` (YYYY-MM-DD strings). Both auto-populated.
- `ideas.Idea` has `Added` only. No completion date.
- Goals have `Current`/`Target`/`Unit` but **no deadline or start date field**.
- Idea-to-task `ToTask` copies title/body/tags then deletes the idea. No provenance recorded.
- Template functions: `percentage`, `formatNum`, `subtract`, `linkify`.
- DB: `tracker_items` with `added`, `completed`, `done`, `type`, `tags` columns.
- `Store.Summary()` returns only `OpenTasks` and `ActiveGoals` counts.

**Key Data Model Gaps:**
1. **No goal deadline field** -- progress bar colour shifting needs a new `Deadline` field.
2. **No idea-to-task linkage** -- needs `FromIdea`/`ConvertedTo` metadata fields.
3. **Streaks/milestones** can be computed from completed dates (no new storage needed).

## Requirements

**Functional Requirements:**
1. Goal progress bars MUST shift colour based on deadline proximity vs percentage complete.
2. Open tasks and untriaged ideas MUST display age badges colour-coded by staleness.
3. Homepage MUST show a one-line micro-insight summarising completion velocity.
4. Homepage MUST show current streak and milestone badges at 10/50/100.
5. Tag aggregation view SHOULD surface cross-section counts per tag.
6. Goals with deadlines MUST show a pace indicator.
7. Idea-to-task conversion MUST record provenance on both sides.

**Technical Constraints:**
1. New date fields MUST use YYYY-MM-DD format.
2. New fields MUST round-trip through parsers without loss.
3. DB changes MUST use the existing migration system.
4. All calculations server-side. Template functions handle display.
5. MUST NOT break parser round-trip tests (especially ideas blank-line preservation).

## Success Criteria

1. Goal progress bars with deadlines visually shift colour.
2. Age badges appear on open tasks and untriaged ideas.
3. Homepage displays velocity insight line.
4. Homepage displays streak and milestone badges.
5. Tag aggregation section on homepage shows cross-section counts.
6. Goals with deadlines show pace indicator string.
7. Converted ideas retain "converted" status with link to resulting task.
8. All existing tests pass; new tests cover insight calculations.
9. `go test ./...` passes; `go build` succeeds.

---

## Development Plan

### Phase 1a: Deadline Field (Data Model)

Split from conversion fields to reduce parser change risk. Parser changes are the most fragile part of the codebase (CLAUDE.md gotchas about blank-line preservation).

- [ ] Add `Deadline` field (string, YYYY-MM-DD) to `tracker.Item` in `internal/tracker/tracker.go`
  - [ ] Add `deadlineRe` regex matching `[deadline: YYYY-MM-DD]`
  - [ ] Parse in `parseItemLine` and write back in `writeItem`
  - [ ] Add `deadline` column via new migration in `internal/db/migrations.go`
  - [ ] Update `Store.ReplaceAllWithAttribution` to persist deadline
- [ ] Write round-trip parser tests for the deadline field
- [ ] Verify ALL existing parser tests pass -- especially ideas blank-line preservation tests
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 1b: Conversion Linkage Fields (Data Model)

- [ ] Add `FromIdea` field (string, slug) to `tracker.Item` and `ConvertedTo` (string, slug) to `ideas.Idea`
  - [ ] Add `[from-idea: slug]` regex in tracker parser
  - [ ] Add `[converted-to: slug]` regex in ideas parser
  - [ ] Write both fields back in their writers
  - [ ] Add `from_idea` column via migration
- [ ] Add `converted` to valid idea statuses
- [ ] Write round-trip parser tests for from-idea and converted-to fields
- [ ] Verify ALL existing parser tests pass
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 2: Insight Computation Engine

New `internal/insights/` package with pure functions, no DB dependency.

- [ ] Create `internal/insights/insights.go` with:
  - [ ] `AgeBadge(added string, now time.Time) (label string, level string)` -- e.g. ("open 14 days", "stale"). Levels: fresh (<7d), ageing (7-14d), stale (14-30d), old (30+d)
  - [ ] `WeeklyVelocity(items []tracker.Item, now time.Time) VelocityInsight` -- return a **struct** (`type VelocityInsight struct { ThisWeek, LastWeek int }`) NOT a pre-formatted string. The template should format it so parts can be styled independently (e.g. bold the number). Provide a `String()` method as convenience but templates should use the fields directly.
  - [ ] `Streak(items []tracker.Item, now time.Time) (current int, total int)` -- consecutive days with completions, plus total count
  - [ ] `MilestoneBadge(total int) string` -- label if crossing 10/50/100/500
  - [ ] `GoalPace(current, target float64, added, deadline string, now time.Time) string` -- pace narrative
  - [ ] `ProgressColour(current, target float64, added, deadline string, now time.Time) string` -- CSS class
  - [ ] `TagAggregation(tasks []TagInfo, ideas []TagInfo) []TagSummary` -- use a **simple interface or struct** (`type TagInfo struct { Tags []string; Type string; Done bool }`) instead of importing concrete `tracker.Item` and `ideas.Idea` types. This avoids a dependency diamond where the insights package imports both tracker and ideas.
- [ ] Define `TagSummary` struct: `Tag`, `TaskCount`, `GoalCount`, `IdeaCount`, `CompletedPct`
- [ ] Define `TagInfo` struct: `Tags []string`, `Type string` (task/goal/idea), `Done bool`
- [ ] Define `VelocityInsight` struct: `ThisWeek int`, `LastWeek int` with `String()` method
- [ ] Write comprehensive table-driven tests in `test/insights_test.go`
- [ ] Verify all tests pass
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 3: Homepage Micro-Insights and Streaks

- [ ] Update `renderHomePage` in `internal/home/handler.go` to compute and pass:
  - `InsightLine` from `insights.WeeklyVelocity`
  - `StreakDays` and `TotalCompleted` from `insights.Streak`
  - `MilestoneBadge` from `insights.MilestoneBadge`
  - `TagSummaries` from `insights.TagAggregation` (top 5)
- [ ] Update `web/templates/homepage.html` to display:
  - Micro-insight line below greeting
  - Streak badge if > 0
  - Milestone badge if present
  - Tag summary section
- [ ] Add CSS classes in `theme.css`: `.homepage-insight`, `.badge-streak`, `.badge-milestone`, `.tag-summary`
- [ ] Update homepage handler tests
- [ ] Verify all tests pass
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 4: Age Badges on Tasks and Ideas

- [ ] Add `ageBadge` template function to `funcMap` in `cmd/dashboard/main.go`
- [ ] Update `web/templates/tracker.html` to show age badge on open tasks
- [ ] Update `web/templates/ideas.html` to show age badge on untriaged ideas
- [ ] Add CSS for `.badge-age-fresh` (green), `.badge-age-ageing` (yellow), `.badge-age-stale` (orange), `.badge-age-old` (red)
- [ ] Test age badge rendering
- [ ] Verify all tests pass
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 5: Goal Progress Context and Pace Indicators

- [ ] Add `progressColour` and `goalPace` template functions to `funcMap`
- [ ] Update `web/templates/goals.html`:
  - Dynamic CSS class from `progressColour` on progress-fill
  - Pace indicator text below progress bar for goals with deadline
  - Deadline display next to added date
- [ ] Update homepage goal cards to use colour-shifted bars
- [ ] Add CSS for `.progress-fill-green`, `-yellow`, `-orange`, `-red`
- [ ] Add deadline field to goal add form
- [ ] Update `Handler.AddGoal` to parse deadline from form input
- [ ] Write tests for goal creation with deadline and pace display
- [ ] Verify all tests pass
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 6: Idea-to-Task Conversion Narrative

- [ ] Change `ToTaskFunc` signature to return `(string, error)` (the slug of the created task)
- [ ] Update all three `ToTaskFunc` implementations in `cmd/dashboard/main.go` to return the task slug
  - Set `FromIdea` on the tracker item before adding
- [ ] Modify `ideas.Handler.ToTask` to set `ConvertedTo` and `Status = "converted"` instead of deleting
- [ ] Update ideas list template to show "converted" ideas in a collapsible section (like the "Done" section in tracker) -- collapsed by default. Users don't need to see converted ideas alongside active ones. Include a link to the resulting task in each converted idea card.
- [ ] Update idea detail page to show "Converted to task: [link]"
- [ ] Update tracker templates to show "From idea: [title]" link when `FromIdea` is set
- [ ] Add conversion rate insight on homepage
- [ ] Update test helpers to match new `ToTaskFunc` signature
- [ ] Update `TestIdeasToTask` to verify "converted" status instead of deletion
- [ ] Write new test for full conversion flow with linkage
- [ ] Verify all tests pass
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 7: Final Review

- [ ] Run full test suite and build
- [ ] Verify all 9 success criteria
- [ ] Check no debug statements remain
- [ ] Verify ideas parser blank-line preservation not regressed
- [ ] Perform critical self-review of all changes

---

## Notes

**Phase ordering:** Phase 1 is split into 1a (deadline) and 1b (conversion linkage) to reduce parser change risk. If 1a causes test failures, it's easier to diagnose with a smaller delta. Phase 2 (insight engine) is pure computation, independently testable. Phases 3-5 can be reordered. Phase 6 depends on Phase 1b's new fields.

**`ToTaskFunc` signature change (Phase 6):** Riskiest change -- touches the function type shared between ideas handler and closures in `main.go`. All three call sites (auth-enabled, single-user, API) plus test helpers in `test/ideas_handler_test.go` and `test/multiuser_test.go` must be updated.

**No JS required:** All calculations are server-side. Age/streak/pace values rendered by Go template functions.

**Staleness thresholds:** The `AgeBadge` function should accept threshold parameters for tuning.

**`TagAggregation` uses `TagInfo` instead of concrete types** to avoid importing both `tracker` and `ideas` packages into `insights`, which would create a dependency diamond. The homepage handler converts items to `TagInfo` structs before passing them.

**`WeeklyVelocity` returns a struct** so templates can style parts independently (e.g. bold numbers). The `String()` method provides a convenience fallback.

**Converted ideas are collapsed by default** in the ideas list, similar to the "Done" section in tracker. This prevents the ideas list from accumulating converted items that clutter active ideation.

## Critical Files

- `internal/tracker/tracker.go` - Item struct, parser/writer for Deadline and FromIdea fields
- `internal/ideas/parser.go` - Idea struct, parser/writer for ConvertedTo, "converted" status
- `internal/home/handler.go` - Homepage handler wiring all insight computations
- `web/templates/homepage.html` - Display insights, streaks, milestones, tag summaries
- `internal/ideas/handler.go` - ToTask handler and ToTaskFunc signature change
