package text

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type htmlRenderer struct {
	renderer.NodeRenderer
}

func newHtmlRenderer() *htmlRenderer {
	return &htmlRenderer{html.NewRenderer()}
}

func (r *htmlRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	r.NodeRenderer.RegisterFuncs(reg)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindHeading, r.renderHeading)
}

// We don't want to render paragraphs, TG doesn't support them
func (r *htmlRenderer) renderParagraph(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString("\n")
	}

	return ast.WalkContinue, nil
}

// TG doesn't support headers, so we render them as bold text
func (r *htmlRenderer) renderHeading(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<strong>")
	} else {
		_, _ = w.WriteString("</strong>\n")
	}
	return ast.WalkContinue, nil
}

// MarkdownToHtml converts user's markdown to Telegram supported subset of HTML
func MarkdownToHtml(markdown string) string {
	r := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(newHtmlRenderer(), 1000)))
	md := goldmark.New(
		goldmark.WithRenderer(r),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		// We won't face any panics as long as we use default
		// renderers and in-memory io.Writer
		panic(err)
	}

	return buf.String()
}
