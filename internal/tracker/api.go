package tracker

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/httputil"
)

const (
	maxTitleLen   = 500
	maxBodyLen    = 10000
	maxSubStepLen = 500
	maxTagLen     = 50
	maxTagCount   = 10
)

// resolveListService returns the service for the given list name.
func resolveListService(list string, personal, family *Service) *Service {
	switch list {
	case "personal", "todos":
		return personal
	case "family":
		return family
	}
	return nil
}

// itemToAPI converts a tracker Item to a JSON-friendly map.
func itemToAPI(it Item, list string) map[string]any {
	m := map[string]any{
		"slug":            it.Slug,
		"title":           it.Title,
		"type":            string(it.Type),
		"priority":        it.Priority,
		"done":            it.Done,
		"tags":            it.Tags,
		"added":           it.Added,
		"planned":         it.Planned,
		"body":            it.Body,
		"sub_steps_done":  it.SubStepsDone,
		"sub_steps_total": it.SubStepsTotal,
		"list":            list,
	}
	if it.Type == GoalType {
		m["current"] = it.Current
		m["target"] = it.Target
		m["unit"] = it.Unit
		m["deadline"] = it.Deadline
	}
	if it.Completed != "" {
		m["completed"] = it.Completed
	}
	if it.FromIdea != "" {
		m["from_idea"] = it.FromIdea
	}
	return m
}

// itemsToAPI converts a slice of items, filtering out private-tagged items.
func itemsToAPI(items []Item, list string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if it.HasTag("private") {
			continue
		}
		out = append(out, itemToAPI(it, list))
	}
	return out
}

// jsonError writes a JSON error response.
func jsonError(w http.ResponseWriter, msg string, status int) {
	httputil.WriteJSON(w, status, map[string]string{"error": msg})
}

// APIListTodos returns all non-deleted items grouped by list.
func APIListTodos(personalSvc, familySvc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personal, err := personalSvc.List()
		if err != nil {
			jsonError(w, "failed to list personal items", http.StatusInternalServerError)
			return
		}
		family, err := familySvc.List()
		if err != nil {
			jsonError(w, "failed to list family items", http.StatusInternalServerError)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"personal": itemsToAPI(personal, "personal"),
			"family":   itemsToAPI(family, "family"),
		})
	}
}

// APIGetTodo returns a single item by slug.
func APIGetTodo(personalSvc, familySvc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		list := r.URL.Query().Get("list")
		if !httputil.ValidateList(list) {
			jsonError(w, "list parameter required (personal or family)", http.StatusBadRequest)
			return
		}

		svc := resolveListService(list, personalSvc, familySvc)
		item, err := svc.Get(slug)
		if err != nil {
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to get item", http.StatusInternalServerError)
			return
		}

		httputil.WriteJSON(w, http.StatusOK, itemToAPI(*item, httputil.NormaliseList(list)))
	}
}

