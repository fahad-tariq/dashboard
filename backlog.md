# Backlog

## Ideas

- **Ironclaw PA commentary on dashboard items** -- Integrate commentary from ironclaw (Claude instance, currently AWS-hosted, may move to local nanoclaw) into the dashboard. Ironclaw would write its thoughts, suggestions, and actionable guidance for each task, exploration item, and idea to a separate data location. The dashboard reads and displays this commentary when a user clicks into an item. Ironclaw's prompt and behaviour are managed separately (via Slack); the dashboard only needs to consume and render the linked commentary data.

## Done

- HTTP handler tests: tracker (18 tests), ideas (13 tests), homepage (4 tests), account (7 tests)
- Editable titles for tasks, goals, and ideas (service, handler, and template changes)
- Security hardening: bluemonday HTML sanitisation, linkify URL validation, URL-encoded auth redirects, MaxBytesReader on API, generic error messages, SecureCookies default true, current password required for change, API token warning
- Architecture: extract main.go (account/, home/, migrate.go, mountAppRoutes), mutate() helpers, auth.TemplateData, httputil.WriteJSON, dedup parseTags/priorityWeight/route registration/auth context injection, narrow registry mutex, safer MoveToList, transactional migrations, session cleanup shutdown, go mod tidy
- Performance: in-memory cache for tracker/ideas, upsert-by-slug, DB index, Cache-Control headers, defer scripts, scoped SSE, single-pass homepage
- Accessibility: ARIA labels, form labels, keyboard-accessible divs, alt text, fg-muted contrast, role=alert flash messages, nav breadcrumbs, focus-visible fix, priority select Apply button, mobile account link
- UI/UX: flash error colours, upload error feedback, SSE loading indicators, 44px touch targets, inline form errors, inline password confirmation, clickable badge distinction, required field indicators, graduated CSS removal, inline styles extracted
- Testing: tracker service mutations (16 tests), ideas service edit (5 tests), API endpoints (5 tests), upload handler (5 tests), SSE broker (4 tests), config validation (4 tests)
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
