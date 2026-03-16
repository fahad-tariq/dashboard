package exploration

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type Service struct {
	dir string
	mu  sync.Mutex
}

func NewService(dir string) *Service {
	return &Service{dir: dir}
}

func (s *Service) List() ([]Exploration, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var all []Exploration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		ex, err := ParseExploration(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		all = append(all, *ex)
	}

	// Sort by date descending.
	slices.SortFunc(all, func(a, b Exploration) int {
		if a.Date > b.Date {
			return -1
		}
		if a.Date < b.Date {
			return 1
		}
		return 0
	})

	return all, nil
}

func (s *Service) Get(slug string) (*Exploration, error) {
	path := filepath.Join(s.dir, slug+".md")
	return ParseExploration(path)
}

func (s *Service) Add(e *Exploration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e.Date == "" {
		e.Date = time.Now().Format("2006-01-02")
	}
	return WriteExploration(s.dir, e)
}

func (s *Service) Update(slug, body string, tags, images []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, err := ParseExploration(filepath.Join(s.dir, slug+".md"))
	if err != nil {
		return fmt.Errorf("exploration %q not found", slug)
	}

	e.Body = body
	e.Tags = tags
	e.Images = images
	return WriteExploration(s.dir, e)
}

func (s *Service) Delete(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, slug+".md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("exploration %q not found", slug)
	}
	return os.Remove(path)
}
