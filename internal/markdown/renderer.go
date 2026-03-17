package markdown

import (
	"bytes"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
)

var md goldmark.Markdown
var sanitiser *bluemonday.Policy

func init() {
	md = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			highlighting.NewHighlighting(
				highlighting.WithStyle("monokai"),
			),
		),
	)
	sanitiser = bluemonday.UGCPolicy()
}

// Render converts markdown source to sanitised HTML.
func Render(source []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := md.Convert(source, &buf); err != nil {
		return nil, err
	}
	return sanitiser.SanitizeBytes(buf.Bytes()), nil
}
