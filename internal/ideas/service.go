package ideas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Service struct {
	ideasDir string
	mu       sync.Mutex // serialise all file writes
}

func NewService(ideasDir string) *Service {
	return &Service{ideasDir: ideasDir}
}

// List returns all ideas grouped by status directory.
func (s *Service) List() ([]Idea, error) {
	var all []Idea
	for _, status := range []string{"untriaged", "parked", "dropped"} {
		dir := filepath.Join(s.ideasDir, status)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			idea, err := ParseIdea(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			idea.Status = status
			all = append(all, *idea)
		}
	}
	return all, nil
}

// Get returns a single idea by slug, searching all status directories.
func (s *Service) Get(slug string) (*Idea, error) {
	for _, status := range []string{"untriaged", "parked", "dropped"} {
		path := filepath.Join(s.ideasDir, status, slug+".md")
		if _, err := os.Stat(path); err == nil {
			idea, err := ParseIdea(path)
			if err != nil {
				return nil, err
			}
			idea.Status = status
			return idea, nil
		}
	}
	return nil, fmt.Errorf("idea %q not found", slug)
}

// Add creates a new idea file in the untriaged directory.
func (s *Service) Add(idea *Idea) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if idea.Date == "" {
		idea.Date = time.Now().Format("2006-01-02")
	}

	dir := filepath.Join(s.ideasDir, "untriaged")
	return WriteIdea(dir, idea)
}

// Triage moves an idea between status directories or assigns it to a project.
func (s *Service) Triage(slug, action, targetProject, projectsDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the idea.
	var srcPath, srcStatus string
	for _, status := range []string{"untriaged", "parked", "dropped"} {
		path := filepath.Join(s.ideasDir, status, slug+".md")
		if _, err := os.Stat(path); err == nil {
			srcPath = path
			srcStatus = status
			break
		}
	}
	if srcPath == "" {
		return fmt.Errorf("idea %q not found", slug)
	}

	switch action {
	case "park":
		return s.moveIdea(srcPath, srcStatus, "parked")
	case "drop":
		return s.moveIdea(srcPath, srcStatus, "dropped")
	case "untriage":
		return s.moveIdea(srcPath, srcStatus, "untriaged")
	case "assign":
		return s.assignToProject(srcPath, slug, targetProject, projectsDir)
	default:
		return fmt.Errorf("unknown triage action %q", action)
	}
}

func (s *Service) moveIdea(srcPath, srcStatus, destStatus string) error {
	if srcStatus == destStatus {
		return nil
	}
	destPath := filepath.Join(s.ideasDir, destStatus, filepath.Base(srcPath))
	return os.Rename(srcPath, destPath)
}

func (s *Service) assignToProject(srcPath, _, projectSlug, projectsDir string) error {
	if projectSlug == "" {
		return fmt.Errorf("target project required for assign action")
	}

	idea, err := ParseIdea(srcPath)
	if err != nil {
		return err
	}

	// Append to project's backlog.md.
	backlogPath := filepath.Join(projectsDir, projectSlug, "backlog.md")
	if err := appendToBacklog(backlogPath, idea); err != nil {
		return fmt.Errorf("appending to backlog: %w", err)
	}

	// Remove the idea file.
	return os.Remove(srcPath)
}

func appendToBacklog(path string, idea *Idea) error {
	// Read existing backlog or create skeleton.
	data, err := os.ReadFile(path)
	if err != nil {
		data = []byte("# Backlog\n\n## Active\n")
	}

	content := string(data)

	// Find ## Active section and append after it.
	entry := fmt.Sprintf("\n### %s\n- priority: medium\n- added: %s\n\n%s\n",
		idea.Title, idea.Date, bodyWithoutTitle(idea.Body))

	idx := strings.Index(content, "## Active")
	if idx < 0 {
		content += "\n## Active\n"
		idx = len(content) - 1
	}

	// Find end of Active section (next ## or end of file).
	rest := content[idx+len("## Active"):]
	nextSection := strings.Index(rest, "\n## ")
	if nextSection < 0 {
		content += entry
	} else {
		insertAt := idx + len("## Active") + nextSection
		content = content[:insertAt] + entry + content[insertAt:]
	}

	return os.WriteFile(path, []byte(content), 0o644)
}

func bodyWithoutTitle(body string) string {
	lines := strings.SplitN(body, "\n", 2)
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") {
		if len(lines) > 1 {
			return strings.TrimSpace(lines[1])
		}
		return ""
	}
	return body
}

// Delete removes an idea file from any status directory.
func (s *Service) Delete(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, status := range []string{"untriaged", "parked", "dropped"} {
		path := filepath.Join(s.ideasDir, status, slug+".md")
		if _, err := os.Stat(path); err == nil {
			return os.Remove(path)
		}
	}
	return fmt.Errorf("idea %q not found", slug)
}

// AddResearch writes research content for an idea.
func (s *Service) AddResearch(slug string, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.ideasDir, "research", slug+".md")
	return os.WriteFile(path, []byte(content), 0o644)
}

// GetResearch reads research content for an idea.
func (s *Service) GetResearch(slug string) ([]byte, error) {
	path := filepath.Join(s.ideasDir, "research", slug+".md")
	return os.ReadFile(path)
}
