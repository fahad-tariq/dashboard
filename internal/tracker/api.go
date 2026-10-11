package tracker

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/itemid"
)

const (
	maxTitleLen   = 500
	maxBodyLen    = 10000
	maxSubStepLen = 500
	maxTagLen     = 50
	maxTagCount   = 10
)

// isAPIList reports whether list names a list this API serves. It is
// narrower than httputil.ValidateList, which also accepts "house".
func isAPIList(list string) bool {
	switch list {
	case "personal", "todos", "family":
		return true
	}
	return false
}

// resolveListService returns the service for the given list name, or nil for
// a list isAPIList rejects.
func resolveListService(list string, personal, family *Service) *Service {
	switch list {
	case "personal", "todos":
		return personal
	case "family":
		return family
	}
	return nil
}

// APIItem is the JSON shape of a tracker item in every API response.
type APIItem struct {
	ID            string       `json:"id"`
	List          string       `json:"list"`
	Title         string       `json:"title"`
	Type          string       `json:"type"`
	Priority      string       `json:"priority"`
	Done          bool         `json:"done"`
	Tags          []string     `json:"tags"`
	Images        []string     `json:"images"`
	Body          string       `json:"body"`
	SubSteps      []APISubStep `json:"sub_steps"`
	SubStepsDone  int          `json:"sub_steps_done"`
	SubStepsTotal int          `json:"sub_steps_total"`
	Added         string       `json:"added"`
	Completed     string       `json:"completed,omitempty"`
	Deadline      string       `json:"deadline"`
	Planned       string       `json:"planned"`
	PlanOrder     int          `json:"plan_order"`
	Deleted       string       `json:"deleted,omitempty"`
	FromIdea      string       `json:"from_idea,omitempty"`

	// Goals only.
	Current *float64 `json:"current,omitempty"`
	Target  *float64 `json:"target,omitempty"`
	Unit    *string  `json:"unit,omitempty"`

	// House projects only.
	Status string   `json:"status,omitempty"`
	Budget *float64 `json:"budget,omitempty"`
	Actual *float64 `json:"actual,omitempty"`
}

// APISubStep is one checkbox line of an item's body.
type APISubStep struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// APITodos is the response of GET /api/v1/todos.
type APITodos struct {
	Personal []APIItem `json:"personal"`
	Family   []APIItem `json:"family"`
}

// ToAPI converts an item from list ("personal", "family" or "house") to its
// API shape. Slices are never nil, so they encode as [] rather than null.
func ToAPI(it Item, list string) APIItem {
	out := APIItem{
		ID:            it.ID,
		List:          list,
		Title:         it.Title,
		Type:          string(it.Type),
		Priority:      it.Priority,
		Done:          it.Done,
		Tags:          nonNil(it.Tags),
		Images:        nonNil(it.Images),
		Body:          it.Body,
		SubSteps:      []APISubStep{},
		SubStepsDone:  it.SubStepsDone,
		SubStepsTotal: it.SubStepsTotal,
		Added:         it.Added,
		Completed:     it.Completed,
		Deadline:      it.Deadline,
		Planned:       it.Planned,
		PlanOrder:     it.PlanOrder,
		Deleted:       it.DeletedAt,
		FromIdea:      it.FromIdea,
	}
	for _, step := range ParseSubSteps(it.Body) {
		out.SubSteps = append(out.SubSteps, APISubStep(step))
	}
	if it.Type == GoalType {
		out.Current, out.Target, out.Unit = &it.Current, &it.Target, &it.Unit
	}
	if list == "house" {
		out.Status, out.Budget, out.Actual = it.Status, &it.Budget, &it.Actual
	}
	return out
}

