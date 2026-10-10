# Plan 3: Stable item IDs and due dates

## Goal

Give every list item a permanent ID, so renames, moves and duplicate titles stop breaking things. Add due dates to tasks so a "must be done by" date is visible and hard to miss, without touching the daily plan.

| Plan | Scope | Status |
|---|---|---|
| 1 | Foundations and module framework | Done (2026-10-06) |
| 2 | Exercise module | Parked (2026-10-07): the owner uses Hevy |
| 3 (this) | Stable item IDs, due dates on tasks | Phases 1 to 3 done and signed off; Phase 4 (plan untick and bulk select) next, then Phase 5 (clean up) |

## Context

The scope comes from an interview with the owner on 2026-10-07. Plan 1 listed nine candidate features, and the owner kept the two they actually miss. The rest are parked in `docs/backlog.md` under "Parked features", with the reasoning: quick capture, recurring tasks, weekly review, reminders, an agent-proposed plan and moving off markdown. The MCP sidecar was deleted on the same day (backlog, "MCP removed"). Five review agents (design, security, simplification, code impact, quality) reviewed this plan against the code before it was accepted.

Read `CLAUDE.md` before starting; its gotchas apply throughout.

**Why IDs.** An item's slug is computed from its title on every parse (`internal/slug`), and slugs are the only identity an item has. They key about 71 route patterns, the planner, search and widget links, morph DOM ids, undo paths, `[from-idea:]`/`[converted-to:]`, the `commentary` primary key `(item_slug, item_list, user_id)` and the API. As a result:
- Two items with the same title share a slug, and every action hits the first one (`AddItem` never checks for duplicates).
- `MoveToList`'s `-<unix>` suffix does nothing, because `AddItem` re-slugifies.
- A rename changes the item's URL, loses its expanded state on the next morph, and orphans its commentary.

**Why due dates.** Tasks have `[planned:]` ("I'll do it that day") but nothing for "must be done by". Goals already have `[deadline:]`.

**Two existing injection gaps this plan closes.** Inline tags are stripped from input only in `tracker/api.go` (`httputil.StripInlineMetadata`). Web forms, house, ideas and sub-step promotion write titles as typed, so a title like `Foo [planned: 2026-01-01]` sets metadata. Date fields are not validated either: `home.SetPlanned` writes `date` as given, so `x] [tags: y` injects a tag.

### Decisions already made

Do not reopen these while executing this plan.

- **IDs are an inline tag**, `[id: xxxxxxxx]`, written last on each unindented item line, in all three parser families: tracker (personal, family, house projects, goals), maintenance and ideas. Sub-steps, maintenance log entries and other indented lines never get IDs. IDs do not live in SQLite: nothing in the text stays fixed across a rename, so there is nothing to key them on.
- **ID format:** exactly 8 characters from `bcdfghjklmnpqrstvwxz0123456789` (20 consonants without `y`, plus digits), generated with `crypto/rand`. With no vowels, an ID can never equal a static route segment such as `add`, `bulk` or `complete`. The parser accepts only a final `[id: ...]` that matches this format, anchored at the end of the line.
- **The service assigns IDs whenever it loads or writes a file.** This covers start-up, lazy per-user loads (`Registry.ForUser`) and watcher resyncs. Any ID-less item gets an ID in place, on the slice the cache will hold, and the file is written through the normal atomic write. There is no separate migration command.

  The watcher does not loop: the write records its hash, so the resync it triggers matches and is skipped. Clobbering an open editor buffer is not a concern because **the owner never hand-edits the markdown**. Parsers and renderers stay pure, so the round-trip fixtures stay byte-identical. Within one file IDs are unique: if a load finds a duplicate, the first occurrence keeps it and later ones get new IDs.
