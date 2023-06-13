package str

import (
	"io"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

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

func myRenderHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	if _, ok := node.(*ast.Paragraph); ok {
		renderParagraph()
		return ast.GoToNext, true
	}
	return ast.GoToNext, false
}
