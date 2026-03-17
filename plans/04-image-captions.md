# Image Captions

## Overview

Add optional captions to uploaded images. Stored inline using pipe delimiter: `[images: abc.png|Caption text, def.jpg]`. Captions appear below thumbnails in read views and are editable in edit forms.

## Design Decision: Parallel Form Fields

Captions are stored as `filename|caption` in markdown, but they do NOT flow through form submission that way. The existing `images` hidden input remains a comma-separated list of filenames only. Captions are sent as separate form fields (`caption-0`, `caption-1`, etc.), indexed by position. Handlers reconstruct `filename|caption` pairs server-side before storing.

This avoids breaking `ParseCSV()`, which splits on commas -- a pipe-delimited caption containing a comma would corrupt parsing. Upload handler response is unchanged; captions are added after upload in edit forms.

Pipes, commas, and `]` characters are stripped from captions both client-side (on input) and server-side (in handlers). Server-side validation is authoritative; client-side stripping is a courtesy.

## Design Decision: template.HTML Constraint

`splitImageCaption` MUST return plain strings, never `template.HTML`. Captions are auto-escaped by Go's `html/template` in both element content and attributes. Do NOT follow the `linkify` pattern (which returns `template.HTML`) -- doing so would bypass all XSS escaping.

## Design Decision: Hidden Input Population

Templates populate the `images` hidden input with `{{$img}}` from `.Images`, which after this change contains `filename|caption` strings. The FuncMap must also provide a `imageFilename` helper that extracts just the filename for the hidden input value. Templates use `imageFilename` when rendering the hidden input, and `splitImageCaption` when rendering the gallery display.

## Success Criteria

1. Existing images without captions work identically
2. Images with `file.png|My caption` round-trip through parse/write
3. Captions appear below thumbnails in all four gallery views
4. `alt` attribute uses caption when present
5. Edit forms allow caption entry/editing per image
6. Forbidden characters stripped server-side; client-side stripping as courtesy
7. All existing tests pass; new tests cover caption round-trips

---

## Development Plan

### Phase 1: Shared Helpers and Parser Verification

- [ ] Add `SplitImageCaption(entry string) (filename, caption string)` and `JoinImageCaption(filename, caption string) string` to `internal/httputil/` -- both tracker and ideas parsers need these; placing in `httputil` avoids import cycles
- [ ] Add `SanitiseCaption(caption string) string` to `internal/httputil/` that strips pipes, commas, `]`, `<`, `>`, and `"` characters (defence in depth for code paths that skip template escaping). Add a `maxCaptionLength` constant (200 chars) and truncate
- [ ] Verify `imagesRe` regex in both parsers captures pipe-delimited entries (it does -- `.*?` stops at `]`)
- [ ] Verify `writeItem` (tracker) and `WriteIdeas` (ideas) serialise image entries verbatim via `strings.Join` (they do)
- [ ] Unit tests for `SplitImageCaption`/`JoinImageCaption` edge cases (no pipe, empty caption, multiple pipes)
- [ ] Round-trip tests for tracker and ideas with captioned and captionless images
- [ ] STOP and wait for human review

### Phase 2: Templates and FuncMap

- [ ] Register `splitImageCaption` and `imageFilename` in FuncMap (`cmd/dashboard/main.go`) calling `httputil.SplitImageCaption` and extracting the filename component respectively
- [ ] Update gallery rendering in `tracker.html`, `goals.html`, `idea.html`, `ideas.html`: call `splitImageCaption`, show caption in `<span class="image-caption">`, use caption as `alt` attribute
- [ ] CSS for `.image-caption`: small font, muted colour, centred, max-width matching thumbnail
- [ ] STOP and wait for human review

### Phase 3: Handler Updates and JavaScript (ES5)

Handler changes -- shared helper and per-handler updates:
- [ ] Extract `httputil.ReconstructImages(r *http.Request) []string` that zips `ParseCSV(r.FormValue("images"))` with `caption-N` form fields via `JoinImageCaption`, applying `SanitiseCaption` to each caption
- [ ] Server-side: validate that the `images` form field contains no pipe characters (captions arrive via `caption-N` fields only). Strip any pipes found in the images field
- [ ] `internal/ideas/handler.go` `Add` (~line 165): call `httputil.ReconstructImages`
- [ ] `internal/ideas/handler.go` `Edit` (~line 231): call `httputil.ReconstructImages`
- [ ] `internal/tracker/handler.go` `QuickAdd` (~line 246): call `httputil.ReconstructImages`
- [ ] `internal/tracker/handler.go` `AddGoal` (~line 283): call `httputil.ReconstructImages`
- [ ] `internal/tracker/handler.go` `UpdateEdit` (~line 433): call `httputil.ReconstructImages`

JavaScript changes (`web/static/upload.js`, ES5 only -- no const/let/arrow functions):
- [ ] `addThumbnail`: add `<input class="image-caption-input" name="caption-N">` below each thumbnail, pre-populate from existing pipe-delimited value via splitting on first `|`
- [ ] `addThumbnail`: set `data-filename` on `.image-thumb-wrap` for reliable reconstruction
- [ ] `appendImage`: update to track caption input index
- [ ] Remove button click handler (inside `addThumbnail`): after removing a thumbnail, walk all remaining `.image-caption-input` elements and reassign `name` attributes sequentially (`caption-0`, `caption-1`, ...). Must handle multiple sequential removals correctly. Off-by-one errors will silently pair captions with wrong images
- [ ] Existing image loading loop (line 54-57): parse `filename|caption` when pre-populating from `hidden.value`
- [ ] Add input event listener on caption inputs to strip commas, pipes, and `]`
- [ ] CSS for `.image-caption-input`
- [ ] STOP and wait for human review

### Phase 4: Self-Review and Final Verification

- [ ] Full test suite, linter, build
- [ ] Verify backwards compatibility: images without captions parse and render identically to current behaviour
- [ ] Verify captions display correctly in all four gallery locations (`tracker.html`, `goals.html`, `idea.html`, `ideas.html`)
- [ ] Verify API response format at `/api/v1/ideas` -- images array contains `filename|caption` entries
- [ ] Verify all success criteria met
- [ ] STOP and wait for human review

## Critical Files

- `internal/httputil/` -- `SplitImageCaption`, `JoinImageCaption`, `SanitiseCaption` shared helpers (new file)
- `internal/ideas/parser.go` -- Image parsing regex, `WriteIdeas` serialisation, `ParseCSV`
- `internal/tracker/tracker.go` -- Image parsing regex, `writeItem` serialisation
- `internal/ideas/handler.go` -- `Add` and `Edit` handlers that parse images from forms
- `internal/tracker/handler.go` -- `AddTask`, `AddGoal`, and `UpdateEdit` handlers that parse images from forms
- `cmd/dashboard/main.go` -- FuncMap registration for `splitImageCaption`
- `web/static/upload.js` -- Caption input in edit forms, hidden value format
- `web/templates/tracker.html` -- Gallery rendering pattern (replicated in goals, idea, ideas)
- `web/static/theme.css` -- Caption display and input styling

## Known Limitations

- The API endpoint `/api/v1/ideas` returns `filename|caption` in the images array. API consumers must handle the pipe-delimited format or split on first `|`.
- API consumers are responsible for output encoding of caption values before rendering in HTML contexts.
- Captions cannot contain commas, pipes, `]`, `<`, `>`, or `"` characters. These are silently stripped. Maximum length is 200 characters.
