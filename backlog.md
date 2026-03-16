# Backlog

## UI/UX Improvements

1. **[M] Display first name instead of username in top nav** — New FirstName field on User struct + DB migration + update admin/registration UI + middleware + layout template
2. **[S-M] Investigate login redirect issue** — Home page redirects to login frequently, possibly on focus/tab switch. Likely session lifetime or SSE 401 triggering re-auth
3. **[S] Remove "View All" buttons on home page** — Template-only change in homepage.html, headings already link
4. **[S] Single column layout on mobile home page** — CSS media query change to collapse the 3fr/2fr grid
5. **[S] Simplify mobile list items** — CSS hide tags/dates on mobile within homepage cards
6. **[L] Combine Experiments and Ideas** — Merge two separate packages (internal/ideas/, internal/exploration/), consolidate storage/handlers/routes/templates. Keep park/drop/to-personal actions, drop rigid category system
7. **[S] Two-row mobile nav** — CSS flex-wrap on nav container at mobile breakpoint
8. **[S] Rename "Personal" to "Todos"** — Text changes in templates + route rename /personal to /todos + list name constant
9. **[S] Reorder top navigation** — Reorder links in layout.html. Blocked by #8 and #6. New order: Todos, Goals, Ideas, Family

> **Legend:** S = small (< 1hr), M = medium (schema + multiple files), L = large (multi-package refactor)
>
> **Sequencing:** 3, 4, 5, 7 are independent quick wins. 8 before 9. 6 is the heavy lift and blocks 9. 1 is independent. 2 needs investigation first.

## Ideas

- **Ironclaw PA commentary on dashboard items** — Integrate commentary from ironclaw (Claude instance, currently AWS-hosted, may move to local nanoclaw) into the dashboard. Ironclaw would write its thoughts, suggestions, and actionable guidance for each task, exploration item, and idea to a separate data location. The dashboard reads and displays this commentary when a user clicks into an item. Ironclaw's prompt and behaviour are managed separately (via Slack); the dashboard only needs to consume and render the linked commentary data.

## Done

- [Admin UI for user management](plans/admin-ui.md)
- [Multi-user support](plans/multi-user.md)
- [Password authentication](plans/password-auth.md)
- [Split tasks into personal/family + add homepage](plans/split-tasks-and-homepage.md)
