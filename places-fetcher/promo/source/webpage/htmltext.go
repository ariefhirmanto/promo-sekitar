package webpage

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func parseHTML(b []byte) (*html.Node, error) {
	return html.Parse(bytes.NewReader(b))
}

// findAll returns every element under n (n included) that match accepts, in document order.
func findAll(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && match(n) {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func findFirst(n *html.Node, match func(*html.Node) bool) *html.Node {
	if all := findAll(n, match); len(all) > 0 {
		return all[0]
	}
	return nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func classes(n *html.Node) []string {
	return strings.Fields(attr(n, "class"))
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range classes(n) {
		if c == class {
			return true
		}
	}
	return false
}

func byID(id string) func(*html.Node) bool {
	return func(n *html.Node) bool { return attr(n, "id") == id }
}

func byClass(class string) func(*html.Node) bool {
	return func(n *html.Node) bool { return hasClass(n, class) }
}

func byAtom(a atom.Atom) func(*html.Node) bool {
	return func(n *html.Node) bool { return n.DataAtom == a }
}

// meta returns the content of <meta property=key> or <meta name=key>.
func meta(doc *html.Node, key string) string {
	m := findFirst(doc, func(n *html.Node) bool {
		return n.DataAtom == atom.Meta && (attr(n, "property") == key || attr(n, "name") == key)
	})
	if m == nil {
		return ""
	}
	return strings.TrimSpace(attr(m, "content"))
}

// skipped elements never hold the promo's words.
var skipped = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Svg: true, atom.Template: true,
	atom.Iframe: true, atom.Form: true, atom.Button: true, atom.Nav: true, atom.Header: true,
	atom.Footer: true, atom.Aside: true,
}

var blocks = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Li: true, atom.Br: true, atom.Tr: true, atom.Ul: true,
	atom.Ol: true, atom.Section: true, atom.Article: true, atom.H1: true, atom.H2: true, atom.H3: true,
	atom.H4: true, atom.H5: true, atom.H6: true, atom.Table: true, atom.Blockquote: true, atom.Hr: true,
	atom.Main: true, atom.Figure: true, atom.Dl: true, atom.Dt: true, atom.Dd: true,
}

// text returns the readable words under n: one line per block element, list items as "- ",
// runs of spaces collapsed, empty lines dropped. skip, if set, leaves out matching subtrees.
func text(n *html.Node, skip func(*html.Node) bool) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
			return
		case html.ElementNode:
			if skipped[n.DataAtom] || (skip != nil && skip(n)) {
				return
			}
			if blocks[n.DataAtom] {
				b.WriteString("\n")
			}
			if n.DataAtom == atom.Li {
				b.WriteString("- ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blocks[n.DataAtom] {
			b.WriteString("\n")
		}
	}
	walk(n)

	var lines []string
	for _, line := range strings.Split(b.String(), "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" && line != "-" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// htmlText is text() of an HTML fragment, e.g. a description field holding markup.
func htmlText(fragment string) string {
	doc, err := parseHTML([]byte(fragment))
	if err != nil {
		return strings.TrimSpace(fragment)
	}
	return text(doc, nil)
}
