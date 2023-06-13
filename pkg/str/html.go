package str

import (
	"io"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// MarkdownToHtml converts user's markdown to Telegram supported subset of HTML
func MarkdownToHtml(md string) string {
	extensions := parser.CommonExtensions | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse([]byte(md))

	htmlFlags := html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags, RenderNodeHook: myRenderHook}
	renderer := html.NewRenderer(opts)

	return strings.TrimSpace(string(markdown.Render(doc, renderer)))
}

// We don't want to render paragraphs, TG doesn't support them
func renderParagraph() {

}

// TG doesn't support headers, so we render them as bold text
func renderHeader(w io.Writer, entering bool) {
	if entering {
		io.WriteString(w, "<b>")
	} else {
		io.WriteString(w, "</b>\n\n")
	}
}

func myRenderHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	if _, ok := node.(*ast.Paragraph); ok {
		renderParagraph()
		return ast.GoToNext, true
	}
	if _, ok := node.(*ast.Heading); ok {
		renderHeader(w, entering)
		return ast.GoToNext, true
	}
	return ast.GoToNext, false
}
