package server

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/k3a/html2text"
	erp "github.com/web-ridge/email-reply-parser"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// DefaultMaxBodyChars is the per-email body size cap applied when maxChars is 0.
const DefaultMaxBodyChars = 4000

const truncationMarker = "\n\n[... body truncated ...]"

// wroteLine matches the attribution line ("On <date>, <name> wrote:") that
// introduces a quoted reply, which may be wrapped onto a second line.
var wroteLine = regexp.MustCompile(`(?s)(?:^|\n)On [^\n]*(?:\n[^\n]*)?\bwrote:\s*$`)

// HTMLToText flattens an HTML email body to text the way PrepareBody sees a
// text/plain one. A <blockquote> with no new text after it in the document is
// the quoted history of a top-posted reply and is dropped, together with its
// attribution line. Every other blockquote is an interleaved quote and is kept,
// rendered as "> "-prefixed lines, nested quotes as "> > ".
func HTMLToText(rawHTML string) string {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return html2text.HTML2Text(rawHTML)
	}
	var quotes []*html.Node
	var seqs []int
	lastText, seq := -1, 0
	var scan func(n *html.Node)
	scan = func(n *html.Node) {
		seq++
		switch {
		case n.Type == html.ElementNode && n.DataAtom == atom.Blockquote:
			quotes = append(quotes, n)
			seqs = append(seqs, seq)
			return
		case n.Type == html.ElementNode && (n.DataAtom == atom.Script || n.DataAtom == atom.Style || n.DataAtom == atom.Head):
			return
		case n.Type == html.TextNode && strings.TrimSpace(n.Data) != "":
			lastText = seq
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			scan(c)
		}
	}
	scan(doc)

	dropped := false
	for i, q := range quotes {
		if seqs[i] > lastText {
			q.Parent.RemoveChild(q)
			dropped = true
			continue
		}
		quoteBlockquotes(q)
		replaceWithQuote(q)
	}
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return html2text.HTML2Text(rawHTML)
	}
	text := html2text.HTML2Text(buf.String())
	if dropped {
		text = wroteLine.ReplaceAllString(text, "")
	}
	return text
}

// quoteBlockquotes replaces the blockquotes below n with "> "-prefixed text,
// innermost first, so that nesting accumulates one prefix per level.
func quoteBlockquotes(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		quoteBlockquotes(c)
		if c.Type == html.ElementNode && c.DataAtom == atom.Blockquote {
			replaceWithQuote(c)
		}
		c = next
	}
}

// replaceWithQuote swaps a blockquote for a paragraph holding its text, one
// "> "-prefixed line per line, so html2text passes the markers through.
func replaceWithQuote(q *html.Node) {
	var inner bytes.Buffer
	for c := q.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&inner, c); err != nil {
			return
		}
	}
	body := strings.Trim(html2text.HTML2Text(inner.String()), "\r\n")
	p := &html.Node{Type: html.ElementNode, DataAtom: atom.P, Data: "p"}
	for i, line := range strings.Split(body, "\n") {
		if i > 0 {
			p.AppendChild(&html.Node{Type: html.ElementNode, DataAtom: atom.Br, Data: "br"})
		}
		p.AppendChild(&html.Node{Type: html.TextNode, Data: strings.TrimRight("> "+line, " \t\r")})
	}
	q.Parent.InsertBefore(p, q)
	q.Parent.RemoveChild(q)
}

// PrepareBody strips text-level quoted replies and signatures, then truncates
// to maxChars. Pass 0 for maxChars to use DefaultMaxBodyChars.
func PrepareBody(text string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = DefaultMaxBodyChars
	}
	stripped := erp.Parse(text)
	return TruncateBody(stripped, maxChars)
}

// TruncateBody cuts text to fit within limit characters. When truncation is
// needed, it cuts at the last newline before the limit (reserving space for the
// marker) and appends a truncation advisory.
func TruncateBody(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	budget := limit - len(truncationMarker)
	if budget <= 0 {
		return truncationMarker
	}
	cut := strings.LastIndex(text[:budget], "\n")
	if cut <= 0 {
		cut = budget
	}
	return text[:cut] + truncationMarker
}
