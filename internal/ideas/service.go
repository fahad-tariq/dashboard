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
	mu       sync.Mutex
}

func NewService(ideasDir string) *Service {
	return &Service{ideasDir: ideasDir}
}

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

func (s *Service) Add(idea *Idea) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if idea.Date == "" {
		idea.Date = time.Now().Format("2006-01-02")
	}

	dir := filepath.Join(s.ideasDir, "untriaged")
	return WriteIdea(dir, idea)
}

func (s *Service) Triage(slug, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

func (s *Service) AddResearch(slug string, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.ideasDir, "research", slug+".md")
	return os.WriteFile(path, []byte(content), 0o644)
}

func (s *Service) GetResearch(slug string) ([]byte, error) {
	path := filepath.Join(s.ideasDir, "research", slug+".md")
	return os.ReadFile(path)
}