// APIAddTodo creates a new task.
func APIAddTodo(personalSvc, familySvc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Title    string   `json:"title"`
			Body     string   `json:"body"`
			Tags     []string `json:"tags"`
			Priority string   `json:"priority"`
			List     string   `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Title == "" {
			jsonError(w, "title required", http.StatusBadRequest)
			return
		}
		if !httputil.ValidateList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}
		if err := validateInputLimits(req.Title, req.Body, req.Tags); err != "" {
			jsonError(w, err, http.StatusBadRequest)
			return
		}

		req.Title = httputil.StripInlineMetadata(req.Title)
		req.Body = httputil.StripInlineMetadata(req.Body)
		req.Priority = sanitisePriority(req.Priority)

		item := Item{
			Title:    req.Title,
			Type:     TaskType,
			Body:     req.Body,
			Tags:     req.Tags,
			Priority: req.Priority,
		}

		svc := resolveListService(req.List, personalSvc, familySvc)
		if err := svc.AddItem(item); err != nil {
			jsonError(w, "failed to create item", http.StatusInternalServerError)
			return
		}

		item.Slug = Slugify(req.Title)
		httputil.WriteJSON(w, http.StatusCreated, itemToAPI(item, httputil.NormaliseList(req.List)))
	}
}

// APIUpdateTodo updates a task's title, body, tags, and images.
func APIUpdateTodo(personalSvc, familySvc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		slug := chi.URLParam(r, "slug")
		var req struct {
			Title  string   `json:"title"`
			Body   string   `json:"body"`
			Tags   []string `json:"tags"`
			Images []string `json:"images"`
			List   string   `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !httputil.ValidateList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}
		if err := validateInputLimits(req.Title, req.Body, req.Tags); err != "" {
			jsonError(w, err, http.StatusBadRequest)
			return
		}

		req.Title = httputil.StripInlineMetadata(req.Title)
		req.Body = httputil.StripInlineMetadata(req.Body)

		svc := resolveListService(req.List, personalSvc, familySvc)
		if e := svc.UpdateEdit(slug, req.Title, req.Body, req.Tags, req.Images); e != nil {
			if httputil.IsNotFound(e) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to update item", http.StatusInternalServerError)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// APICompleteTodo marks a task as done.
func APICompleteTodo(personalSvc, familySvc *Service) http.HandlerFunc {
	return statusMutation(personalSvc, familySvc, func(svc *Service, slug string) error {
		return svc.Complete(slug)
	})
}

// APIUncompleteTodo marks a task as not done.
func APIUncompleteTodo(personalSvc, familySvc *Service) http.HandlerFunc {
	return statusMutation(personalSvc, familySvc, func(svc *Service, slug string) error {
		return svc.Uncomplete(slug)
	})
}

// APIDeleteTodo soft-deletes a task.
func APIDeleteTodo(personalSvc, familySvc *Service) http.HandlerFunc {
	return statusMutation(personalSvc, familySvc, func(svc *Service, slug string) error {
		return svc.Delete(slug)
	})
}

// APIUpdatePriority sets the priority on a task.
func APIUpdatePriority(personalSvc, familySvc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		slug := chi.URLParam(r, "slug")
		var req struct {
			Priority string `json:"priority"`
			List     string `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !httputil.ValidateList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}
		req.Priority = sanitisePriority(req.Priority)

		svc := resolveListService(req.List, personalSvc, familySvc)
		if err := svc.UpdatePriority(slug, req.Priority); err != nil {
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to update priority", http.StatusInternalServerError)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// APIUpdateTags sets the tags on a task.
func APIUpdateTags(personalSvc, familySvc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		slug := chi.URLParam(r, "slug")
		var req struct {
			Tags []string `json:"tags"`
			List string   `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !httputil.ValidateList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}
		if len(req.Tags) > maxTagCount {
			jsonError(w, "too many tags (max 10)", http.StatusBadRequest)
			return
		}

		svc := resolveListService(req.List, personalSvc, familySvc)
		if err := svc.UpdateTags(slug, req.Tags); err != nil {
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to update tags", http.StatusInternalServerError)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// APIAddSubStep adds a sub-step to a task.
func APIAddSubStep(personalSvc, familySvc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		slug := chi.URLParam(r, "slug")
		var req struct {
			Text string `json:"text"`
			List string `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !httputil.ValidateList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}
		req.Text = strings.TrimSpace(req.Text)
		if req.Text == "" {
			jsonError(w, "text required", http.StatusBadRequest)
			return
		}
		if utf8.RuneCountInString(req.Text) > maxSubStepLen {
			jsonError(w, "sub-step text too long (max 500 chars)", http.StatusBadRequest)
			return
		}
		req.Text = httputil.StripInlineMetadata(req.Text)

		svc := resolveListService(req.List, personalSvc, familySvc)
		if err := svc.AddSubStep(slug, req.Text); err != nil {
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to add sub-step", http.StatusInternalServerError)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// APIToggleSubStep toggles a sub-step's done state.
func APIToggleSubStep(personalSvc, familySvc *Service) http.HandlerFunc {
	return subStepIndexMutation(personalSvc, familySvc, func(svc *Service, slug string, index int) error {
		return svc.ToggleSubStep(slug, index)
	})
}

// APIRemoveSubStep removes a sub-step by index.
func APIRemoveSubStep(personalSvc, familySvc *Service) http.HandlerFunc {
	return subStepIndexMutation(personalSvc, familySvc, func(svc *Service, slug string, index int) error {
		return svc.RemoveSubStep(slug, index)
	})
}

// statusMutation is a helper for simple slug+list mutation endpoints.
func statusMutation(personalSvc, familySvc *Service, fn func(svc *Service, slug string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		slug := chi.URLParam(r, "slug")
		var req struct {
			List string `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !httputil.ValidateList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}

		svc := resolveListService(req.List, personalSvc, familySvc)
		if err := fn(svc, slug); err != nil {
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to update item", http.StatusInternalServerError)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// subStepIndexMutation is a helper for sub-step operations that take an index from the URL.
func subStepIndexMutation(personalSvc, familySvc *Service, fn func(svc *Service, slug string, index int) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		slug := chi.URLParam(r, "slug")
		indexStr := chi.URLParam(r, "index")
		index, err := strconv.Atoi(indexStr)
		if err != nil || index < 0 {
			jsonError(w, "invalid sub-step index", http.StatusBadRequest)
			return
		}

		var req struct {
			List string `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !httputil.ValidateList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}

		svc := resolveListService(req.List, personalSvc, familySvc)
		if err := fn(svc, slug, index); err != nil {
			if strings.Contains(err.Error(), "out of range") {
				jsonError(w, err.Error(), http.StatusBadRequest)
				return
			}
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to update sub-step", http.StatusInternalServerError)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// validateInputLimits checks title, body, and tags against size limits.
// Returns an error message or empty string if valid.
func validateInputLimits(title, body string, tags []string) string {
	if utf8.RuneCountInString(title) > maxTitleLen {
		return "title too long (max 500 chars)"
	}
	if utf8.RuneCountInString(body) > maxBodyLen {
		return "body too long (max 10000 chars)"
	}
	if len(tags) > maxTagCount {
		return "too many tags (max 10)"
	}
	for _, tag := range tags {
		if utf8.RuneCountInString(tag) > maxTagLen {
			return "tag too long (max 50 chars)"
		}
	}
	return ""
}
