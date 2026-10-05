package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"regexp"
	"slices"
	"testing"

	"github.com/fahad/dashboard/web"
)

// Module templates and the shared components carry no inline event handlers,
// so the CSP's script-src can drop 'unsafe-inline' once core pages follow.
func TestModuleTemplatesHaveNoInlineHandlers(t *testing.T) {
	handler := regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
	files, err := fs.Glob(web.TemplateFS, "templates/*/*.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no module or component templates found")
	}
	for _, f := range files {
		b, err := fs.ReadFile(web.TemplateFS, f)
		if err != nil {
			t.Fatal(err)
		}
		if m := handler.Find(b); m != nil {
			t.Errorf("%s has an inline handler: %q", f, m)
		}
	}
}

// Only linkify, which escapes every segment it writes, may return
// template.HTML from the func map; anything else would bypass escaping.
func TestFuncMapHTMLFunctionsLimited(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../internal/app/templates.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var html []string
	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.BasicLit)
		fn, isFunc := kv.Value.(*ast.FuncLit)
		if !ok || !isFunc || fn.Type.Results == nil {
			return true
		}
		for _, r := range fn.Type.Results.List {
			if sel, ok := r.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "HTML" {
				html = append(html, key.Value)
			}
		}
		return true
	})
	if !slices.Equal(html, []string{`"linkify"`}) {
		t.Errorf("func map entries returning template.HTML = %v, want only linkify", html)
	}
}
