package usecase

import (
	"context"
	"log"
	"sort"
	"strings"

	"places-fetcher/match"
	"places-fetcher/models"
	"places-fetcher/promo"

	"github.com/pkg/errors"
)

// ParseDeps turns on the parse stage: after fetching, raw items are read into promos. Without
// it (nil), a run only fetches.
type ParseDeps struct {
	Parser    promo.Parser
	Stored    promo.RawItemReader       // items left new or failed by earlier runs
	Statuses  promo.RawItemStatusWriter // the parse stage's raw_items columns
	Brands    promo.BrandReader
	Outlets   promo.OutletReader
	Promos    promo.PromoRepository
	Retries   int     // a failed item is parsed again on this many later runs
	BudgetUSD float64 // parsing stops when a run has spent this much; the rest waits for the next run
	HideBelow float64 // promos read with less confidence are saved hidden
}

// parseRun is the state of one parse stage: the sheet data it resolves against and what it wrote.
type parseRun struct {
	deps     *ParseDeps
	today    string
	sources  map[string]models.Source
	brands   []models.Brand
	brandsBy map[string]models.Brand
	outlets  map[string][]models.Outlet // by brand id
	promos   map[string]models.Promo    // by id, merged
	order    []string                   // promo ids in the order first read
	statuses []models.RawItemStatus
	resp     models.ParseRunResult
}

// parse reads every item that needs it (this run's new items, then stored new ones and failed
// ones with retries left) into promos, writes the promos, then each item's status. A parser
// error that isn't the item's fault stops the stage after writing what was done.
func (u *usecase) parse(ctx context.Context, sources []models.Source, fetched []models.RawItem, today string) (resp models.ParseRunResult, err error) {
	d := u.parseDeps
	items, err := u.parseCandidates(ctx, fetched)
	if err != nil {
		return resp, err
	}
	if len(items) == 0 {
		log.Println("[Promo][Parse][Usecase] No raw item to parse")
		return resp, nil
	}

	brands, err := d.Brands.GetBrands(ctx)
	if err != nil {
		log.Println("[Promo][Parse][Usecase] Problem getting brands, err: ", err.Error())
		return resp, errors.WithMessage(err, "promoUC.Parse.GetBrands")
	}
	outlets, err := d.Outlets.GetOutlets(ctx)
	if err != nil {
		log.Println("[Promo][Parse][Usecase] Problem getting outlets, err: ", err.Error())
		return resp, errors.WithMessage(err, "promoUC.Parse.GetOutlets")
	}

	r := &parseRun{deps: d, today: today, sources: map[string]models.Source{}, brands: brands,
		brandsBy: map[string]models.Brand{}, outlets: map[string][]models.Outlet{}, promos: map[string]models.Promo{}}
	for _, s := range sources {
		r.sources[s.ID] = s
	}
	for _, b := range brands {
		r.brandsBy[b.ID] = b
	}
	for _, o := range outlets {
		r.outlets[o.BrandID] = append(r.outlets[o.BrandID], o)
	}

	var stopErr error
	for i, it := range items {
		if r.resp.CostUSD >= d.BudgetUSD {
			r.resp.Left = len(items) - i
			log.Printf("[Promo][Parse][Usecase] Budget of $%.2f reached; %d items wait for the next run", d.BudgetUSD, r.resp.Left)
			break
		}
		if stopErr = r.item(ctx, it); stopErr != nil {
			log.Println("[Promo][Parse][Usecase] Parser can't work this run, err: ", stopErr.Error())
			break
		}
	}

	// Promos before statuses: if promos can't be stored, the items stay new and are read again.
	promos := make([]models.Promo, 0, len(r.order))
	for _, id := range r.order {
		p := r.promos[id]
		promos = append(promos, p)
		if p.Status == "hidden" {
			r.resp.Hidden++
		}
	}
	r.resp.Promos = len(promos)
	if len(promos) > 0 {
		r.resp.Written, err = d.Promos.UpsertPromos(ctx, promos)
		if err != nil {
			log.Println("[Promo][Parse][Usecase] Problem upserting promos, err: ", err.Error())
			return r.resp, errors.WithMessage(err, "promoUC.Parse.UpsertPromos")
		}
	}
	if len(r.statuses) > 0 {
		if _, err = d.Statuses.UpdateRawItems(ctx, r.statuses); err != nil {
			log.Println("[Promo][Parse][Usecase] Problem updating raw item statuses, err: ", err.Error())
			return r.resp, errors.WithMessage(err, "promoUC.Parse.UpdateRawItems")
		}
	}
	if stopErr != nil {
		return r.resp, errors.WithMessage(stopErr, "promoUC.Parse.Parse")
	}
	return r.resp, nil
}

// parseCandidates puts this run's items first, then stored new items, then failed items that
// still have retries; an id is parsed once.
func (u *usecase) parseCandidates(ctx context.Context, fetched []models.RawItem) (resp []models.RawItem, err error) {
	d := u.parseDeps
	stored, err := d.Stored.GetRawItems(ctx, "new")
	if err != nil {
		log.Println("[Promo][Parse][Usecase] Problem getting new raw items, err: ", err.Error())
		return resp, errors.WithMessage(err, "promoUC.Parse.GetRawItems")
	}
	failed, err := d.Stored.GetRawItems(ctx, "failed")
	if err != nil {
		log.Println("[Promo][Parse][Usecase] Problem getting failed raw items, err: ", err.Error())
		return resp, errors.WithMessage(err, "promoUC.Parse.GetRawItems")
	}

	seen := map[string]bool{}
	add := func(it models.RawItem) {
		if !seen[it.ID] {
			seen[it.ID] = true
			resp = append(resp, it)
		}
	}
	for _, it := range fetched {
		add(it)
	}
	for _, it := range stored {
		add(it)
	}
	for _, it := range failed {
		if it.Attempts <= d.Retries {
			add(it)
		}
	}
	return resp, nil
}

