package app

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/httputil"
)

// revisionsHeader reports each module's change revision after a mutation,
// so the page can ignore the SSE echo of its own write.
const revisionsHeader = "Dash-Revisions"

// flashPattern finds the flash message the layout renders for ?msg=.
var flashPattern = regexp.MustCompile(`<div id="flash"[^>]*data-error="(true|false)"[^>]*>([^<]*)</div>`)

// fragmentResponses turns an htmx form POST into a single request. Handlers
// keep answering with a 303 to the page the change belongs on; for htmx
// requests that redirect is replayed here as an internal GET, and the page is
// returned with the flash as an HX-Trigger event. htmx then morphs the page's
// live container. Plain posts, errors and non-local redirects pass through.
//
// HX-Request only chooses the response format. It proves nothing about where
// the request came from; cross-origin protection runs before this.
func fragmentResponses(replay func() http.Handler, revisions func() map[string]uint64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.Header.Get("HX-Request") != "true" {
				next.ServeHTTP(w, r)
				return
			}
			rec := newBufferedResponse()
			next.ServeHTTP(rec, r)

			undo := rec.header.Get(httputil.UndoHeader)
			rec.header.Del(httputil.UndoHeader)
			location := rec.header.Get("Location")
			out := w.Header()
			for k, v := range rec.header {
				if k != "Content-Length" {
					out[k] = v
				}
			}
			out.Add("Vary", "HX-Request")
			out.Set("Cache-Control", "no-store")

			dest, err := url.Parse(location)
			if rec.status != http.StatusSeeOther || err != nil || !httputil.IsLocalPath(dest.Path) || dest.Host != "" || dest.Scheme != "" {
				w.WriteHeader(rec.status)
				writeBody(w, rec.body.Bytes())
				return
			}

			// A lapsed session redirects to the login page, which has no
			// live container; the whole page goes there instead, and comes
			// back to the page the user was on rather than the form's URL.
			if dest.Path == "/login" {
				out.Del("Location")
				out.Set("HX-Redirect", loginReturningTo(r))
				w.WriteHeader(http.StatusOK)
				return
			}

			out.Del("Location")
			// Read before the replay renders, so the page holds at least the
			// changes counted. Only a page that renders reports them: a tab
			// that got nothing back must still refresh for them.
			revs := formatRevisions(revisions())
			page := replayGet(replay(), r, dest)
			if page.status >= 200 && page.status < 300 {
				out.Set(revisionsHeader, revs)
			}
			if trigger := flashTrigger(dest.Query().Get("msg"), page.body.Bytes(), undo); trigger != "" {
				out.Set("HX-Trigger", trigger)
			}
			out.Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(page.status)
			writeBody(w, page.body.Bytes())
		})
	}
}

// replayGet renders dest for the same user as r. The route context is
// cleared so the router matches dest afresh; the encoding header is dropped
// so the body comes back uncompressed (the outer response compresses it).
func replayGet(h http.Handler, r *http.Request, dest *url.URL) *bufferedResponse {
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, nil)
	req := r.Clone(ctx)
	req.Method = http.MethodGet
	req.URL = &url.URL{Path: dest.Path, RawQuery: dest.RawQuery}
	req.RequestURI = req.URL.RequestURI()
	req.Body = http.NoBody
	req.ContentLength = 0
	req.Form, req.PostForm, req.MultipartForm = nil, nil, nil
	for _, k := range []string{"Content-Type", "Content-Length", "Accept-Encoding", "HX-Request"} {
		req.Header.Del(k)
	}
	rec := newBufferedResponse()
	h.ServeHTTP(rec, req)
	return rec
}

// loginReturningTo is the login URL that returns to the page an htmx request
// came from, as the layout's expired-session redirect does.
func loginReturningTo(r *http.Request) string {
	q := url.Values{"expired": {"1"}}
	if cur, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil && httputil.IsLocalPath(cur.Path) {
		q.Set("next", cur.Path)
	}
	return "/login?" + q.Encode()
}

// flashTrigger builds the HX-Trigger value carrying the flash key, its
// message as the page rendered it, and an undo path if the handler offered
// one. Item titles never go into it.
func flashTrigger(key string, page []byte, undo string) string {
	if undo != "" && !httputil.IsLocalPath(undo) {
		undo = ""
	}
	if key == "" && undo == "" {
		return ""
	}
	flash := map[string]any{"key": key}
	if m := flashPattern.FindSubmatch(page); m != nil {
		flash["message"] = html.UnescapeString(string(m[2]))
		flash["error"] = string(m[1]) == "true"
	}
	if undo != "" {
		flash["undo"] = undo
	}
	b, err := json.Marshal(map[string]any{"dash:flash": flash})
	if err != nil {
		slog.Error("encoding flash trigger", "error", err)
		return ""
	}
	return string(b)
}

func formatRevisions(revs map[string]uint64) string {
	parts := make([]string, 0, len(revs))
	for _, id := range slices.Sorted(maps.Keys(revs)) {
		parts = append(parts, id+"="+strconv.FormatUint(revs[id], 10))
	}
	return strings.Join(parts, " ")
}

func writeBody(w http.ResponseWriter, b []byte) {
	if _, err := w.Write(b); err != nil {
		slog.Debug("writing fragment response", "error", err)
	}
}

// bufferedResponse holds a handler's response so it can be inspected
// before anything reaches the client.
type bufferedResponse struct {
	header      http.Header
	status      int
	wroteHeader bool
	body        bytes.Buffer
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: http.Header{}, status: http.StatusOK}
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }

func (b *bufferedResponse) WriteHeader(status int) {
	if !b.wroteHeader {
		b.status, b.wroteHeader = status, true
	}
}
