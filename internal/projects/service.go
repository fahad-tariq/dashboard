package projects

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Project struct {
	ID            int64    `json:"id"`
	Slug          string   `json:"slug"`
	Path          string   `json:"path"`
	Status        string   `json:"status"`
	Tags          []string `json:"tags,omitempty"`
	LastCommit    string   `json:"last_commit,omitempty"`
	LastCommitAgo string   `json:"last_commit_ago,omitempty"`
	RecentCommit  string   `json:"recent_commit,omitempty"` // latest commit message
	Activity      string   `json:"activity,omitempty"`      // unicode sparkline
	Created       string   `json:"created"`
	Updated       string   `json:"updated"`
	BacklogLen    int      `json:"backlog_count"`
	IsStale       bool     `json:"is_stale"`
}

// ProjectDetail holds expanded info for a single project row.
type ProjectDetail struct {
	Slug       string
	Commits    []Commit
	BacklogTop []BacklogItem
}

type Commit struct {
	Hash    string
	Message string
	Date    string
	Ago     string
}

type Service struct {
	db          *sql.DB
	projectsDir string
}

func NewService(db *sql.DB, projectsDir string) *Service {
	return &Service{db: db, projectsDir: projectsDir}
}

// Scan walks the projects directory, finds dirs containing README.md,
// and upserts them into the database.
func (s *Service) Scan() error {
	entries, err := os.ReadDir(s.projectsDir)
	if err != nil {
		return fmt.Errorf("reading projects dir: %w", err)
	}

	seen := make(map[string]bool)

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		dir := filepath.Join(s.projectsDir, e.Name())
		readme := filepath.Join(dir, "README.md")
		if _, err := os.Stat(readme); err != nil {
			continue
		}

		seen[e.Name()] = true

		if err := s.upsert(e.Name(), dir); err != nil {
			return fmt.Errorf("upserting project %q: %w", e.Name(), err)
		}

		if commitDate := gitLastCommit(dir); commitDate != "" {
			s.db.Exec(`
				UPDATE projects SET last_commit = ? WHERE slug = ?
			`, commitDate, e.Name())
		}
	}

	// Remove projects that no longer exist on disk.
	s.pruneStale(seen)

	return nil
}

func (s *Service) pruneStale(seen map[string]bool) {
	rows, err := s.db.Query("SELECT slug FROM projects")
	if err != nil {
		return
	}
	defer rows.Close()

	var toDelete []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err == nil && !seen[slug] {
			toDelete = append(toDelete, slug)
		}
	}
	for _, slug := range toDelete {
		s.db.Exec("DELETE FROM projects WHERE slug = ?", slug)
		slog.Info("removed stale project", "slug", slug)
	}
}

func gitLastCommit(dir string) string {
	cmd := exec.Command("git", "-C", dir, "log", "-1", "--format=%aI")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format("2006-01-02T15:04:05")
	}
	return s
}

// gitRecentCommitMessage returns the latest commit's subject line.
func gitRecentCommitMessage(dir string) string {
	cmd := exec.Command("git", "-C", dir, "log", "-1", "--format=%s")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	msg := strings.TrimSpace(string(out))
	if len(msg) > 60 {
		msg = msg[:57] + "..."
	}
	return msg
}

// gitRecentCommits returns the last n commits with hash, message, and date.
func gitRecentCommits(dir string, n int) []Commit {
	cmd := exec.Command("git", "-C", dir, "log", fmt.Sprintf("-%d", n), "--format=%h|%s|%aI")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var commits []Commit
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		msg := parts[1]
		if len(msg) > 72 {
			msg = msg[:69] + "..."
		}
		c := Commit{Hash: parts[0], Message: msg, Date: parts[2]}
		if t, err := time.Parse(time.RFC3339, parts[2]); err == nil {
			c.Ago = timeAgo(t)
		}
		commits = append(commits, c)
	}
	return commits
}

// gitActivitySparkline returns a unicode sparkline of commit frequency
// over the last 30 days.
func gitActivitySparkline(dir string) string {
	cmd := exec.Command("git", "-C", dir, "log", "--format=%aI", "--since=30 days ago")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Bucket commits by day offset (0 = today, 29 = 30 days ago).
	buckets := make([]int, 30)
	now := time.Now()
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(line))
		if err != nil {
			continue
		}
		daysAgo := int(now.Sub(t).Hours() / 24)
		if daysAgo >= 0 && daysAgo < 30 {
			buckets[29-daysAgo]++ // index 0 = oldest, 29 = today
		}
	}

	// Find max for scaling.
	maxVal := 0
	for _, v := range buckets {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		return strings.Repeat(" ", 30)
	}

	// Map to sparkline characters.
	bars := []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	var b strings.Builder
	for _, v := range buckets {
		if v == 0 {
			b.WriteRune(' ')
		} else {
			idx := min((v*8)/maxVal, 8)
			b.WriteRune(bars[idx])
		}
	}
	return b.String()
}