- **IDs are unique per file and unique across files by chance.** Every lookup goes through a list-scoped service. A moved item keeps its ID. If the target file already holds that ID (a leftover from a failed move), the moved item gets a new one. Converting an idea to a task gives the task a new ID. `[from-idea:]` and `[converted-to:]` hold IDs; their regexes (`[\w-]+`) already accept them.
- **Slug stops being an identity.** Route params become `{id}`, form fields `id`/`ids`, data attributes `data-id`. The `Slug` field is deleted from the item structs, so the compiler and template execution errors find every remaining use.
- **Inline tags are stripped from every title at the service layer**, for all callers: web, API, house, ideas and sub-step promotion. No user types a tag on purpose. Body lines are indented, so they are never parsed as tags; leave body handling as it is.
- **Dates are validated in the service methods** (`SetPlanned`, `BulkSetPlanned`, the deadline edit, `AddGoal`'s service call). Every caller is covered: web and API.
- **Commentary moves to a new table** keyed `(item_id, user_id)`. The old `commentary` table stays untouched until the last phase, so an older binary still runs if production rolls back. Keying without the list means a move keeps its commentary. Commentary is written only through the API (`internal/app/routes.go`), which is off in production, so there is nothing to migrate.
- **The API switches to IDs together with the web app** (Phase 3), because both call the same services. There is no MCP sidecar any more; a future one is rebuilt against the ID-based API.
- **Due dates reuse `[deadline:]`.** The parser, `writeItem`, `inlineMetaRe` and the round-trip fixtures already handle it on tasks. The field stays `Deadline`; the UI says "due". `GoalPace` and `ProgressColour` return early for items with no target, so goal logic doesn't touch tasks.
- **A due date is separate from the planned date and never changes it.** Due items are never added to the daily plan automatically. Changing the planned date never moves the deadline.
- **Where due dates show:** a row badge, the planner row label (only when due today or overdue), and a "Due soon" home widget with a one-click "Plan today". They do not appear on the calendar, which is built on planned dates, so a drag would be ambiguous. No list filter, no quick-add syntax (`ParseQuickAdd` has no production caller).
- **Due dates cover todos and family tasks, plus the goals edit form.** House projects are out: their rows have no `tracker-meta` and no plan button.
- **Goals stay a separate item type.** Plan 1 left "do goals fold into tasks?" to this plan. The answer is no.
- **Markdown stays the source of truth.**

## Requirements

1. All existing behaviour MUST keep working: tasks, family, goals, ideas, house, planner, calendar, digest, search, commentary, REST API.
2. Every bug fix MUST start with a failing test, committed before the fix.
3. Every phase MUST end with `make lint test` passing (with `INTEGRATION=1`, as CI does) and CI green, Playwright included, before deploying. Each phase MUST leave `main` deployable.
4. Round-trip fixtures in `test/testdata/roundtrip/` MUST stay byte-identical. ID-less fixtures stay ID-less (renderers do not assign); new fixtures cover IDs.
5. After Phase 2 is deployed, every item line in every file the app has loaded MUST carry an ID. No write may drop or change an existing ID.
6. A title MUST NOT be able to set or change any inline tag, `[id:]` included, through any input path.
7. Due-date text MUST pass the contrast tests in all six style and theme blocks, and MUST NOT carry meaning by colour alone.
8. No new dependencies.

**Test conventions:** new Go tests go in `test/` as map-based table tests, with temp dirs and in-memory SQLite and no external services. Playwright runs only in CI (or `bash e2e/run.sh` outside the nono sandbox).

**Deploying and recording.** Follow the homelab repo's `CLAUDE.md` section "Dashboard (fliptronic)":
1. Run `./backup.sh` as its own call and check its exit code.
2. Pull the image, then `up -d`.
3. Confirm the container's `org.opencontainers.image.revision` label matches the commit.
4. Run the Caddy curl checks and `verify-stack.sh`.

"Record the deploy" means adding to that phase's Working notes: the commit, the image revision, the backup filename, the check results, and anything the phase asks you to measure. To roll back, restore the backup and pin the compose image to the previous revision (production pulls `:latest`).

## Non-goals

- Everything under "Parked features" in `docs/backlog.md`.
- Due dates on house projects, or in the API's add endpoint.
- Slug fallback addressing or redirects from old slug URLs. `/ideas/{slug}` bookmarks stop working; `/exploration/*` redirects to `/ideas`.
- Fixing `relativeDate`'s UTC parsing for its existing uses.

---

## Phase 1: Due dates on tasks

**Purpose:** a task can carry a due date that shows where it matters, and dates are always valid.

- [x] **Validate dates (bug, test-first).** Each case below is one row in a table test.
  - Reject anything that is not a real `YYYY-MM-DD` date (`time.Parse`) in the service methods `SetPlanned`, `BulkSetPlanned` and the deadline edit, and in the deadline handling behind `AddGoal`.
  - Handlers map the error to a 400 with a specific message.
  - Entry points: `tracker.Handler.AddGoal`; the `home/handler.go` set, bulk-set, reorder and JSON plan endpoints; the web edit form; `APIUpdateTodo`.
  - An empty value clears the date where clearing is allowed.
  - Include an injection row: `x] [tags: y` is rejected.
- [x] **One edit path.**
  - Add `Deadline *string` to `tracker.Edit` (nil keeps the value, empty clears it).
  - Switch `tracker.Handler.UpdateEdit` (it serves todos, family and the goals edit form) to `Service.ApplyEdit` with every field it holds. Set `Deadline` only when `r.PostForm` contains `deadline`, so a form without the field never wipes it.
  - Delete `Service.UpdateEdit` once nothing calls it.
  - `APIUpdateTodo` accepts an optional `deadline`. `itemToAPI` returns `deadline` for tasks as well as goals.
- [x] **Forms.**
  - Add a date input with a visible "Due" label and an id unique per item to the task add and edit forms (`web/templates/tracker/tracker.html`) and the goals edit form (`goals.html`).
  - Give each one a "Clear" control, because Safari's desktop date input has none.
- [x] **Due label helper.** A func-map helper built in `buildFuncMap(loc)` returns plain strings, never `template.HTML`: a short label, a full label and a level.
  - Short label: "due today", "due Fri" within 6 days, "due 14 Nov" further out, "overdue 2d".
  - Full label: "Due Friday 9 October", "Overdue by 2 days".
  - Level: muted when more than 2 days away, attention within 2 days, danger once overdue.
  - Compute days in `loc`.
  - Table-test the boundaries: today, tomorrow, 2, 3, 6 and 7 days, yesterday, a year boundary, and a DST change in `Australia/Sydney`.
- [x] **Row badge and planner label.**
  - In the `tracker-meta` define, after priority and before the planned badge, render `<time datetime="YYYY-MM-DD">` with the short label visible and the full label as screen-reader text.
  - Colours: `--text-muted`, `--attention`, `--danger`, with no opacity. The word "overdue" carries the meaning.
  - In the homepage `plan-row`, show due only when it is today or past, merged with the carry-forward label into one span ("from 3 days ago · due today").
  - Goals use the same helper instead of printing the raw ISO date.
  - Add any new colour pairing to the contrast tests as `CLAUDE.md` describes.
- [x] **"Due soon" home widget** from the `todos` module.
  - **Contents.**
    - Open, non-deleted tasks (not goals) from the personal and family lists that are overdue or due within 3 days.
    - Overdue first, then by date. Family rows are labelled "Family".
    - Up to 5 rows. The count shows the total. No "+N more" link, because no page lists due items.
    - Return no widget when nothing is due, as the house and ideas widgets do.
  - **Fields.** `WidgetItem` gains a plain-string `Meta` (the due label and its level) and an optional plan action (list and item key). The `card` component renders `Meta` as its own coloured span, so the title link keeps its normal colour. Item severity maps to the existing `warning` (due within 2 days) and `danger` (overdue).
  - **Plan today.** The button posts to `/plan/set` with the list and item key, and no `date`, so the server uses today in `loc` and a tab left open past midnight still plans the right day. Its aria-label is "Plan {title} for today". Once the item is planned for today, the button is replaced by a "planned" badge rather than removed, so the row and focus stay put.
  - **Morph and focus.** Card rows get unique ids. The widget body carries `data-row-list`/`data-row-heading` and rows carry `data-row`, so `live.js` focus restore works after the morph.
  - **Contract.** The registry checks any action path with `httputil.IsLocalPath`. Give the fixture module one action under its own prefix, and extend `TestModuleContract`.
  - **Refresh.** The homepage already refreshes on `changed:todos` and `changed:family`.
- [x] **Tests.**
  - Edit semantics: an absent field keeps the date, an empty one clears it, API nil keeps it.
  - A deadline survives `MoveToList` and is untouched by house page and API edits.
  - Widget contents, order, empty state and plan-today post.
  - Playwright: set a due date through the edit form, see the row badge and the widget, press "Plan today" and see the item in the plan with focus kept. Run `TestE2ESelectorsExist`.
- [x] **Verification.** `make lint test` green. Route goldens unchanged (or any diff explained here). CI green. Deploy and record it.
- [x] Self-review with an independent agent; fix what holds up.
- [x] **STOP and wait for human review.** The owner judges the badge and widget in the live app. Also decide whether to hide the age badge on rows with a due date: `badge-age-old` already uses `--attention`.

---

## Phase 2: IDs in the files

**Purpose:** every item in every file gets an ID, and every write keeps it. Addressing still uses slugs, so nothing visible changes.

- [x] **Strip tags from titles (bug, test-first).**
  - Every service method that sets a title strips inline tags: `AddItem`, the edit paths, house project add and edit, idea add and edit, `PromoteSubStep`, `toTask`.
  - Add `id` to `inlineMetaRe`.
  - Allowlist the ideas `[status:]` at parse time, falling back to `untriaged` (backlog "Ideas status CSS class injection"), and remove that backlog entry.
  - Table-test one injection row per input path.
- [x] **Parse and render IDs.**
  - The tracker, maintenance and ideas parsers read a final `[id:]` in the agreed format into an `ID` field and strip it from the title. A malformed one stays in the title and is logged with file and line, not the title text.
  - Renderers emit `[id:]` last when the item has one.
  - New `*_ids.md` round-trip fixtures per parser family cover: IDs on items, sub-steps under an item with an ID, maintenance log entries, ideas with blank lines, an ideas-file indented checkbox before any idea, and `[from-idea:]`/`[converted-to:]` holding IDs. They must be byte-identical.
- [x] **Generate and assign.**
  - Add one generator using `crypto/rand` over the agreed alphabet.
  - Each service's `loadCache`, `ResyncIfChanged` and `write` assign missing IDs **in place** on the slice the cache keeps, and repair duplicates (first occurrence wins). Write only when something was assigned.
  - `AddItem` and ideas `Add` assign on the caller's item and return the ID, which `toTask` and `MarkConverted` need. `MoveToList` keeps the ID, unless the target already has it.
  - Delete the dead `-<unix>` suffix.
  - `toTask` writes `[from-idea:]`/`[converted-to:]` as IDs.
  - `cmd/dashboard/migrate.go` writes through `ideas.WriteIdeas`, outside the service, so assign IDs there too.
- [x] **Old references.** On the load that assigns IDs, existing `[from-idea:]`/`[converted-to:]` slugs that resolve to exactly one item become that item's ID. Ambiguous or missing targets stay as they are and are logged. Dangling references are already accepted. Do this in each service's assignment step, never holding two service locks at once (see `CLAUDE.md`).
- [x] **Tests.**
  - Every load and write path leaves every item with an ID.
  - No mutation changes an existing ID: rename, move, bulk actions, trash, restore, purge.
  - Duplicates are repaired, and the first keeps its ID.
  - An external ID-less edit gets IDs on resync, then a mutation on another item, then `Get` by the new ID works with no restart.
  - An app write still gives exactly one change event and zero re-parses, using the existing event tests.
  - Lazy per-user loads assign IDs.
- [x] **Verification.** `make lint test` green and route goldens unchanged. CI green. Deploy and record it, including a count of items and IDs per production file, taken from the backup and the live files after start-up. A file with items and no IDs fails the phase.
- [x] Self-review with an independent agent; fix what holds up.
- [x] **STOP and wait for human review.** This is the first deploy that rewrites every production file. The owner confirms the pages look right before addressing changes.

---

## Phase 3: Address items by ID

**Purpose:** the web app and the API identify items by ID. This is the riskiest phase: a missed `data-slug` or `item-{{.Slug}}` silently breaks drag, select, focus or expanded state, and only Playwright catches it. Do it in one pass, because `tracker.js` and `planner.js` are shared across lists.

- [x] **Remove `Slug`.**
  - Delete the `Slug` field from the tracker, maintenance and ideas item structs.
  - Services (`Get`, `mutate`, `mutateBatch`, `PermanentDelete`, `MarkConverted`, restore, bulk methods) take IDs.
  - Slug-keyed maps (`home/handler.go` `exclude`, `modules/tasks/tasks.go` `planned`, and any others the compiler finds) key on IDs.
- [x] **Routes and handlers.**
  - Every module mount, `/plan/...` and `/commentary/{list}/{id}` use `{id:[b-df-hj-np-tv-xz0-9]{8}}`, so a malformed ID never reaches a service.
  - Form fields `slug`/`slugs` become `id`/`ids`, validated before any service call.
  - Undo, `?undo=` and `redirectBack` anchors encode the ID (house `LogDone`, `PromoteSubStep`, the widget action).
  - `/exploration/*` redirects to `/ideas`.
  - Regenerate both route goldens and record the diff: parameter names and the regex only, plus the exploration change.
- [x] **Templates and JS.**
  - DOM ids become `item-{id}`, `idea-{id}`, `maint-{id}`, `plan-{list}-{id}`, `pick-{list}-{id}`, keeping the existing suffixes.
  - `data-slug` becomes `data-id`, including `data-row`, `data-row-of`, `data-commentary-url` and the drag `dataTransfer`.
  - Update `tracker.js` (`getSelectedSlugs`, `#item-` hash handling), `planner.js` (`collectSlugs`, request bodies) and `house.js`.
  - The "From idea" link uses link text such as "Original idea", not the raw ID.
  - Search results, widget URLs, calendar entries and planner "open in list" links use IDs.
- [x] **Commentary.**
  - Add migration entries (one statement each) creating `item_commentary` keyed `(item_id, user_id)`. The store, web handler, API handler and `httputil` validation use it.
  - Permanent delete and purge remove an item's commentary.
  - Tests: a renamed item keeps its commentary, and a moved item keeps it too.
- [x] **API.**
  - `/api/v1` routes take `{id}`. JSON items carry `id` and no `slug`. Plan endpoints take `ids`. `planItemsToAPI` and the ideas API include `id`.
- [x] **Tests and e2e.**
  - Update the slug-pinned Go tests. The largest are `ideas_handler_test`, `tracker_test`, `ideas_service_test`, `tracker_handler_test`, `planner_test` and `tracker_api_test`; also `commentary_*`, `bench_test`, `multiuser_test`, `admin_test`, `fragment_test`, `calendar_test`, `move_to_list_test`, `house_*`, `modules_test` and `api_test`.
  - Test-first regressions: two tasks with the same title are completed, edited and trashed independently, and a rename keeps the URL.
  - Give the Playwright fixtures fixed IDs and update the specs that address slugs (`search.spec.ts`, `planner.spec.ts`, and any others).
  - New spec: rename an expanded item and confirm it stays expanded after the live refresh.
  - `TestE2ESelectorsExist` and `TestMorphSkipElementsHaveIDs` pass.
  - End with `grep -rn -i slug internal/ web/`, and list in Working notes every remaining hit with its reason (for example, `internal/slug` used for display or anchors).
- [x] **Verification.** `make lint test` green. Route golden diff recorded. CI green, Playwright in all styles and themes included. Deploy and record it.
- [x] Self-review with an independent agent; fix what holds up.
- [x] **STOP and wait for human review.** The owner uses the live app for a day before Phase 4.

---

## Phase 4: Untick and bulk select on today's plan

**Purpose:** a done task can be unticked from the homepage plan, and several of today's tasks can be completed, moved to tomorrow, dropped or trashed at once. Added at the Phase 3 sign-off (2026-10-11) from the owner's use of the live app; a three-agent design council (interaction, visual and accessibility, implementation) agreed the shape below.

- [x] **House ticks set the status (bug, test-first).** Ticking a house project on the plan (`home.CompletePlanned`) or the house page (`house.CompleteProject`) calls `Complete`, which sets `Done` but leaves `Status`, so the house page and the digest disagree. Both call `UpdateStatus(id, "done")`.
- [x] **Untick.**
  - `POST /plan/{id}/uncomplete` with `list`, mirroring `CompletePlanned`. For house, `UpdateStatus(id, "todo")` (the earlier status is not stored, so in-progress becomes todo; accepted). Flash `plan-uncompleted`.
  - The done row's circle becomes a form button like the todos page's "Mark not done" (`tick tick-done`, accessible name "Mark {title} not done", `data-stop-click`). It keeps the id `{row}-done` in both states so morph pairs it and focus stays. Planned date and plan order are kept. No celebrate, no undo toast.
  - Hover and focus on a done tick preview the unticked state, on the plan and the todos page. Contrast checked in all six blocks.
- [x] **Bulk select on the plan.**
  - A "select" toggle (`aria-pressed`) beside the Today heading, reusing `tracker.js` select mode: `toggleSelectMode`/`exitSelectMode` also find the plan container; selection reads `.plan-item` as `list:id`.
  - In select mode the checkbox (`{row}-select`, "Select {title}") takes the tick's grid slot and the tick hides; only open rows are selectable; a row click toggles its checkbox instead of expanding; drag and the up/down buttons are off (`syncDraggable`).
  - Bar: select all, deselect, complete, move to tomorrow, drop, trash (trash confirms through `confirmBulkDelete`; no undo, as on todos). The bar wraps at 360px and does not cover the last row.
  - `POST /plan/bulk/{complete,tomorrow,clear,delete}` take `items=list:id,...`. Every entry is validated (`itemid.Valid`, known list) and every item checked to exist and not be deleted before anything is written; then one batch write per list, in turn, never two locks at once. A failure after the check (a race) answers an error flash. House complete goes through a batch status update so it sets `Status`. "Tomorrow" uses `BulkSetPlanned` with tomorrow in `loc`.
  - Flash messages carry the count. After a bulk action select mode exits and focus goes to the select toggle.
  - Coarse pointer: 44px minimums for `.bulk-checkbox` and `.bulk-bar .btn` (the todos page gains them too).
- [x] **Tests.** Handler table tests: untick on all three lists, house status both ways, each bulk action on a mixed-list selection, a malformed entry and a missing ID that write nothing. Route goldens regenerated and the diff recorded. `TestE2ESelectorsExist`, `TestMorphSkipElementsHaveIDs`. Playwright: untick keeps focus on the circle; select two rows from different lists and complete, move to tomorrow, drop and trash them; axe in select mode.
- [x] **Verification.** `make lint test` green, CI green, deploy and record it.
- [x] Self-review with an independent agent; fix what holds up.
- [ ] **STOP and wait for human review.** The owner tries untick and bulk select in the live app.

---

## Phase 5: Clean up and document

- [ ] **Drop dead tables.** Add migrations dropping the old `commentary` table and the unused `tracker_items` table. Remove `DELETE FROM tracker_items` from `auth.DeleteUser` in the same change, or user deletion fails. Remove the backlog entry "Drop the `tracker_items` table".
- [ ] **`CLAUDE.md`** (use the `claude-md-authoring` skill):
  - Update the id scheme to use IDs, and remove "renaming changes its slug and id".
  - Add `[id:]` to the tag list, with the rule that IDs are assigned on load and write.
  - Note that titles are stripped at the service layer and dates are validated in services.
  - Replace the `UpdateEdit`/`ApplyEdit` convention with the single `ApplyEdit` path.
  - Note deadlines on tasks.
- [ ] **`docs/backlog.md`:** remove "Duplicate slugs" and the `MoveToList` suffix from known limitations, and add follow-ups found during the plan. If `README.md` lists tags or slug URLs, update it too.
- [ ] Verify the docs against the code with parallel clean-context agents (the owner does not read docs).
- [ ] **Verification.** `make lint test` green and CI green. Back up the DB before deploying, because this migration drops tables. Deploy and record it. Check the success criteria.
- [ ] **STOP: plan complete.**

---

## Success criteria

1. Two tasks with the same title can each be completed, edited and trashed independently (Go and Playwright tests).
2. Renaming an item keeps its URL, its expanded state across a live refresh, and its commentary.
3. Moving a task between todos and family keeps its ID, deadline and commentary.
4. After Phase 2, every item line in production files carries an `[id:]`, recorded per file in Working notes.
5. A task with a due date shows the correct label in its row, in the planner when due or overdue, and in the "Due soon" widget, whose "Plan today" plans it and keeps focus. Nothing is added to the plan automatically.
6. Invalid dates and tag-carrying titles are rejected or stripped on every input path, with a table-test row per path.
7. Round-trip fixtures are byte-identical. Contrast, module contract, e2e selector and morph tests pass. Route golden diffs are explained in Working notes.
8. Lint, the race suite with `INTEGRATION=1`, and Playwright in CI pass.

## Risks

- **Phase 3 misses a slug reference** in templates or JS. Mitigations: deleting `Slug` makes Go and template uses fail loudly; the grep residue list; `TestE2ESelectorsExist`; the rename and duplicate-title specs; the STOP after deploy.
- **The cache lacks IDs that are on disk** if assignment builds a new slice. Assign in place; the resync-then-mutate test covers it.
- **Rollback after Phase 2.** An older binary reads `[id: x]` as part of the title, so its slugs change. Restore the backup with the previous image rather than running old code on new files. Never let the old image write a rewritten file: its writer moves the ID ahead of the other tags, so the new code no longer finds it (the tag must be last), shows it in the title and assigns a fresh ID; all IDs and relinked references are lost, and rolling forward does not repair it (council simulation, Phase 2 notes). Restoring the backup also loses every edit made since the deploy.
- **Deadline wiped by a partial edit.** Only `ApplyEdit` writes it, with nil meaning keep; tests cover the house page and the API.

---

## Working notes

One `### Phase N notes` section per phase: decisions, measurements, deploys (as defined under Requirements), route golden diffs, and follow-ups.

### Phase 1 notes

- Commits: `05198ad` (failing test), `bbc6346` (validation, one edit path), `8d67313` (due dates UI and widget), then the self-review fixes.
- Plan vs code: the reorder endpoints (`/plan/reorder`, API reorder) take no date, so there is nothing to validate there. `AddItem` validates both `Deadline` and `Planned`, which also covers the task quick-add form (it now takes a due date too) and `MoveToList`. A calendar-impossible date the parser regex accepts (`2026-02-30`, hand edits only) makes a move answer 400 rather than 500, and renders no label.
- `ErrInvalidDate` carries no user text, because `httputil.IsNotFound` matches on the error string; a date reading "not found" would otherwise be reported as a missing item. Handlers answer `tracker.InvalidDateMessage` (`Invalid date: use YYYY-MM-DD`).
- Severity: the widget sets the card's count colour (`danger` if anything is overdue, else `warning` if anything is due within 2 days), not `WidgetItem.Severity`. Item severity colours the row's link, and the plan also says the title link keeps its normal colour; the card count was the reading that satisfies both. Do not "fix" this.
- Label details decided here: one day out reads "due tomorrow" (not the weekday); the year shows only beyond 300 days. Goals hide the due badge once done or once the target is reached. Days are counted from civil dates in UTC after taking today's date in `loc`, so DST days are not 23 or 25 hours.
- Widget contract: `WidgetItem` gained `ID` (row id `<card id>-<ID>`, `data-row`, the link gets `data-row-focus`), `Context` (the "Family" word), `Meta` (`Text`, screen-reader `Label`, `Level`) and `Action` (`Path`, `Fields`, `Text`, `Label`, `Done`). `Registry.Widgets` drops an action whose path fails `httputil.IsLocalPath` and clears an unknown meta level (`TestModuleRegistryChecksWidgetItems`). Card headings now carry `tabindex="-1"` as the focus fallback. "Planned" covers carried-over tasks (`Planned <= today`).
- The "plan today" button's visible text is not part of its accessible name ("Plan {title} for today", as the plan specified); WCAG 2.5.3 best practice would include it. Left as specified.
- The e2e fixture gains an always-overdue task (`Lodge the tax return`, deadline 2026-01-01) so the axe passes see the danger badge, the Due soon card and the planner label in every style and theme.
- Route goldens unchanged.
- Tooling inside the sandbox: lint runs locally with `GOLANGCI_LINT_CACHE="$TMPDIR/gl-cache"`; e2e type-checks with `npm_config_cache="$TMPDIR/npm-cache" npx -y -p typescript@5 tsc --noEmit -p e2e`.
- Self-review (independent agent): nothing serious. Fixed: the empty plan-row span and the move 500 for impossible stored dates, axe coverage via the fixture. Recorded: the severity reading and the accessible-name note above.- Deploy 2026-10-07: commit `bf5449e`, CI run 37609971283 green (lint, test, vuln, e2e, screenshots, build). Image revision label `bf5449eaef45a3fcf8b03aa776439a6e80d8c6ad`. Backup `dashboard-backup-20261007-122724.tar.gz` (exit 0). Caddy checks: `/login` 200, `/todos` 303, `/events` 401, `/api/v1/todos` 404, cross-site POST `/login` 403. `verify-stack.sh` all green.
- Owner decisions at the STOP: keep the age badge on rows with a due date (both are worth seeing); the label wording stands.

### Phase 2 notes

- Commits: `2bdf66d` (failing injection tests), `55dcee8` (stripping), `05e3df2` (itemid, parsers, renderers, fixtures), `6c0ec9d` (assignment, moves, references), `53f7a6a` (failing tests from the review), then the review fixes.
- Injection, beyond titles: tags, image entries, goal units and idea projects are cleaned at render time (`httputil.CleanMetaValue`/`CleanMetaList`: brackets, line breaks, commas in list values, priority markers). Without that a tag `ok] [planned: 2026-01-01` or `!high` set metadata. A parsed value cannot hold these (except `[`, which no production value holds; checked before deploy), so writing parsed items back is unchanged and the round-trip fixtures stay byte-identical. Captions now also drop `[`. Titles are cleaned at the service layer (`httputil.CleanTitle`: tags, unclosed tags, priority markers, line breaks); a title left empty answers the form's title-required message, not a 500.
- `inlineMetaRe` gains `id` and `project`.
- The plan's "an ideas-file indented checkbox before any idea" cannot be in a byte-identical fixture (the writer unindents it), so it is a parse row in `TestParseItemIDs` instead.
- Load-time assignment saves without publishing (`changes.Recorder.Record`): the load, or the watcher event that caused the resync, already tells open pages, and publishing gave an external edit two events. If that save fails, the cache keeps the file's (missing) IDs.
- References: `[from-idea:]`/`[converted-to:]` slugs become IDs in `services.linkReferences`, run by `Registry.ForUser` only when one of the user's or shared services assigned IDs on its load, so a slug left unresolved at deploy never adopts a later item with the same title. Family and house resolve against the first user loaded (the owner). A reference the owner's ideas leave unresolved can still be linked by a later user's first load in the same process, because the shared services keep reporting `AssignedOnLoad`; production has one user, and multi-user code is frozen, so this is accepted.
- Plan vs code: Phase 2 was meant to change nothing visible, but references now hold IDs while routes still take slugs. A task's "From idea" link pointed at `/ideas/<slug>`, so the idea page now also opens by ID (temporary, until Phase 3's `{id}` routes) and the link reads "the original idea" (Phase 3's wording, brought forward). The idea page's "Converted to a task" anchor (`/todos#item-<id>`) does not scroll to the row until Phase 3. API JSON gains `id` (todos, plan, ideas) next to `slug`.
- Route goldens unchanged.
- Self-review (independent agent): no data-loss or ID-loss path. Fixed test-first: priority markers through tags, title-of-only-tags 500s, relinking on every load. Also fixed: cache/file mismatch after a failed load-time save, `refIndex` ignoring invalid IDs, `[` in captions.
- Production baseline before deploy (2026-10-09): `users/1/personal.md` 35 items, `users/1/ideas.md` 15 (4 converted), `data/family.md` 8, `data/maintenance.md` 0, `data/house-projects.md` 0; no `[id:]` anywhere. All idea statuses valid; no tag, image, project or unit value holds `[`. `users/legacy/` and the root-owned `data/personal.md` are not loaded by the app and stay ID-less.
- Council review before deploy (owner's request, three agents):
  - Data: ran the new start-up over a copy of the 2026-10-09 backup (`dashboard-backup-20261009-142539.tar.gz`). Only changes: 58 IDs appended (personal 35, ideas 15, family 8; all unique, one per unindented item line, none on indented lines) and 8 references relinked (4 `from-idea`, 4 `converted-to`, each pair consistent), 0 unresolved. Line counts unchanged. A second start-up is byte-identical. The backup is byte-identical to production, so restoring it is exact.
  - Security: no title path sets any tag. One hole, fixed test-first: `CleanMetaValue` removed priority markers in one pass, so `!hi!highgh` became `!high` in a tag, caption or image name and set the priority. It now repeats until stable.
  - Correctness and operations: no lost write, lost or duplicated ID, or cache/file mismatch; `go test -race -count=3` on the ID, event, resync, lazy-load and move tests passes. Reference linking runs only on the load that assigns IDs and is never retried, so a crash between assignment and linking, or a failed relink write, leaves slugs for good (Phase 3's `{id}` routes would then not resolve them). Accepted with a deploy check: after start-up, count `[from-idea:]`/`[converted-to:]` values that are not IDs; expect 0. Also noted: the assign/save helpers are repeated in the three services, like the services themselves (accepted).
  - Carried into Phase 3: `MoveToList` must use the ID `AddItem` returns (a collision gives the moved item a new one); items left without an ID by a failed load-time save cannot be addressed until the next write assigns one; delete the idea page's ID fallback with the `{id}` routes.
- First CI run of Phase 2 (`9f35e26`): vuln failed on advisories published after the last run: Go 1.27.2 (net/http, net/textproto, html/template, os) and `golang.org/x/net` 0.60.0. Bumped `go.mod` to 1.27.2, x/net to 0.60.0 and the Dockerfile builder to `golang:1.27.2-alpine` (digest pinned). The sandbox could not download the 1.27.2 toolchain, so the race suite ran on 1.27.1 with a copy of `go.mod` set to 1.27.1 and the new x/net; CI runs it on 1.27.2. The non-gating screenshots job also failed; see the next run.
- Deploy 2026-10-09: commit `d7c190e`, CI run 37951569425 green (lint, vuln, test, e2e, screenshots, build; the screenshots failure on `9f35e26` did not recur). Image revision label `d7c190ee54503464ea9dba7f58f5339f691e26b0`. Backup `dashboard-backup-20261009-153755.tar.gz` (exit 0). Start-up log: `from-idea references now hold IDs count=4`, `converted-to references now hold IDs count=4`, no unresolved or malformed-ID warnings. Caddy checks: `/login` 200, `/todos` 303, `/events` 401, `/api/v1/todos` 404, cross-site POST `/login` 403. `verify-stack.sh`: one check failed on the first run and all passed on an immediate re-run.
- Items and IDs per production file (unindented item lines; IDs counted as a final valid `[id:]`; references counted as `[from-idea:]`/`[converted-to:]` values that are not IDs):

  | File | Before (backup): items / IDs / slug refs | After start-up: items / IDs / slug refs |
  |---|---|---|
  | `users/1/personal.md` | 35 / 0 / 4 | 35 / 35 / 0 |
  | `users/1/ideas.md` | 15 / 0 / 4 | 15 / 15 / 0 |
  | `data/family.md` | 8 / 0 / 0 | 8 / 8 / 0 |
  | `data/maintenance.md` | 0 / 0 / 0 | 0 / 0 / 0 |
  | `data/house-projects.md` | 0 / 0 / 0 | 0 / 0 / 0 |

  No ID repeats across the files. No file has items without IDs, so the phase passes.
- Owner sign-off 2026-10-09: pages look right after the rewrite. Phase 3 starts in a fresh session; its inputs are the "Carried into Phase 3" bullet above, the rollback hazard in Risks, and the reference-count check (`[from-idea:]`/`[converted-to:]` values that are not IDs, now 0 in production).

### Phase 3 notes

- Commits: `4131687` (failing tests: duplicate titles, rename keeps the URL), `6b41719` (addressing by ID), `355efc8` (failing test from the review: negative sub-step index), `fe00758` (its fix).
- `itemid.Route` (`{id:[b-df-hj-np-tv-xz0-9]{8}}`) is the one route pattern; `TestItemIDRouteMatchesAlphabet` holds it to `itemid.Alphabet`. Lists of IDs (`ids` form field, API reorder) go through `itemid.ParseList`/`AllValid`, which reject the whole list if any entry is not an ID. Service lookups never match an empty ID, so an item left ID-less by a failed load-time save cannot be hit by accident; it stays unaddressable until the next write assigns one (accepted in Phase 2).
- Plan vs code: the commentary routes keep `{list}` (validated, then unused), because the plan names `/commentary/{list}/{id}`; the key is `(item_id, user_id)`. `PurgeExpired` now returns the purged IDs, so the hourly purge removes their commentary; the web purge routes (tasks, goals, ideas, maintenance, house projects) do the same. `MoveToList` copies commentary when the target had to give the item a new ID. Existing rows in the old `commentary` table are not copied (production never had any). Old bulk and plan form fields (`slug`, `slugs`) answer 400; old `/todos/<slug>/...` URLs answer 404 from the router, so a tab open across the deploy cannot write to the wrong item. `ListForSlugs`/`HasCommentary` had no production caller and were deleted.
- Route goldens: both files change only from `{slug}` to `{id:[b-df-hj-np-tv-xz0-9]{8}}`, and `GET /exploration/{slug}` becomes `GET /exploration/*` (redirects to `/ideas`). Auth and no-auth goldens remain identical.
- Tests whose expectations changed on purpose: renames (`TestUpdateEditTitle`, the ideas edit tests, `TestTrackerServiceApplyEdit`) now keep the ID; `TestIdeasServiceEdit_TitleCollision` became `_DuplicateTitles`; a missing item is addressed by a valid unknown ID (`n0n3x1st`) to reach the handler's 400. Three bulk-date tests had been getting their 400 from the missing field rather than the bad date; fixed.
- e2e: fixtures carry fixed IDs (`fixtureIds` in `e2e/tests/helpers.ts`), plus a same-title pair (`Call the bank`, `bnkc4ll1`/`bnkc4ll2`). New specs: `tasks.spec.ts` "two tasks with the same title are completed and trashed independently", `sse.spec.ts` "renaming an expanded item keeps its id and expanded state, here and in another tab".
- `grep -rn -i slug internal/ web/` residue, each kept on purpose: `internal/slug` (the slugifier); `services/refs.go` (resolving older files' slug references, using `slug.Slugify` on titles); `cmd/dashboard/migrate.go` (legacy data matches research files by slugified title); `db/migrations.go` (old `tracker_items` and `commentary` tables, dropped in Phase 5); comments on `FromIdea`/`ConvertedTo` and in `services/registry.go` and `modules/ideas/ideas.go` about older slug values. Nothing in `web/`.
- Self-review (independent agent): nothing serious. Fixed test-first: `PromoteSubStep` panicked on a negative index (500; predates this phase). Added: commentary removal tests for the idea, maintenance and project purge routes and the colliding move. Not reachable from `test/`: the hourly `appServices.purgeExpired` (unexported); it calls the same `ForgetItems` with the IDs `PurgeExpired` returns, which is tested.
- Council, security: no blockers. Low: commentary cleanup (`DeleteItems`, `Copy`) acts on an ID for every user, so with a second user a chance ID collision across two files could remove the other user's commentary. Info: the commentary API's `{list}` is ignored; with several users, old slug references on shared lists resolve against the first user loaded (Phase 2). All need a second user; multi-user code is frozen.
- Council, deploy safety (built `6b41719` and the Phase 2 image's commit `d7c190e`, both in auth mode on fixtures plus Phase-2-shaped files): GO. Start-up and page reads leave every file byte-identical, twice. The migration adds one statement (schema 21 to 22, `CREATE TABLE IF NOT EXISTS item_commentary`); the Phase 2 binary starts on a v22 DB and serves every page, and its writes keep the `[id:]` tags, so rollback needs only the image pin (plus the backup if anything must be undone). Sixteen stale-tab posts (slug URLs, `slug=`/`slugs=` fields, old `?undo=` links) answered 404 or 400 and changed no file. Deploy checks added: before, `select version from schema_version` gives 21 and `select count(*) from commentary` gives 0; `shasum` of the three production files before stopping the old container and after the new one has served `/`, `/todos` and `/ideas` must match; after, the schema version is 22; hard-reload open tabs.
- Accepted residue: items left without an ID (a failed load-time save) share the key `""` in the planner's exclude maps and give duplicate DOM ids until a write assigns them IDs.
- Follow-ups (for `docs/backlog.md` in Phase 5): the idea "Converted to a task" link always points at `/todos#item-<id>`, even for family and house tasks (predates this phase); `APIReorderPlan` answers 500 for a well-formed ID not in the list (predates this phase, API off in production).
- Tooling: the sandbox cannot download Go 1.27.2, so tests ran with `GOFLAGS=-modfile=$TMPDIR/dash-mod/go.mod` (a copy pinned to 1.27.1) and lint ran with `go.mod` briefly set to 1.27.1, then restored. `TestMigrateDataNeverOverwritesExistingIdeas` builds a binary and needs the `GOFLAGS` form.
- Deploy 2026-10-10: commit `40b40e7`, CI run 38007753765 green. Image revision label `40b40e77056a3917b68d90b30301d1445965d480`. Backup `dashboard-backup-20261010-024228.tar.gz` (exit 0). Before: schema version 21, old `commentary` table 0 rows, no `item_commentary` (read from the backup's DB snapshot; the server has no `sqlite3`). SSH to fliptronic timed out for several minutes during the pre-checks and recovered unaided; nothing had changed in production meanwhile. The sha256 of all five data files was identical before the pull and after start-up. After: schema version 22 with `item_commentary` (from a second backup, `dashboard-backup-20261010-031229.tar.gz`). Caddy checks: `/login` 200, `/todos` 303, `/events` 401, `/api/v1/todos` 404, cross-site POST `/login` 403. `verify-stack.sh` all green.
- Owner sign-off 2026-10-11: rename while expanded, same-title tasks, plan reorder, bulk plan from todos, search and widget links all work in the live app. New requests from that use (homepage plan: untick a done task, bulk select with complete, drop and trash) go through a design council before any code.

### Phase 4 notes

- Commits: `337dfee` (failing test: house ticks leave the status), `acb7960` (fix), `4474c71` (untick and plan bulk select), then the review fixes.
- House status: fixed in the service rather than per handler. Only house projects carry a `[status:]`, so `Complete` and `BulkComplete` set it to `done` when present and `Uncomplete` turns `done` back into `todo`. A hand-edited house line without a status is ticked without gaining one (accepted: the app always writes a status).
- Plan vs code: the "move to tomorrow" action was added at the owner's request. Bulk forms are plain posts (as on the todos page), so after one the page reloads with select mode off and focus at the top, not on the select toggle. Counted flash messages use `?msg=<key>&n=<count>`; the homepage gained one error key, `plan-bulk-failed`, for a write failing after validation (a race). The coarse-pointer 44px minimums for checkboxes and `.btn` already existed in the utilities layer.
- Dropped after the review: the hover and focus preview of an unticked circle on done ticks. The row stays put after a tick, so the pointer (and on iOS the sticky `:hover`) made a freshly ticked circle look open. The tooltip "Mark not done" and the pointer cursor remain.
- Route goldens: five new routes in both files (`/plan/{id}/uncomplete`, `/plan/bulk/{complete,tomorrow,clear,delete}`); auth and no-auth stay identical.
- Self-review (independent agent): nothing serious. Fixed: the done-tick preview above; up and down buttons in an expanded row still worked in select mode (entering select mode now collapses plan rows, and a button click no longer toggles the row's checkbox); `personal:x,todos:x` counted twice; the trash confirm said "1 items". Accepted: a done row's untick stays reachable by keyboard in select mode (pointer taps are off).
- First CI run (`e3e65cf`): e2e failed. The plan's select-mode rules were in the `components` layer, but `.plan-item > .plan-tick { display: flex }` is in `pages`, which wins whatever the specificity, so the tick stayed visible over the checkbox and took its clicks (the axe and bulk specs). They now sit beside the plan row rules in `pages`. The reorder spec failed as a knock-on: the failed axe runs left twelve rows on today's plan, more than its 20-move loop can bubble past; the axe spec now drops its two rows afterwards.
- Deploy 2026-10-10: commit `1e7b54e`, CI run 38070956768 green (e2e, lint, test, screenshots, vuln, build). Image revision label `1e7b54ebd7a30fc619debde61576c4cd4ab43d57`. Backup `dashboard-backup-20261010-173110.tar.gz` (exit 0). No migration. The sha256 of all five data files was identical before the pull and after start-up. Caddy checks: `/login` 200, `/todos` 303, `/events` 401, `/api/v1/todos` 404, cross-site POST `/login` 403. `verify-stack.sh`: `dash` OK on every run; other services (cca, otterholt, brotato) failed intermittently with `200000` (a 200 followed by a timed-out retry), a different set on each run, and its local sudo step cannot run in the sandbox.
- Owner use after the deploy found three bugs, fixed test-first (`5866f57` failing tests, `234ca7e` fixes): a task carried over from yesterday read "from today" (`relativeDate` subtracted the date's UTC midnight from now's instant; now `insights.DaysAgo`, counting calendar days like `DueLabel`); a carried task vanished from the plan when ticked (`ListOverdue` returns open items only; carried tasks completed today now stay on the plan as done rows, and the carried count counts open ones); the toast's undo button showed with nothing to undo (`.btn`'s display beat `[hidden]`), and a plan tick now offers undo (`/plan/{id}/uncomplete?list=`). This also fixes `relativeDate` for its existing use, which the plan's non-goals had left alone. Deployed 2026-10-10: CI run 38074383597 green, image revision `234ca7eab7d0190882bb91913913d4507af75653`, backup `dashboard-backup-20261010-182116.tar.gz` (exit 0), data files byte-identical across the restart, Caddy checks as before, `verify-stack.sh` `dash` OK (`cca` failed with the intermittent `200000` again).
