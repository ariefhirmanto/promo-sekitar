package webpage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"

	"places-fetcher/models"

	"golang.org/x/net/html/atom"
)

// genericRule reads a page on a host with no rule: the page is one item. Its ref is a hash of the
// text, so an unchanged page is skipped and a changed one becomes a new item.
type genericRule struct{}

func (genericRule) fetch(ctx context.Context, get fetcher, req models.FetchRequest, target *url.URL, maxNew int) (resp models.FetchResult, err error) {
	page, err := get.Get(ctx, target.String())
	if err != nil {
		return resp, err
	}
	doc, err := parseHTML(page)
	if err != nil {
		return resp, err
	}
	body := findFirst(doc, byAtom(atom.Main))
	if body == nil {
		body = findFirst(doc, byAtom(atom.Article))
	}
	if body == nil {
		body = doc
	}
	txt := joinText(meta(doc, "og:title"), text(body, nil))
	if txt == "" {
		return resp, fmt.Errorf("%s has no readable text (a page built by script needs a host rule)", target)
	}
	sum := sha256.Sum256([]byte(txt))
	ref := hex.EncodeToString(sum[:8])
	if !req.KnownRefs[ref] {
		resp.Items = []models.RawItem{{Ref: ref, URL: target.String(), Text: txt}}
	}
	return resp, nil
}

// renderRule is for hosts whose pages are empty until a browser runs their script (qpon.id).
// Rendering with headless Chrome is decided in the RFC but not built yet, so these sources fail
// loudly instead of storing an empty page. Keep them paused until then.
type renderRule struct{}

func (renderRule) fetch(ctx context.Context, get fetcher, req models.FetchRequest, target *url.URL, maxNew int) (resp models.FetchResult, err error) {
	return resp, fmt.Errorf("%s pages need headless rendering, which isn't built yet; keep %s paused", target.Hostname(), req.Source.ID)
}
