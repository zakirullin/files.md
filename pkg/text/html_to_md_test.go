package text

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkdownToHtmlHeader(t *testing.T) {
	r := require.New(t)

	md := `# Header`
	html := MarkdownToHtml(md)

	r.Equal("<strong>Header</strong>\n", html)
}

func TestMarkdownToHtmlHeaderAndText(t *testing.T) {
	r := require.New(t)

	md := "# Header\nText"
	html := MarkdownToHtml(md)

	r.Equal("<strong>Header</strong>\nText\n", html)
}

func TestMarkdownToHtmlBold(t *testing.T) {
	r := require.New(t)

	md := "**bold**"
	html := MarkdownToHtml(md)

	r.Equal("<strong>bold</strong>\n", html)
}

func TestMarkdownToHtmlItalic(t *testing.T) {
	r := require.New(t)

	md := "*italic*"
	html := MarkdownToHtml(md)

	r.Equal("<em>italic</em>\n", html)
}

func TestMarkdownToHtmlInvalid(t *testing.T) {
	r := require.New(t)

	md := "__valid__**invalid"
	html := MarkdownToHtml(md)

	r.Equal("<strong>valid</strong>**invalid\n", html)
}

func TestMarkdownToHtmlMultiline(t *testing.T) {
	r := require.New(t)

	md := "line1 \n**line2**\nline3"
	html := MarkdownToHtml(md)

	r.Equal("line1\n<strong>line2</strong>\nline3\n", html)
}
