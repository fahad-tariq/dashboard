# Backlog

## Backlog

### S (under 2 hours)

- **Edit form drops image captions** -- Investigated: the JS init path correctly populates caption fields even inside collapsed `<details>`. Manual testing needed to confirm, but code analysis shows this is likely not-a-bug.

### M (2-5 hours)

- **Planner: drag-and-drop scheduling** -- Reorder tasks within the daily plan and drag between days in calendar view.
- **Planner: ironclaw API integration** -- Automated triage via the ironclaw agent using `PUT /api/v1/plan/{slug}` to plan tasks.

### Ideas

- **Ironclaw PA commentary on dashboard items** -- Integrate commentary from ironclaw (Claude instance, currently AWS-hosted, may move to local nanoclaw) into the dashboard. Ironclaw would write its thoughts, suggestions, and actionable guidance for each task, exploration item, and idea to a separate data location. The dashboard reads and displays this commentary when a user clicks into an item. Ironclaw's prompt and behaviour are managed separately (via Slack); the dashboard only needs to consume and render the linked commentary data.
- **Personality slider** -- User preference for interface tone: minimal vs chatty vs playful. Adjusts copy and animations per preference.
- **Seasonal/contextual touches** -- Reflect time of day, seasons, or streaks in the visual language.

## Done

- Planner calendar view: `/plan/calendar` with week/month toggle, prev/next navigation, responsive grid, `g c` keyboard shortcut
- Consolidated `ParseCSV` into `httputil.ParseCSV` (removed duplicate from ideas package)
- Removed unused `Graduated` field from tracker data model (DB column left as harmless DEFAULT 0)
- Daily planner: `[planned: YYYY-MM-DD]` inline metadata, homepage redesign with Today's Plan section (progress bar, carried-over tasks, task picker with filter), plan/unplan/complete from homepage, plan button on tracker items + bulk plan action, API endpoints (GET/PUT/DELETE `/api/v1/plan`), planner tests (8 tests)
- Soft delete / trash: items move to trash with `[deleted:]` metadata, "Recently Deleted" sections, restore/purge, auto-purge after 7 days
- Bulk actions: checkbox select mode, bulk complete/delete/priority/tag, `mutateBatch` atomicity, sticky bulk bar
- Weekly/monthly digest view: `/digest` page with period-specific activity summaries
- Image captions: inline `filename|caption` storage, upload form caption fields, sanitisation
- UX/UI overhaul: success flash messages on all 15 mutation endpoints, warm copy, empty state rewrites with family-specific text, button label consistency, onboarding help text, idea status legend, time-of-day greeting, form placeholder improvements
- Visual feedback: loading indicator pulse animation, filter-active badge, task completion celebration (green flash with beforeSwap delay), idea triage fade-out animations, 7 styled confirmation modals replacing browser confirm()
- Typography and layout: base font 14px to 16px, WCAG AA contrast for --fg-dim/--fg-muted, 2-line clamp on homepage cards, mobile action button consolidation behind details/summary menu, container max-width 1100px to 1200px
- Status and errors: contextual error messages replacing generic "Bad request" across all handlers, httputil.ServerError with 8-char correlation IDs, idea status badges on list cards, session expiry toast with 2s delay before login redirect
- Progress insights: internal/insights package (AgeBadge, WeeklyVelocity, Streak, MilestoneBadge, GoalPace, ProgressColour, TagAggregation), homepage micro-insights with velocity/streaks/milestones/tag summaries, age badges on tasks and ideas, goal progress bar colour shift by deadline proximity, pace indicators, deadline field on goals
- Idea-to-task conversion: bidirectional linkage (FromIdea on tasks, ConvertedTo on ideas), converted status preserves ideas instead of deleting, collapsed converted section in ideas list, ToTaskFunc returns task slug
- Power user features: Ctrl+K/Cmd+K/slash search overlay with full-text search across tasks/goals/ideas, keyboard shortcut help modal (?), go-to navigation (g h/t/o/i/f), arrow key result selection, search-results.html standalone fragment template
- HTTP handler tests: tracker (18 tests), ideas (13 tests), homepage (4 tests), account (7 tests)
- Editable titles for tasks, goals, and ideas (service, handler, and template changes)
- Security hardening: bluemonday HTML sanitisation, linkify URL validation, URL-encoded auth redirects, MaxBytesReader on API, generic error messages, SecureCookies default true, current password required for change, API token warning
- Architecture: extract main.go (account/, home/, migrate.go, mountAppRoutes), mutate() helpers, auth.TemplateData, httputil.WriteJSON, dedup parseTags/priorityWeight/route registration/auth context injection, narrow registry mutex, safer MoveToList, transactional migrations, session cleanup shutdown, go mod tidy
- Performance: in-memory cache for tracker/ideas, upsert-by-slug, DB index, Cache-Control headers, defer scripts, scoped SSE, single-pass homepage
- Accessibility: ARIA labels, form labels, keyboard-accessible divs, alt text, fg-muted contrast, role=alert flash messages, nav breadcrumbs, focus-visible fix, priority select Apply button, mobile account link
- UI/UX: flash error colours, upload error feedback, SSE loading indicators, 44px touch targets, inline form errors, inline password confirmation, clickable badge distinction, required field indicators, graduated CSS removal, inline styles extracted
- Testing: tracker service mutations (16 tests), ideas service edit (5 tests), API endpoints (5 tests), upload handler (5 tests), SSE broker (4 tests), config validation (4 tests), insights (8 tests), search (6 tests), httputil (3 tests), auth expiry (2 tests), parser round-trips (5 tests)
- Combine ideas and explorations with flat-file storage
- Reorder top navigation
- Display first name in nav + self-service account page
- Fix login redirect on SSE errors
- Remove "View All" buttons on home page
- Single column layout on mobile home page
- Simplify mobile list items
- Two-row mobile nav
- Rename "Personal" to "Todos"
- Admin UI for user management
- Multi-user support
- Password authentication
- Split tasks into personal/family + add homepage
