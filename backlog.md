# Backlog

## Ideas

- **Ironclaw PA commentary on dashboard items** — Integrate commentary from ironclaw (Claude instance, currently AWS-hosted, may move to local nanoclaw) into the dashboard. Ironclaw would write its thoughts, suggestions, and actionable guidance for each task, exploration item, and idea to a separate data location. The dashboard reads and displays this commentary when a user clicks into an item. Ironclaw's prompt and behaviour are managed separately (via Slack); the dashboard only needs to consume and render the linked commentary data.
- **Lightweight authentication** — Add auth to the dashboard. Options to evaluate: OIDC (e.g. via an external provider), or a simple username/password scheme. Priority is security without heavy infrastructure -- no user database if avoidable, no complex OAuth flows. Could be as simple as a hashed password in an env var with a session cookie, or a reverse proxy auth layer.
- **Multi-user support** — Allow multiple users to each have their own isolated view of personal/family/goals/ideas/explorations. Each user gets their own data directory (or namespaced files). Depends on authentication being in place first. Key decisions: shared family list vs per-user, data isolation model (separate directories vs database-level), and how user identity maps to data paths.

## Done

- [Split tasks into personal/family + add homepage](plans/split-tasks-and-homepage.md)
