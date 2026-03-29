# Backlog

## House Tab Follow-ups

### ServiceMap refactor for tracker/api.go
Refactor `resolveListService` from positional `personal, family *Service` parameters to a `ServiceMap` type (`map[string]*Service` with `Resolve` method). This eliminates the parameter cascade when adding new list types. Currently all ~12 API handlers take positional params. The house projects API is served through `/house/projects/*` routes, not `/api/v1/todos`, so this is not blocking.

### planner.js DnD for house list
The HTML5 drag-and-drop in `planner.js` does not explicitly handle `data-list="house"` -- structural DnD works via form posts but the JS-side reorder logic may not correctly identify house items. Needs testing and a `"house"` case in the DnD handlers.

### itemToAPI Budget/Actual/Status
The `itemToAPI` function in `tracker/api.go` does not expose Budget, Actual, or Status fields in API responses. Add these when an API consumer needs them.

### MoveToList for house projects
Moving items between personal/family/house lists is not wired. The infrastructure supports it (house projects use `*tracker.Service`), but no route or handler exists for moving to/from house.

## Security

### CSRF protection
No CSRF protection on any POST form. Needs `gorilla/csrf` or `nosurf` middleware. Affects the entire app, not just house.

### Ideas status CSS class injection
`ideas.Status` is parsed from markdown without allowlist validation. A crafted `[status: x badge-tag]` value would inject CSS classes. Low severity -- Go's `html/template` prevents XSS but cosmetic injection is possible. Add allowlist validation matching `tracker.SanitiseStatus` pattern.

### Image filename path traversal in templates
`SplitImageCaption` does not validate that filenames are safe before rendering in `img src`. The server-side `http.Dir` prevents file-serving traversal, but the HTML `src` attribute could reference arbitrary same-origin paths. Validate filename contains no `/` or `..`.

## Accessibility

### Focus traps for search and shortcut modals
Search overlay and shortcut help modal have `role="dialog"` and `aria-modal` (added) but no JS focus trap. A keyboard user can Tab out of the overlay into the page behind it.

### Calendar drag-and-drop keyboard alternative
Calendar tasks are `<div>` elements with `draggable="true"` but no keyboard interaction handler. Need a keyboard-accessible alternative to drag-and-drop for rescheduling.
