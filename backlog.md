# Backlog

## Backlog

### L (5+ hours each)

- **Soft delete / trash** -- Items move to trash instead of permanent deletion. "Recently Deleted" section with restore button. Auto-purge after 7 days. Addresses no-undo gap.
- **Bulk actions** -- Checkbox select mode with bulk complete/delete/retag/reprioritise. Reduces repetitive clicking when managing many items.
- **Weekly/monthly digest view** -- "This week" tab showing completed, added, converted, triaged counts. Small bar chart or summary. Positions dashboard as reflective tool.
- **Image captions** -- Add caption field to uploaded images. Currently images are inert thumbnails with no context.

### Ideas

- **Ironclaw PA commentary on dashboard items** -- Integrate commentary from ironclaw (Claude instance, currently AWS-hosted, may move to local nanoclaw) into the dashboard. Ironclaw would write its thoughts, suggestions, and actionable guidance for each task, exploration item, and idea to a separate data location. The dashboard reads and displays this commentary when a user clicks into an item. Ironclaw's prompt and behaviour are managed separately (via Slack); the dashboard only needs to consume and render the linked commentary data.
- **Graduated field resurrection or removal** -- `Graduated` exists in the data model but is unused in UI. Either remove it (simplify) or resurrect as "completed N times, now a habit" celebration.
- **Personality slider** -- User preference for interface tone: minimal vs chatty vs playful. Adjusts copy and animations per preference.
- **Seasonal/contextual touches** -- Reflect time of day, seasons, or streaks in the visual language.

## Done

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
