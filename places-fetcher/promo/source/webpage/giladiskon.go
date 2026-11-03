package webpage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"places-fetcher/models"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// giladiskonRule reads a giladiskon.com merchant page (/merchant/<slug>). Its #deal-feed lists the
// merchant's deals as links to /deals/<slug>; each deal page holds the text in a .prose block.
// Only the first page of the feed is read (more loads by script); it is newest first.
type giladiskonRule struct{}

func (giladiskonRule) fetch(ctx context.Context, get fetcher, req models.FetchRequest, target *url.URL, maxNew int) (resp models.FetchResult, err error) {
	page, err := get.Get(ctx, target.String())
	if err != nil {
		return resp, err
	}
	doc, err := parseHTML(page)
	if err != nil {
		return resp, err
	}
	feed := findFirst(doc, byID("deal-feed"))
	if feed == nil {
		return resp, fmt.Errorf("giladiskon %s: no #deal-feed, the page layout changed", target)
	}

	var list []detail
	for _, a := range findAll(feed, func(n *html.Node) bool {
		return n.DataAtom == atom.A && strings.HasPrefix(attr(n, "href"), "/deals/")
	}) {
		slug := strings.Trim(strings.TrimPrefix(attr(a, "href"), "/deals/"), "/")
		if slug == "" || strings.Contains(slug, "/") {
			continue
		}
		list = append(list, detail{ref: slug, url: absolute(target, "/deals/"+slug), item: giladiskonDeal})
	}

	// A merchant with no current deals has an empty feed; that is not an error.
	resp.Items, err = openDetails(ctx, get, req.Source, req.KnownRefs, dedupe(list), maxNew)
	return resp, err
}

func giladiskonDeal(page []byte) (it models.RawItem, err error) {
	doc, err := parseHTML(page)
	if err != nil {
		return it, err
	}
	body := findFirst(doc, byClass("prose"))
	if body == nil {
		return it, fmt.Errorf("no .prose block")
	}
	title := meta(doc, "og:title")
	if h1 := findFirst(doc, byAtom(atom.H1)); h1 != nil {
		title = text(h1, nil)
	}
	words := text(body, nil)
	// The end date sits outside .prose (a banner, and the Product data), and many deal texts don't
	// repeat it: without it the parser can't tell a deal that ended last year from a current one.
	if until := giladiskonValidUntil(doc); until != "" && words != "" {
		words = "Berlaku sampai: " + until + "\n" + words
	}
	it.Text = joinText(title, words)
	if it.Text == "" {
		return it, fmt.Errorf("empty deal")
	}
	return it, nil
}

// giladiskonValidUntil returns the deal's offers.priceValidUntil (YYYY-MM-DD) from the page's
// schema.org Product block, or "" when there is none.
func giladiskonValidUntil(doc *html.Node) string {
	for _, s := range findAll(doc, func(n *html.Node) bool {
		return n.DataAtom == atom.Script && attr(n, "type") == "application/ld+json"
	}) {
		if s.FirstChild == nil {
			continue
		}
		var ld struct {
			Type   string `json:"@type"`
			Offers struct {
				PriceValidUntil string `json:"priceValidUntil"`
			} `json:"offers"`
		}
		if json.Unmarshal([]byte(s.FirstChild.Data), &ld) != nil || ld.Type != "Product" {
			continue
		}
		if _, err := time.Parse(time.DateOnly, ld.Offers.PriceValidUntil); err == nil {
			return ld.Offers.PriceValidUntil
		}
	}
	return ""
}
