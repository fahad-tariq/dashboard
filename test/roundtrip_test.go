package test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/tracker"
)

// rewriteFunc parses src with one parser, writes the result to dst with the
// matching writer and returns the number of items parsed.
type rewriteFunc func(t *testing.T, src, dst string) int

func rewriteTracker(heading string) rewriteFunc {
	return func(t *testing.T, src, dst string) int {
		t.Helper()
		items, err := tracker.ParseTracker(src)
		if err != nil {
			t.Fatalf("ParseTracker: %v", err)
		}
		if err := tracker.WriteTracker(dst, heading, items); err != nil {
			t.Fatalf("WriteTracker: %v", err)
		}
		return len(items)
	}
}

func rewriteIdeas(t *testing.T, src, dst string) int {
	t.Helper()
	parsed, err := ideas.ParseIdeas(src)
	if err != nil {
		t.Fatalf("ParseIdeas: %v", err)
	}
	if err := ideas.WriteIdeas(dst, "Ideas", parsed); err != nil {
		t.Fatalf("WriteIdeas: %v", err)
	}
	return len(parsed)
}

func rewriteMaintenance(t *testing.T, src, dst string) int {
	t.Helper()
	items, err := house.ParseMaintenance(src)
	if err != nil {
		t.Fatalf("ParseMaintenance: %v", err)
	}
	if err := house.WriteMaintenance(dst, "Maintenance", items); err != nil {
		t.Fatalf("WriteMaintenance: %v", err)
	}
	return len(items)
}

// roundtrip runs rewrite over the file at src and returns the written bytes.
func roundtrip(t *testing.T, rewrite rewriteFunc, src string) ([]byte, int) {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "out.md")
	n := rewrite(t, src, dst)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	return got, n
}

// TestRoundtripByteIdentical parses each fixture and writes it back. Fixtures
// are in the writers' canonical form, so the output must match byte for byte.
func TestRoundtripByteIdentical(t *testing.T) {
	cases := map[string]struct {
		fixture   string
		rewrite   rewriteFunc
		wantItems int
	}{
		"tracker personal: every task and goal tag, sub-steps, captions, done, soft-deleted": {
			fixture: "tracker_personal.md", rewrite: rewriteTracker("Personal"), wantItems: 9,
		},
		"tracker family: planned with plan-order, goal with unit, caption, soft-deleted": {
			fixture: "tracker_family.md", rewrite: rewriteTracker("Family"), wantItems: 5,
		},
		"tracker house projects: budget, actual, status, decimal budget, soft-deleted": {
			fixture: "tracker_house_projects.md", rewrite: rewriteTracker("House"), wantItems: 4,
		},
		"ideas: blank lines and indented headings in bodies, every status, converted-to, project, captions": {
			fixture: "ideas.md", rewrite: rewriteIdeas, wantItems: 5,
		},
		"maintenance: every cadence unit, newest-first logs with notes, indented notes, captions, soft-deleted": {
			fixture: "maintenance.md", rewrite: rewriteMaintenance, wantItems: 5,
		},

		"tracker with IDs: on items only, not sub-steps; from-idea holds an ID": {
			fixture: "tracker_ids.md", rewrite: rewriteTracker("Personal"), wantItems: 4,
		},
		"ideas with IDs: blank lines and a checklist in the body; converted-to holds an ID": {
			fixture: "ideas_ids.md", rewrite: rewriteIdeas, wantItems: 2,
		},
		"maintenance with IDs: not on log entries": {
			fixture: "maintenance_ids.md", rewrite: rewriteMaintenance, wantItems: 2,
		},

		// Known bug: the tracker parser treats any line whose trimmed form starts
		// with "#" as a section heading, indented or not. An indented markdown
		// heading in a task body ends the item, the heading is dropped, and every
		// following body line is discarded because no item is open.
		"tracker bug: indented heading in task body": {
			fixture: "tracker_body_heading.md", rewrite: rewriteTracker("Personal"), wantItems: 2,
		},
		// Known bug: the ideas parser checks for a checkbox on the trimmed line
		// before checking for indentation, so a checklist inside an idea body
		// splits off into new top-level ideas with [status: untriaged].
		"ideas bug: checklist in idea body": {
			fixture: "ideas_body_checklist.md", rewrite: rewriteIdeas, wantItems: 1,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			src := filepath.Join("testdata", "roundtrip", tc.fixture)
			want, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			got, n := roundtrip(t, tc.rewrite, src)
			if n != tc.wantItems {
				t.Errorf("parsed %d items, want %d", n, tc.wantItems)
			}
			if string(got) != string(want) {
				t.Errorf("round-trip of %s is not byte-identical\n--- got ---\n%s\n--- want ---\n%s", tc.fixture, got, want)
			}
		})
	}
}

