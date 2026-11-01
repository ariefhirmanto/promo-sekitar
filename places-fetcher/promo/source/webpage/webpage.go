// Package webpage is the web_page source type: a page on the web, read with a rule for its host.
//
//	client.go      fetches pages (User-Agent, timeout, retries, 1 request a second per host,
//	               robots.txt) or replays saved ones in tests
//	htmltext.go    HTML → readable text
//	robots.go      robots.txt rules
//	katalog.go     katalogpromosi.com brand category pages: one item per post
//	giladiskon.go  giladiskon.com merchant pages: one item per deal
//	bca.go         promo.bca.co.id category pages: one item per promo card
//	generic.go     any other host: the page is one item (ref = hash of its text); qpon.id refuses
//
// A listing rule reads the source's target page and lists its items; only items whose ref isn't
// stored yet get their own page opened, at most MaxNewItems per run.
package webpage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/promo"
	"places-fetcher/promo/source"
)

const Name = "web_page" // the sources tab's type column

func init() {
	source.Register(Name, func(cfg config.MainConfig) (promo.SourceAdapter, error) {
		return New(cfg.WebPage, cfg.Promo.UserAgent)
	})
}

// rule reads one host's pages.
type rule interface {
	fetch(ctx context.Context, get fetcher, req models.FetchRequest, target *url.URL, maxNew int) (models.FetchResult, error)
}

// rules by host, without "www."
var rules = map[string]rule{
	"katalogpromosi.com": katalogRule{},
	"giladiskon.com":     giladiskonRule{},
	"promo.bca.co.id":    bcaRule{},
	"qpon.id":            renderRule{},
}

type adapter struct {
	get    fetcher
	maxNew int
}

func New(cfg config.WebPageConfig, userAgent string) (*adapter, error) {
	if strings.TrimSpace(userAgent) == "" {
		return nil, errors.New("Promo.UserAgent must be set")
	}
	return newAdapter(newHTTPFetcher(cfg, userAgent), cfg.MaxNewItems), nil
}

func newAdapter(get fetcher, maxNew int) *adapter {
	if maxNew <= 0 {
		maxNew = 25
	}
	return &adapter{get: get, maxNew: maxNew}
}

func (a *adapter) Type() string {
	return Name
}

func (a *adapter) FetchItems(ctx context.Context, req models.FetchRequest) (resp models.FetchResult, err error) {
	target, err := url.Parse(strings.TrimSpace(req.Source.Target))
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return resp, fmt.Errorf("target %q is not a web page URL", req.Source.Target)
	}
	r, ok := rules[strings.TrimPrefix(strings.ToLower(target.Hostname()), "www.")]
	if !ok {
		r = genericRule{}
	}
	return r.fetch(ctx, a.get, req, target, a.maxNew)
}

// detail is one listed item whose page still has to be read.
type detail struct {
	ref, url string
	item     func(page []byte) (models.RawItem, error) // builds the item from its page
}

// openDetails opens up to maxNew pages of items not stored yet. One page failing is logged and
// skipped (the next run tries it again); every page failing fails the source.
func openDetails(ctx context.Context, get fetcher, src models.Source, known map[string]bool, list []detail, maxNew int) ([]models.RawItem, error) {
	var items []models.RawItem
	var tried, failed int
	var lastErr error
	for _, d := range list {
		if known[d.ref] {
			continue
		}
		if tried == maxNew {
			log.Printf("[Promo][FetchItems][WebPage] %s: %d new items left for the next run", src.ID, countNew(list, known)-tried)
			break
		}
		tried++
		page, err := get.Get(ctx, d.url)
		if err == nil {
			var it models.RawItem
			if it, err = d.item(page); err == nil {
				it.Ref, it.URL = d.ref, d.url
				items = append(items, it)
				continue
			}
		}
		if ctx.Err() != nil {
			return items, ctx.Err()
		}
		failed++
		lastErr = err
		log.Printf("[Promo][FetchItems][WebPage] %s: skipping %s, err: %s", src.ID, d.url, err.Error())
	}
	if tried > 0 && failed == tried {
		return nil, fmt.Errorf("every new item page failed, last: %w", lastErr)
	}
	return items, nil
}

func countNew(list []detail, known map[string]bool) int {
	n := 0
	for _, d := range list {
		if !known[d.ref] {
			n++
		}
	}
	return n
}

// absolute resolves href against the page it was found on.
func absolute(base *url.URL, href string) string {
	u, err := base.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}
	u.Fragment = ""
	return u.String()
}

// joinText puts a title above the body unless the body already starts with it.
func joinText(title, body string) string {
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if title == "" || strings.HasPrefix(body, title) {
		return body
	}
	return title + "\n" + body
}
