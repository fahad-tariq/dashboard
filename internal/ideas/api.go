package ideas

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/itemid"
)

// APIIdea is the JSON shape of an idea in every API response.
type APIIdea struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
	Images      []string `json:"images"`
	Project     string   `json:"project"`
	Added       string   `json:"added"`
	Body        string   `json:"body"`
	ConvertedTo string   `json:"converted_to,omitempty"`
	Deleted     string   `json:"deleted,omitempty"`
}

// ToAPI converts an idea to its API shape.
func ToAPI(idea Idea) APIIdea {
	return APIIdea{
		ID:          idea.ID,
		Title:       idea.Title,
		Status:      idea.Status,
		Tags:        nonNil(idea.Tags),
		Images:      nonNil(idea.Images),
		Project:     idea.Project,
		Added:       idea.Added,
		Body:        idea.Body,
		ConvertedTo: idea.ConvertedTo,
		Deleted:     idea.DeletedAt,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// writeStored answers with the idea as stored after a change.
func writeStored(w http.ResponseWriter, svc *Service, id string, status int) {
	idea, err := svc.Get(id)
	if err != nil {
		httputil.JSONError(w, "failed to read idea", http.StatusInternalServerError)
		return
	}
	httputil.WriteJSON(w, status, ToAPI(*idea))
}

// APIListIdeas returns every idea not in the trash.
func (h *Handler) APIListIdeas(w http.ResponseWriter, r *http.Request) {
	ideas, err := h.resolve(r).List()
	if err != nil {
		httputil.JSONError(w, "failed to list ideas", http.StatusInternalServerError)
		return
	}
	out := make([]APIIdea, 0, len(ideas))
	for _, idea := range ideas {
		out = append(out, ToAPI(idea))
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// APIAddIdea creates an idea. A request may bring its own ID, so a client
// that retries after a lost response creates one idea, not two.
func (h *Handler) APIAddIdea(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		ID    string   `json:"id"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
		Type  string   `json:"type"` // Legacy: converted to single tag.
		Body  string   `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if httputil.CleanTitle(req.Title) == "" {
		httputil.JSONError(w, "title required", http.StatusBadRequest)
		return
	}
	if req.ID != "" && !itemid.Valid(req.ID) {
		httputil.JSONError(w, "invalid id: 8 characters from "+itemid.Alphabet, http.StatusBadRequest)
		return
	}

	tags := req.Tags
	if len(tags) == 0 && req.Type != "" {
		tags = []string{req.Type}
	}
	idea := &Idea{
		ID:    req.ID,
		Title: req.Title,
		Tags:  tags,
		Added: time.Now().In(h.loc).Format("2006-01-02"),
		Body:  req.Body,
	}

	svc := h.resolve(r)
	created, err := svc.AddOnce(idea)
	if err != nil {
		httputil.JSONError(w, "failed to create idea", http.StatusInternalServerError)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	writeStored(w, svc, idea.ID, status)
}

// APITriageIdea changes an idea's status.
func (h *Handler) APITriageIdea(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	id := chi.URLParam(r, "id")
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	svc := h.resolve(r)
	if err := svc.Triage(id, req.Action); err != nil {
		switch {
		case httputil.IsNotFound(err):
			httputil.JSONError(w, "idea not found", http.StatusNotFound)
		case strings.Contains(err.Error(), "unknown triage action"):
			httputil.JSONError(w, "action must be park, drop or untriage", http.StatusBadRequest)
		default:
			httputil.JSONError(w, "failed to triage idea", http.StatusInternalServerError)
		}
		return
	}
	writeStored(w, svc, id, http.StatusOK)
}

// APIAddResearch appends research content to an idea's body.
func (h *Handler) APIAddResearch(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	id := chi.URLParam(r, "id")
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	svc := h.resolve(r)
	if err := svc.AddResearch(id, req.Content); err != nil {
		if httputil.IsNotFound(err) {
			httputil.JSONError(w, "idea not found", http.StatusNotFound)
			return
		}
		httputil.JSONError(w, "failed to add research", http.StatusInternalServerError)
		return
	}
	writeStored(w, svc, id, http.StatusCreated)
}