// TestRoundtripNormalised covers hand-edited or legacy input the writers
// deliberately rewrite into canonical form. Each case compares against a
// golden file and then checks the golden is a fixed point (parse+write of the
// golden returns it unchanged), so normalisation happens at most once.
//
// Documented normalisations, all in the current production code:
//
// Tracker (tracker_normalise.md):
//   - Section headings ("## Tasks", "## Goals") are dropped; the file is written
//     as one flat list under the single "# <heading>" line.
//   - Blank lines in bodies are dropped (CLAUDE.md: the tracker parser drops them).
//   - Body lines are trimmed and re-indented with exactly two spaces, so deeper
//     indentation and tab indentation are flattened and trailing spaces removed.
//   - Inline metadata is rewritten in the writer's fixed order, and "- [X]"
//     becomes "- [x]".
//   - Numbers lose trailing zeros ("120.50" -> "120.5", "12.0" -> "12").
//   - "[plan-order: 0]" is dropped because 0 means unset.
//
// Ideas (ideas_normalise.md):
//   - Legacy section headings are dropped; status lives in [status:] only.
//   - Ideas have no done state, so "- [x]" becomes "- [ ]".
//   - A missing [status:] is written as [status: untriaged].
//   - Whitespace-only body lines become empty lines; runs of blank lines
//     between ideas collapse to one; trailing blank body lines are trimmed.
//   - Inline metadata is rewritten in the writer's fixed order.
//
// Maintenance (maintenance_normalise.md):
//   - Notes and log entries are stored separately, so notes are written first
//     and log entries after them in their original relative order.
//   - Blank lines in notes are dropped.
//   - Maintenance items have no done state, so "- [x]" becomes "- [ ]", and
//     "- [X]" log entries become "- [x]".
//   - Inline metadata is rewritten in the writer's fixed order.
func TestRoundtripNormalised(t *testing.T) {
	cases := map[string]struct {
		fixture string
		rewrite rewriteFunc
	}{
		"tracker":     {fixture: "tracker_normalise.md", rewrite: rewriteTracker("Personal")},
		"ideas":       {fixture: "ideas_normalise.md", rewrite: rewriteIdeas},
		"maintenance": {fixture: "maintenance_normalise.md", rewrite: rewriteMaintenance},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			src := filepath.Join("testdata", "roundtrip", tc.fixture)
			golden := filepath.Join("roundtrip", tc.fixture[:len(tc.fixture)-len(".md")]+".golden")

			got, _ := roundtrip(t, tc.rewrite, src)
			assertGolden(t, golden, got)

			again, _ := roundtrip(t, tc.rewrite, filepath.Join("testdata", golden))
			if string(again) != string(got) {
				t.Errorf("golden %s is not a fixed point\n--- second pass ---\n%s\n--- first pass ---\n%s", golden, again, got)
			}
		})
	}
}

// Each parser reads a final [id:] into ID and out of the title, leaves a
// malformed one in the title, and gives indented lines no ID.
func TestParseItemIDs(t *testing.T) {
	write := func(t *testing.T, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "list.md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	type got struct{ id, title string }
	tracker1 := func(t *testing.T, content string) []got {
		items, err := tracker.ParseTracker(write(t, content))
		if err != nil {
			t.Fatal(err)
		}
		var out []got
		for _, it := range items {
			out = append(out, got{it.ID, it.Title})
		}
		return out
	}
	ideas1 := func(t *testing.T, content string) []got {
		items, err := ideas.ParseIdeas(write(t, content))
		if err != nil {
			t.Fatal(err)
		}
		var out []got
		for _, it := range items {
			out = append(out, got{it.ID, it.Title})
		}
		return out
	}
	maint1 := func(t *testing.T, content string) []got {
		items, err := house.ParseMaintenance(write(t, content))
		if err != nil {
			t.Fatal(err)
		}
		var out []got
		for _, it := range items {
			out = append(out, got{it.ID, it.Title})
		}
		return out
	}
	cases := map[string]struct {
		parse   func(*testing.T, string) []got
		content string
		want    []got
	}{
		"tracker: final ID": {tracker1, "# P\n\n- [ ] Pay rego [tags: car] [id: b7k2m9xq]\n", []got{{"b7k2m9xq", "Pay rego"}}},
		"tracker: no ID":    {tracker1, "# P\n\n- [ ] Pay rego\n", []got{{"", "Pay rego"}}},
		"tracker: malformed ID stays in the title": {
			tracker1, "# P\n\n- [ ] Pay rego [id: abc]\n", []got{{"", "Pay rego [id: abc]"}},
		},
		"tracker: a vowel is not an ID": {
			tracker1, "# P\n\n- [ ] Pay rego [id: aaaaaaaa]\n", []got{{"", "Pay rego [id: aaaaaaaa]"}},
		},
		"tracker: indented sub-step with an ID tag stays body": {
			tracker1, "# P\n\n- [ ] Pay rego [id: b7k2m9xq]\n  - [ ] Step [id: c7k2m9xq]\n", []got{{"b7k2m9xq", "Pay rego"}},
		},
		"ideas: final ID": {ideas1, "# I\n\n- [ ] Kayak [status: parked] [id: w3th3rs7]\n", []got{{"w3th3rs7", "Kayak"}}},
		"ideas: indented checkbox before any idea starts one": {
			ideas1, "# I\n\n  - [ ] Kayak [id: w3th3rs7]\n", []got{{"w3th3rs7", "Kayak"}},
		},
		"maintenance: final ID, log entries get none": {
			maint1, "# M\n\n- [ ] Clean gutters [cadence: 6m] [id: g7tt3rs2]\n  - [x] 2026-09-01 - done\n", []got{{"g7tt3rs2", "Clean gutters"}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if g := tc.parse(t, tc.content); !slices.Equal(g, tc.want) {
				t.Errorf("got %+v, want %+v", g, tc.want)
			}
		})
	}
}
