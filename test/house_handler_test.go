package test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/tracker"
)

// The house page has no image controls, so saving a project's edit form must
// keep the images it already has (e.g. from a converted idea).
func TestHouseProjectEditKeepsImages(t *testing.T) {
	dir := t.TempDir()
	projectsPath := filepath.Join(dir, "house-projects.md")
	writeFile(t, projectsPath, "# House Projects\n\n- [ ] Kitchen splashback [images: tiles.jpg|Chosen tile] [status: active]\n")
	maintPath := filepath.Join(dir, "maintenance.md")
	writeFile(t, maintPath, "# Maintenance\n")
	projects := tracker.NewService(projectsPath, "House Projects", time.UTC)
	h := house.NewHandler(house.NewService(maintPath, time.UTC), projects, nil, time.UTC)

	form := url.Values{"title": {"Kitchen splashback"}, "body": {"Grout is grey."}, "tags": {"kitchen"}}
	req := httptest.NewRequest(http.MethodPost, "/house/projects/kitchen-splashback/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withChiURLParam(req, "slug", "kitchen-splashback")
	w := httptest.NewRecorder()
	h.EditProject(w, req)
	if w.Code >= 400 {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	item, err := projects.Get("kitchen-splashback")
	if err != nil {
		t.Fatal(err)
	}
	if item.Body != "Grout is grey." {
		t.Errorf("body = %q, want the edited body", item.Body)
	}
	if want := []string{"tiles.jpg|Chosen tile"}; !slices.Equal(item.Images, want) {
		t.Errorf("images = %v, want %v", item.Images, want)
	}
}