// ToAPIList converts every item in items.
func ToAPIList(items []Item, list string) []APIItem {
	out := make([]APIItem, 0, len(items))
	for _, it := range items {
		out = append(out, ToAPI(it, list))
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// WriteStored answers with the item as stored after a change.
func WriteStored(w http.ResponseWriter, svc *Service, id, list string, status int) {
	it, err := svc.Get(id)
	if err != nil {
		jsonError(w, "failed to read item", http.StatusInternalServerError)
		return
	}
	httputil.WriteJSON(w, status, ToAPI(*it, httputil.NormaliseList(list)))
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	httputil.JSONError(w, msg, status)
}

// APIListTodos returns all non-deleted items grouped by list.
func APIListTodos(resolve ServiceResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personalSvc, familySvc := resolve(r)
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
		httputil.WriteJSON(w, http.StatusOK, APITodos{
			Personal: ToAPIList(personal, "personal"),
			Family:   ToAPIList(family, "family"),
		})
	}
}

// APIGetTodo returns a single item by id.
func APIGetTodo(resolve ServiceResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personalSvc, familySvc := resolve(r)
		id := chi.URLParam(r, "id")
		list := r.URL.Query().Get("list")
		if !isAPIList(list) {
			jsonError(w, "list parameter required (personal or family)", http.StatusBadRequest)
			return
		}

		svc := resolveListService(list, personalSvc, familySvc)
		item, err := svc.Get(id)
		if err != nil {
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to get item", http.StatusInternalServerError)
			return
		}

		httputil.WriteJSON(w, http.StatusOK, ToAPI(*item, httputil.NormaliseList(list)))
	}
}

// APIAddTodo creates a new task. A request may bring its own ID, so a
// client that retries after a lost response creates one task, not two.
func APIAddTodo(resolve ServiceResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personalSvc, familySvc := resolve(r)
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			ID       string   `json:"id"`
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

		req.Title, req.Tags, req.Priority = QuickAdd(req.Title, req.Tags, sanitisePriority(req.Priority))
		if httputil.CleanTitle(req.Title) == "" {
			jsonError(w, "title required", http.StatusBadRequest)
			return
		}
		if !isAPIList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}
		if err := validateInputLimits(req.Title, req.Body, req.Tags); err != "" {
			jsonError(w, err, http.StatusBadRequest)
			return
		}
		if req.ID != "" && !itemid.Valid(req.ID) {
			jsonError(w, "invalid id: 8 characters from "+itemid.Alphabet, http.StatusBadRequest)
			return
		}

		req.Title = httputil.CleanTitle(req.Title)
		req.Body = httputil.StripInlineMetadata(req.Body)

		item := Item{
			ID:       req.ID,
			Title:    req.Title,
			Type:     TaskType,
			Body:     req.Body,
			Tags:     req.Tags,
			Priority: req.Priority,
		}

		svc := resolveListService(req.List, personalSvc, familySvc)
		// A retry that brings an ID the list holds gets the stored item.
		id, created, err := svc.AddItemOnce(item)
		if err != nil {
			jsonError(w, "failed to create item", http.StatusInternalServerError)
			return
		}
		status := http.StatusCreated
		if !created {
			status = http.StatusOK
		}
		WriteStored(w, svc, id, req.List, status)
	}
}

// APIUpdateTodo updates a task's title, body, tags, images and deadline.
func APIUpdateTodo(resolve ServiceResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personalSvc, familySvc := resolve(r)
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		id := chi.URLParam(r, "id")
		// Pointers tell an omitted field (kept) from an empty one (cleared).
		var req struct {
			Title    string    `json:"title"`
			Body     *string   `json:"body"`
			Tags     *[]string `json:"tags"`
			Images   *[]string `json:"images"`
			Deadline *string   `json:"deadline"`
			List     string    `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !isAPIList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}
		edit := Edit{Title: httputil.StripInlineMetadata(req.Title), Tags: req.Tags, Images: req.Images, Deadline: req.Deadline}
		var body string
		var tags []string
		if req.Body != nil {
			body = *req.Body
			stripped := httputil.StripInlineMetadata(body)
			edit.Body = &stripped
		}
		if req.Tags != nil {
			tags = *req.Tags
		}
		if err := validateInputLimits(req.Title, body, tags); err != "" {
			jsonError(w, err, http.StatusBadRequest)
			return
		}

		svc := resolveListService(req.List, personalSvc, familySvc)
		if e := svc.ApplyEdit(id, edit); e != nil {
			if errors.Is(e, ErrInvalidDate) {
				jsonError(w, "invalid deadline: use YYYY-MM-DD", http.StatusBadRequest)
				return
			}
			if httputil.IsNotFound(e) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to update item", http.StatusInternalServerError)
			return
		}
		WriteStored(w, svc, id, req.List, http.StatusOK)
	}
}

// APICompleteTodo marks a task as done.
func APICompleteTodo(resolve ServiceResolver) http.HandlerFunc {
	return statusMutation(resolve, func(svc *Service, id string) error {
		return svc.Complete(id)
	})
}

// APIUncompleteTodo marks a task as not done.
func APIUncompleteTodo(resolve ServiceResolver) http.HandlerFunc {
	return statusMutation(resolve, func(svc *Service, id string) error {
		return svc.Uncomplete(id)
	})
}

// APIDeleteTodo soft-deletes a task.
func APIDeleteTodo(resolve ServiceResolver) http.HandlerFunc {
	return statusMutation(resolve, func(svc *Service, id string) error {
		return svc.Delete(id)
	})
}

// APIUpdatePriority sets the priority on a task.
func APIUpdatePriority(resolve ServiceResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personalSvc, familySvc := resolve(r)
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		id := chi.URLParam(r, "id")
		var req struct {
			Priority string `json:"priority"`
			List     string `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !isAPIList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}
		req.Priority = sanitisePriority(req.Priority)

		svc := resolveListService(req.List, personalSvc, familySvc)
		if err := svc.UpdatePriority(id, req.Priority); err != nil {
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to update priority", http.StatusInternalServerError)
			return
		}
		WriteStored(w, svc, id, req.List, http.StatusOK)
	}
}

