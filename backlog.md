# Backlog

## UI/UX Improvements

1. **[L] Combine Experiments and Ideas** — Merge two separate packages (internal/ideas/, internal/exploration/), consolidate storage/handlers/routes/templates. Keep park/drop/to-personal actions, drop rigid category system
2. **[S] Reorder top navigation** — Reorder links in layout.html. Blocked by #1. New order: Todos, Goals, Ideas, Family

> **Legend:** S = small (< 1hr), M = medium (schema + multiple files), L = large (multi-package refactor)
>
> **Sequencing:** 1 is the heavy lift and blocks 2.

## Ideas

- **Ironclaw PA commentary on dashboard items** — Integrate commentary from ironclaw (Claude instance, currently AWS-hosted, may move to local nanoclaw) into the dashboard. Ironclaw would write its thoughts, suggestions, and actionable guidance for each task, exploration item, and idea to a separate data location. The dashboard reads and displays this commentary when a user clicks into an item. Ironclaw's prompt and behaviour are managed separately (via Slack); the dashboard only needs to consume and render the linked commentary data.

## Done

- [Display first name in nav + self-service account page](plans/first-name-display.md)
- [Fix login redirect on SSE errors](plans/quick-wins.md)
- [Remove "View All" buttons on home page](plans/quick-wins.md)
- [Single column layout on mobile home page](plans/quick-wins.md) (already implemented)
- [Simplify mobile list items](plans/quick-wins.md)
- [Two-row mobile nav](plans/quick-wins.md)
- [Rename "Personal" to "Todos"](plans/quick-wins.md)
- [Admin UI for user management](plans/admin-ui.md)
- [Multi-user support](plans/multi-user.md)
- [Password authentication](plans/password-auth.md)
- [Split tasks into personal/family + add homepage](plans/split-tasks-and-homepage.md)
