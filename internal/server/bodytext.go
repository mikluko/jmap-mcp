package server

import (
	"bytes"
	"strings"

	erp "github.com/web-ridge/email-reply-parser"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// DefaultMaxBodyChars is the per-email body size cap applied when maxChars is 0.
const DefaultMaxBodyChars = 4000

const truncationMarker = "\n\n[... body truncated ...]"

// StripBlockquotes parses HTML and removes all <blockquote> elements and their
// children, returning the remaining HTML. This structurally removes quoted
// replies before they get flattened into plain text by html2text.
func StripBlockquotes(rawHTML string) string {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return rawHTML
	}
	removeBlockquotes(doc)
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return rawHTML
	}
	return buf.String()
}

// removeBlockquotes walks the node tree and removes blockquote elements in place.
func removeBlockquotes(n *html.Node) {
	var next *html.Node
	for c := n.FirstChild; c != nil; c = next {
		next = c.NextSibling
		if c.Type == html.ElementNode && c.DataAtom == atom.Blockquote {
			n.RemoveChild(c)
			continue
		}
		removeBlockquotes(c)
	}
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
