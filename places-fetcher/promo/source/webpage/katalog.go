package webpage

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"places-fetcher/models"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// katalogRule reads a katalogpromosi.com brand category page (/restoran-dan-cafe/<slug>/).
// The listing holds the brand's posts as <article class="… post-<id> … category-<slug> …">; the
// sidebar and related posts on the same page lack that category and are left out.
type katalogRule struct{}

var katalogPostID = regexp.MustCompile(`^post-(\d+)$`)

func (katalogRule) fetch(ctx context.Context, get fetcher, req models.FetchRequest, target *url.URL, maxNew int) (resp models.FetchResult, err error) {
	slug := path.Base(strings.TrimSuffix(target.Path, "/"))
	if slug == "" || slug == "." || slug == "/" {
		return resp, fmt.Errorf("katalogpromosi target %s has no category slug", target)
	}

	page, err := get.Get(ctx, target.String())
	if err != nil {
		return resp, err
	}
	doc, err := parseHTML(page)
	if err != nil {
		return resp, err
	}
	if findFirst(doc, byID("main-content")) == nil {
		return resp, fmt.Errorf("katalogpromosi %s: no #main-content, the page layout changed", target)
	}

	var list []detail
	for _, art := range findAll(doc, func(n *html.Node) bool {
		return n.DataAtom == atom.Article && hasClass(n, "category-"+slug)
	}) {
		ref := ""
		for _, c := range classes(art) {
			if m := katalogPostID.FindStringSubmatch(c); m != nil {
				ref = m[1]
			}
		}
		link := findFirst(art, func(n *html.Node) bool { return n.DataAtom == atom.A && attr(n, "href") != "" })
		if ref == "" || link == nil {
			continue
		}
		list = append(list, detail{ref: ref, url: absolute(target, attr(link, "href")), item: katalogPost})
	}
	if len(list) == 0 {
		return resp, fmt.Errorf("katalogpromosi %s: no posts in category %q", target, slug)
	}

	resp.Items, err = openDetails(ctx, get, req.Source, req.KnownRefs, dedupe(list), maxNew)
	return resp, err
}

// katalogPost reads one post page: its title, publish time and entry-content without the share buttons.
func katalogPost(page []byte) (it models.RawItem, err error) {
	doc, err := parseHTML(page)
	if err != nil {
		return it, err
	}
	content := findFirst(doc, byClass("entry-content"))
	if content == nil {
		return it, fmt.Errorf("no entry-content")
	}
	title := meta(doc, "og:title")
	if h1 := findFirst(doc, byClass("entry-title")); h1 != nil {
		title = text(h1, nil)
	}
	it.Text = joinText(title, text(content, katalogBoilerplate))
	it.PostedAt = meta(doc, "article:published_time")
	if it.Text == "" {
		return it, fmt.Errorf("empty post")
	}
	return it, nil
}

// katalogBoilerplate is what every post's entry-content carries besides the post: share buttons,
// the "REKOMENDASI PROMO LAINNYA" list (other brands' promos), and the code-block banners.
func katalogBoilerplate(n *html.Node) bool {
	for _, c := range classes(n) {
		if c == "mh-social-top" || c == "mh-social-bottom" || c == "crp_related" || strings.HasPrefix(c, "code-block") {
			return true
		}
	}
	return false
}

// dedupe keeps the first listing of each ref (a post can appear in two blocks of the page).
func dedupe(list []detail) []detail {
	seen := map[string]bool{}
	out := list[:0]
	for _, d := range list {
		if !seen[d.ref] {
			seen[d.ref] = true
			out = append(out, d)
		}
	}
	return out
}
