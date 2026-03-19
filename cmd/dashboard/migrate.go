package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/ideas"
)

// runMigrateData handles the "migrate-data" CLI subcommand.
// Converts old directory-based ideas and explorations into a single ideas.md flat file.
func runMigrateData() {
	fs := flag.NewFlagSet("migrate-data", flag.ExitOnError)
	userID := fs.Int("user-id", 0, "target user ID")
	oldIdeasDir := fs.String("ideas-dir", "", "old ideas directory (with untriaged/parked/dropped subdirs)")
	oldExpDir := fs.String("explorations-dir", "", "old explorations directory")
	fs.Parse(os.Args[2:])

	if *userID <= 0 {
		fmt.Fprintln(os.Stderr, "usage: dashboard migrate-data --user-id <N> [--ideas-dir <path>] [--explorations-dir <path>]")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading config: %v\n", err)
		os.Exit(1)
	}

	userDir := fmt.Sprintf("%s/%d", cfg.UserDataDir, *userID)
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "creating user directory: %v\n", err)
		os.Exit(1)
	}

	// Move personal.md.
	migrateFile(cfg.PersonalPath, fmt.Sprintf("%s/personal.md", userDir))

	// Collect ideas from old directory structure into flat-file format.
	var allIdeas []ideas.Idea
	slugSet := map[string]bool{}

	ideasBase := *oldIdeasDir
	if ideasBase == "" {
		ideasBase = fmt.Sprintf("%s/ideas", userDir)
	}

	// Read ideas from status directories.
	for _, status := range []string{"untriaged", "parked", "dropped"} {
		dir := ideasBase + "/" + status
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			idea := migrateOldIdea(dir+"/"+e.Name(), status)
			if idea != nil {
				slugSet[idea.Slug] = true
				allIdeas = append(allIdeas, *idea)
			}
		}
	}

	// Merge research files into matching idea bodies.
	researchDir := ideasBase + "/research"
	if entries, err := os.ReadDir(researchDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			slug := strings.TrimSuffix(e.Name(), ".md")
			data, err := os.ReadFile(researchDir + "/" + e.Name())
			if err != nil {
				continue
			}
			content := strings.TrimSpace(string(data))
			if content == "" {
				continue
			}

			found := false
			for i := range allIdeas {
				if allIdeas[i].Slug == slug {
					allIdeas[i].Body += "\n\n## Research\n\n" + content
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("  orphaned research file %s -- creating standalone idea\n", e.Name())
				allIdeas = append(allIdeas, ideas.Idea{
					Slug:   slug,
					Title:  slug,
					Status: "untriaged",
					Body:   "## Research\n\n" + content,
				})
				slugSet[slug] = true
			}
		}
	}

	// Read explorations and add as parked ideas.
	expBase := *oldExpDir
	if expBase == "" {
		expBase = fmt.Sprintf("%s/explorations", userDir)
	}
	if entries, err := os.ReadDir(expBase); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			idea := migrateOldIdea(expBase+"/"+e.Name(), "parked")
			if idea != nil {
				if slugSet[idea.Slug] {
					idea.Slug += "-exp"
					idea.Title += " (exp)"
					fmt.Printf("  slug collision: renamed to %s\n", idea.Slug)
				}
				slugSet[idea.Slug] = true
				allIdeas = append(allIdeas, *idea)
			}
		}
	}

	// Write combined ideas.md.
	ideasPath := fmt.Sprintf("%s/ideas.md", userDir)
	if err := ideas.WriteIdeas(ideasPath, "Ideas", allIdeas); err != nil {
		fmt.Fprintf(os.Stderr, "writing ideas.md: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("migrated %d ideas to %s\n", len(allIdeas), ideasPath)
}

// migrateOldIdea reads an old-format idea file (frontmatter + body with # Title heading)
// and converts it to the new flat-file Idea struct.
func migrateOldIdea(path, status string) *ideas.Idea {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("  error reading %s: %v\n", path, err)
		return nil
	}

	content := string(data)
	frontmatter, body := splitOldFrontmatter(content)

	idea := &ideas.Idea{
		Status: status,
	}

	// Parse frontmatter fields.
	for line := range strings.SplitSeq(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		switch k {
		case "tags":
			idea.Tags = httputil.ParseCSV(v)
		case "type":
			// Legacy: single type becomes a tag.
			v = strings.TrimSpace(v)
			if v != "" {
				idea.Tags = append(idea.Tags, v)
			}
		case "images":
			idea.Images = httputil.ParseCSV(v)
		case "suggested-project":
			idea.Project = strings.TrimSpace(v)
		case "date":
			idea.Added = strings.TrimSpace(v)
		}
	}

	// Extract title from first # heading and strip it from body.
	body = strings.TrimSpace(body)
	lines := strings.SplitN(body, "\n", 2)
	if len(lines) > 0 {
		if title, ok := strings.CutPrefix(strings.TrimSpace(lines[0]), "# "); ok {
			idea.Title = title
			if len(lines) > 1 {
				body = strings.TrimSpace(lines[1])
			} else {
				body = ""
			}
		}
	}

	if idea.Title == "" {
		// Fall back to slug from filename.
		base := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".md")
		idea.Title = base
	}

	idea.Slug = ideas.Slugify(idea.Title)
	idea.Body = body

	return idea
}

func splitOldFrontmatter(content string) (frontmatter, body string) {
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	rest := content[4:]
	fm, after, ok := strings.Cut(rest, "\n---")
	if !ok {
		return "", content
	}
	return fm, strings.TrimPrefix(after, "\n")
}

// migrateFile moves src to dst if src exists and dst does not.
func migrateFile(src, dst string) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return
	}
	if _, err := os.Stat(dst); err == nil {
		fmt.Printf("  skip %s (already exists at %s)\n", src, dst)
		return
	}
	if err := os.MkdirAll(dst[:strings.LastIndex(dst, "/")], 0o755); err != nil {
		fmt.Printf("  error creating parent dir: %v\n", err)
		return
	}

	data, err := os.ReadFile(src)
	if err != nil {
		fmt.Printf("  error reading %s: %v\n", src, err)
		return
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		fmt.Printf("  error writing %s: %v\n", dst, err)
		return
	}
	fmt.Printf("  moved %s -> %s\n", src, dst)
}