// APIUpdateTags sets the tags on a task.
func APIUpdateTags(resolve ServiceResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personalSvc, familySvc := resolve(r)
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		id := chi.URLParam(r, "id")
		var req struct {
			Tags []string `json:"tags"`
			List string   `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !isAPIList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}
		if len(req.Tags) > maxTagCount {
			jsonError(w, "too many tags (max 10)", http.StatusBadRequest)
			return
		}

		svc := resolveListService(req.List, personalSvc, familySvc)
		if err := svc.UpdateTags(id, req.Tags); err != nil {
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to update tags", http.StatusInternalServerError)
			return
		}
		WriteStored(w, svc, id, req.List, http.StatusOK)
	}
}

// APIAddSubStep adds a sub-step to a task.
func APIAddSubStep(resolve ServiceResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personalSvc, familySvc := resolve(r)
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		id := chi.URLParam(r, "id")
		var req struct {
			Text string `json:"text"`
			List string `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !isAPIList(req.List) {
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
		if err := svc.AddSubStep(id, req.Text); err != nil {
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to add sub-step", http.StatusInternalServerError)
			return
		}
		WriteStored(w, svc, id, req.List, http.StatusOK)
	}
}

// APIToggleSubStep toggles a sub-step's done state.
func APIToggleSubStep(resolve ServiceResolver) http.HandlerFunc {
	return subStepIndexMutation(resolve, func(svc *Service, id string, index int) error {
		return svc.ToggleSubStep(id, index)
	})
}

// APIRemoveSubStep removes a sub-step by index.
func APIRemoveSubStep(resolve ServiceResolver) http.HandlerFunc {
	return subStepIndexMutation(resolve, func(svc *Service, id string, index int) error {
		return svc.RemoveSubStep(id, index)
	})
}

// statusMutation is a helper for simple id+list mutation endpoints.
func statusMutation(resolve ServiceResolver, fn func(svc *Service, id string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personalSvc, familySvc := resolve(r)
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		id := chi.URLParam(r, "id")
		var req struct {
			List string `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !isAPIList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}

		svc := resolveListService(req.List, personalSvc, familySvc)
		if err := fn(svc, id); err != nil {
			if httputil.IsNotFound(err) {
				jsonError(w, "item not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to update item", http.StatusInternalServerError)
			return
		}
		WriteStored(w, svc, id, req.List, http.StatusOK)
	}
}

// subStepIndexMutation is a helper for sub-step operations that take an index from the URL.
func subStepIndexMutation(resolve ServiceResolver, fn func(svc *Service, id string, index int) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		personalSvc, familySvc := resolve(r)
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		id := chi.URLParam(r, "id")
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
		if !isAPIList(req.List) {
			jsonError(w, "list required (personal or family)", http.StatusBadRequest)
			return
		}

		svc := resolveListService(req.List, personalSvc, familySvc)
		if err := fn(svc, id, index); err != nil {
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
		WriteStored(w, svc, id, req.List, http.StatusOK)
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
