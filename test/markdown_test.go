package test

import (
	"testing"

	"github.com/fahad/dashboard/internal/markdown"
)

func TestRenderGolden(t *testing.T) {
	tests := map[string]string{
		"fenced_code": "Intro paragraph.\n\n```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n\nAfter.\n",
		"gfm":         "# Heading\n\n- [x] done\n- [ ] todo\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n~~struck~~ https://example.com\n",
		"unsafe_html": "<script>alert(1)</script>\n\n[link](javascript:alert(1)) <img src=x onerror=alert(1)>\n",
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := markdown.Render([]byte(src))
			if err != nil {
				t.Fatal(err)
			}
			assertGolden(t, "markdown/"+name+".html", got)
		})
	}
}
