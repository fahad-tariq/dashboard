package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/ideas"
)

// Hand-edited files may indent body lines with a tab; those lines must not
// vanish on parse (and so on the next write).
func TestTabIndentedBodyLinesSurvive(t *testing.T) {
	tests := map[string]struct {
		content string
		body    func(path string) (string, error)
		want    []string
	}{
		"idea": {
			content: "# Ideas\n\n- [ ] Camping kit [status: untriaged]\n\tPack light.\n\t- [x] Head torch\n",
			body: func(p string) (string, error) {
				got, err := ideas.ParseIdeas(p)
				if err != nil || len(got) != 1 {
					return "", err
				}
				return got[0].Body, nil
			},
			want: []string{"Pack light.", "- [x] Head torch"},
		},
		"maintenance": {
			content: "# Maintenance\n\n- [ ] Clean gutters [cadence: 6m]\n\tUse the tall ladder.\n",
			body: func(p string) (string, error) {
				got, err := house.ParseMaintenance(p)
				if err != nil || len(got) != 1 {
					return "", err
				}
				return got[0].Notes, nil
			},
			want: []string{"Use the tall ladder."},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name+".md")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			body, err := tc.body(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				if !strings.Contains(body, w) {
					t.Errorf("body %q lost tab-indented line %q", body, w)
				}
			}
		})
	}
}