// item parses one raw item and records its status. The returned error is only for a parser
// that can't work at all; everything about the item itself goes in its status.
func (r *parseRun) item(ctx context.Context, it models.RawItem) error {
	status := models.RawItemStatus{ID: it.ID, SourceID: it.SourceID, Attempts: it.Attempts, PromoIDs: []string{}}
	fail := func(reason string) {
		status.Status, status.Error = "failed", truncate(reason, maxErrorLen)
		r.statuses = append(r.statuses, status)
		r.resp.Failed++
	}

	src, ok := r.sources[it.SourceID]
	if !ok {
		fail("source " + it.SourceID + " is not in the sources tab")
		return nil
	}
	req := models.ParseRequest{Item: it, Today: r.today}
	var brand models.Brand
	if src.IsAggregator {
		for _, b := range r.brands {
			req.KnownBrands = append(req.KnownBrands, brandLabel(b))
		}
		sort.Strings(req.KnownBrands)
	} else {
		if brand, ok = r.brandsBy[src.BrandID]; !ok {
			// No call is made, so no attempt is used: the item is read once the brand is added.
			fail("brand " + src.BrandID + " is not in the brands tab")
			return nil
		}
		req.Brand = brandLabel(brand)
		req.Outlets = outletNames(r.outlets[brand.ID])
	}

	status.Attempts++
	got, err := r.deps.Parser.Parse(ctx, req)
	r.resp.CostUSD += got.CostUSD
	r.resp.CacheReadTokens += got.CacheReadTokens
	r.resp.Items++
	var perr *models.ParseError
	if errors.As(err, &perr) {
		log.Printf("[Promo][Parse][Usecase] Item %s failed, err: %s", it.ID, perr.Reason)
		fail(perr.Reason)
		return nil
	}
	if err != nil {
		r.resp.Items--
		return err
	}

	for _, pp := range got.Promos {
		p, ok := r.promo(pp, it, src, brand)
		if !ok {
			r.resp.Dropped++
			continue
		}
		if kept, dup := r.promos[p.ID]; dup {
			r.promos[p.ID] = mergePromo(kept, p)
		} else {
			r.promos[p.ID] = p
			r.order = append(r.order, p.ID)
		}
		status.PromoIDs = union(status.PromoIDs, []string{p.ID})
	}
	if len(status.PromoIDs) > 0 {
		status.Status = "parsed"
		r.resp.Parsed++
	} else {
		status.Status = "skipped"
		r.resp.Skipped++
	}
	r.statuses = append(r.statuses, status)
	return nil
}

// promo resolves a parsed promo's brand and outlets and decides whether it is shown. ok is false
// for what is never written: not a deal, or an aggregator's promo for a brand that isn't tracked.
func (r *parseRun) promo(pp models.ParsedPromo, it models.RawItem, src models.Source, brand models.Brand) (p models.Promo, ok bool) {
	if !pp.IsDeal {
		return p, false
	}
	if src.IsAggregator {
		if brand, ok = matchBrandName(pp.BrandName, r.brands); !ok {
			return p, false
		}
	}

	outletIDs, matched := matchOutlets(pp.OutletLimits, r.outlets[brand.ID])
	p = models.Promo{
		BrandID:        brand.ID,
		BrandText:      strings.TrimSpace(pp.BrandName),
		Title:          strings.TrimSpace(pp.Title),
		Deal:           strings.TrimSpace(pp.Deal),
		Channel:        pp.Channel,
		ValidFrom:      pp.ValidFrom,
		ValidUntil:     pp.ValidUntil,
		PostedAt:       r.today,
		OutletIDs:      outletIDs,
		Status:         "active",
		ActionURL:      it.URL,
		SourceIDs:      []string{src.ID},
		SourceRefs:     []string{it.ID},
		Confidence:     pp.Confidence,
		Recurrence:     strings.TrimSpace(pp.Recurrence),
		PaymentMethods: pp.PaymentMethods,
	}
	if p.BrandText == "" {
		p.BrandText = brand.Name
	}
	if len(it.PostedAt) >= 10 {
		p.PostedAt = it.PostedAt[:10]
	}
	if pp.Confidence < r.deps.HideBelow || pp.Category == "non_food" || !matched {
		p.Status = "hidden"
	}
	p.DedupeKey = dedupeKey(p)
	p.ID = promoID(p.DedupeKey)
	return p, true
}

// matchBrandName finds the tracked brand an aggregator's promo names; the whole name must be
// the brand or one of its aliases, or contain one ("Promo HokBen Hari Ini").
func matchBrandName(name string, brands []models.Brand) (models.Brand, bool) {
	b, _, ok := match.BestBrand([]string{name}, brands)
	if ok {
		return b, true
	}
	want := match.Compact(name)
	for _, b := range brands {
		for _, a := range b.Names() {
			if want != "" && match.Compact(a) == want { // short names like "KFC" match only exactly
				return b, true
			}
		}
	}
	return models.Brand{}, false
}
