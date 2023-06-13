package str

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkdownToHtmlHeader(t *testing.T) {
	r := require.New(t)

	md := `# Header`
	html := MarkdownToHtml(md)

	r.Equal("<h1>Header</h1>", html)
}

func TestMarkdownToHtmlBold(t *testing.T) {
	r := require.New(t)

	md := `**Bold**`
	html := MarkdownToHtml(md)

	r.Equal("<strong>Bold</strong>", html)
}