func (s *Service) upsert(slug, path string) error {
	_, err := s.db.Exec(`
		INSERT INTO projects (slug, path)
		VALUES (?, ?)
		ON CONFLICT(slug) DO UPDATE SET
			path = excluded.path,
			updated = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime')
	`, slug, path)
	return err
}

// List returns all projects ordered by status then name, with git metadata.
func (s *Service) List() ([]Project, error) {
	rows, err := s.db.Query(`
		SELECT id, slug, path, status, tags, last_commit, created, updated
		FROM projects
		ORDER BY
			CASE status
				WHEN 'active' THEN 0
				WHEN 'paused' THEN 1
				WHEN 'archived' THEN 2
			END,
			slug
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		var tags sql.NullString
		var lastCommit sql.NullString
		if err := rows.Scan(&p.ID, &p.Slug, &p.Path, &p.Status, &tags, &lastCommit, &p.Created, &p.Updated); err != nil {
			return nil, err
		}
		if tags.Valid && tags.String != "" {
			json.Unmarshal([]byte(tags.String), &p.Tags)
		}
		if lastCommit.Valid {
			p.LastCommit = lastCommit.String
			p.IsStale = isStale(lastCommit.String)
			if t, err := time.Parse("2006-01-02T15:04:05", lastCommit.String); err == nil {
				p.LastCommitAgo = timeAgo(t)
			}
		}
		p.RecentCommit = gitRecentCommitMessage(p.Path)
		p.Activity = gitActivitySparkline(p.Path)
		p.BacklogLen = s.countBacklogItems(p.Path)
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// GetDetail returns expanded info for the expandable row.
func (s *Service) GetDetail(slug string) (*ProjectDetail, error) {
	p, err := s.Get(slug)
	if err != nil {
		return nil, err
	}

	commits := gitRecentCommits(p.Path, 5)

	backlog, _ := ParseBacklog(p.Path)
	var active []BacklogItem
	for _, item := range backlog {
		if isDoneSection(item) {
			continue
		}
		active = append(active, item)
		if len(active) >= 5 {
			break
		}
	}

	return &ProjectDetail{
		Slug:       slug,
		Commits:    commits,
		BacklogTop: active,
	}, nil
}

// Get returns a single project by slug.
func (s *Service) Get(slug string) (*Project, error) {
	var p Project
	var tags sql.NullString
	var lastCommit sql.NullString
	err := s.db.QueryRow(`
		SELECT id, slug, path, status, tags, last_commit, created, updated
		FROM projects WHERE slug = ?
	`, slug).Scan(&p.ID, &p.Slug, &p.Path, &p.Status, &tags, &lastCommit, &p.Created, &p.Updated)
	if err != nil {
		return nil, err
	}
	if tags.Valid && tags.String != "" {
		json.Unmarshal([]byte(tags.String), &p.Tags)
	}
	if lastCommit.Valid {
		p.LastCommit = lastCommit.String
		p.IsStale = isStale(lastCommit.String)
		if t, err := time.Parse("2006-01-02T15:04:05", lastCommit.String); err == nil {
			p.LastCommitAgo = timeAgo(t)
		}
	}
	p.RecentCommit = gitRecentCommitMessage(p.Path)
	p.Activity = gitActivitySparkline(p.Path)
	p.BacklogLen = s.countBacklogItems(p.Path)
	return &p, nil
}

func (s *Service) countBacklogItems(projectPath string) int {
	items, err := ParseBacklog(projectPath)
	if err != nil {
		return 0
	}
	count := 0
	for _, item := range items {
		if !isDoneSection(item) {
			count++
		}
	}
	return count
}

// isDoneSection returns true if the item belongs to a "done" section
// or is individually marked as done.
func isDoneSection(item BacklogItem) bool {
	s := strings.ToLower(item.Section)
	return s == "done" || s == "completed" || item.Done != ""
}

func isStale(isoDate string) bool {
	t, err := time.Parse("2006-01-02T15:04:05", isoDate)
	if err != nil {
		return false
	}
	return time.Since(t) > 30*24*time.Hour
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}
