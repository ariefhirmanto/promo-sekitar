package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"places-fetcher/models"
	"places-fetcher/promo"
)

// fakes for the parse stage's ports

type fakeParser struct {
	byItem map[string]models.ParseResult // by raw item id
	errs   map[string]error
	cost   float64
	got    []models.ParseRequest
}

func (f *fakeParser) Parse(ctx context.Context, req models.ParseRequest) (models.ParseResult, error) {
	f.got = append(f.got, req)
	res := f.byItem[req.Item.ID]
	res.CostUSD = f.cost
	return res, f.errs[req.Item.ID]
}

type memStored map[string][]models.RawItem // by status

func (m memStored) GetRawItems(ctx context.Context, status string) ([]models.RawItem, error) {
	return m[status], nil
}

type memRawStatuses struct{ got []models.RawItemStatus }

func (m *memRawStatuses) UpdateRawItems(ctx context.Context, s []models.RawItemStatus) (models.WriteResult, error) {
	m.got = s
	return models.WriteResult{Updated: len(s)}, nil
}

type memBrands []models.Brand

func (m memBrands) GetBrands(ctx context.Context) ([]models.Brand, error) { return m, nil }

type memOutlets []models.Outlet

func (m memOutlets) GetOutlets(ctx context.Context) ([]models.Outlet, error) { return m, nil }

type memPromos struct {
	got []models.Promo
	err error
}

func (m *memPromos) UpsertPromos(ctx context.Context, p []models.Promo) (models.WriteResult, error) {
	m.got = p
	return models.WriteResult{Added: len(p)}, m.err
}

var (
	brands = memBrands{
		{ID: "brand_miecontoh", Name: "Mie Contoh", Aliases: "Mie Contoh+"},
		{ID: "brand_kopi", Name: "Kopi Contoh"},
	}
	outlets = memOutlets{
		{ID: "out_osm_n1", BrandID: "brand_miecontoh", Name: "Mie Contoh Mal Contoh Raya", MallName: "Mal Contoh Raya", Address: "Jl. Contoh 1, Depok"},
		{ID: "out_osm_n2", BrandID: "brand_miecontoh", Name: "Mie Contoh Bekasi", Address: "Jl. Raya 2, Bekasi", Format: "Mie Contoh+"},
		{ID: "out_osm_n3", BrandID: "brand_kopi", Name: "Kopi Contoh Depok", Address: "Depok"},
	}
	parseSources = memSources{
		{ID: "src_miecontoh_katalog", Type: "web_page", BrandID: "brand_miecontoh", Status: "paused"},
		{ID: "src_bank", Type: "web_page", IsAggregator: true, Status: "paused"},
		{ID: "src_newbrand", Type: "web_page", BrandID: "brand_missing", Status: "paused"},
	}
)

func deal(title, d string, conf float64) models.ParsedPromo {
	return models.ParsedPromo{IsDeal: true, BrandName: "Mie Contoh", Title: title, Deal: d, Channel: "dine_in",
		ValidFrom: "2026-11-03", ValidUntil: "2026-11-09", Category: "food", Confidence: conf}
}

type parseFixture struct {
	parser   *fakeParser
	stored   memStored
	statuses *memRawStatuses
	promos   *memPromos
	budget   float64
}

func newParseFixture() *parseFixture {
	return &parseFixture{parser: &fakeParser{byItem: map[string]models.ParseResult{}, errs: map[string]error{}},
		stored: memStored{}, statuses: &memRawStatuses{}, promos: &memPromos{}, budget: 1}
}

func (f *parseFixture) run(t *testing.T) (models.PromoRunResult, error) {
	t.Helper()
	deps := &ParseDeps{Parser: f.parser, Stored: f.stored, Statuses: f.statuses, Brands: brands, Outlets: outlets,
		Promos: f.promos, Retries: 3, BudgetUSD: f.budget}
	uc := NewPromoUsecase(parseSources, &memStatuses{}, adapters{}, &memRaw{}, memRefs{}, 3, time.Minute, deps)
	return uc.RunSources(context.Background(), promo.InputRun{Now: now})
}

func (f *parseFixture) status(id string) models.RawItemStatus {
	for _, s := range f.statuses.got {
		if s.ID == id {
			return s
		}
	}
	return models.RawItemStatus{}
}

func item(src, ref string) models.RawItem {
	return models.RawItem{ID: models.RawItemID(src, ref), SourceID: src, Ref: ref, URL: "https://example.com/" + ref,
		PostedAt: "2026-11-01T10:00:00+07:00", Text: "promo text"}
}

func TestParseBrandSourceItem(t *testing.T) {
	f := newParseFixture()
	it := item("src_miecontoh_katalog", "1001")
	f.stored["new"] = []models.RawItem{it}
	p := deal("Beli 2 Mie Ayam Rp35.000", "2 mangkok Rp35.000", 0.95)
	p.OutletLimits = []models.OutletLimit{{Kind: "mall", Name: "Mal Contoh Raya"}, {Kind: "city", Name: "Bekasi"}}
	p.PaymentMethods = []string{"QRIS"}
	f.parser.byItem[it.ID] = models.ParseResult{Promos: []models.ParsedPromo{p}}

	res, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	req := f.parser.got[0]
	if req.Brand != "Mie Contoh (Mie Contoh+)" || req.Today != "2026-11-02" || len(req.KnownBrands) != 0 {
		t.Fatalf("a brand source sends its brand: %+v", req)
	}
	if strings.Join(req.Outlets, "|") != "Mal Contoh Raya|Mie Contoh Bekasi|Mie Contoh Mal Contoh Raya" {
		t.Fatalf("outlet names sorted, no duplicates, only this brand: %q", req.Outlets)
	}

	if len(f.promos.got) != 1 {
		t.Fatalf("want 1 promo, got %+v", f.promos.got)
	}
	got := f.promos.got[0]
	if got.BrandID != "brand_miecontoh" || got.Status != "active" || got.PostedAt != "2026-11-01" ||
		got.ActionURL != it.URL || strings.Join(got.OutletIDs, ",") != "out_osm_n1,out_osm_n2" ||
		strings.Join(got.SourceIDs, ",") != "src_miecontoh_katalog" || strings.Join(got.SourceRefs, ",") != it.ID ||
		strings.Join(got.PaymentMethods, ",") != "QRIS" {
		t.Fatalf("promo fields: %+v", got)
	}
	if !strings.HasPrefix(got.ID, "promo_") || got.DedupeKey != "brand_miecontoh|2_35_000|2026-11-03|2026-11-09|out_osm_n1,out_osm_n2" {
		t.Fatalf("id and dedupe key: %s %s", got.ID, got.DedupeKey)
	}
	st := f.status(it.ID)
	if st.Status != "parsed" || st.Attempts != 1 || st.Error != "" || strings.Join(st.PromoIDs, ",") != got.ID {
		t.Fatalf("item status: %+v", st)
	}
	if res.Parse.Items != 1 || res.Parse.Parsed != 1 || res.Parse.Promos != 1 || res.Parse.Hidden != 0 {
		t.Fatalf("summary: %+v", res.Parse)
	}
}

func TestParseHiddenRules(t *testing.T) {
	f := newParseFixture()
	it := item("src_miecontoh_katalog", "1001")
	f.stored["new"] = []models.RawItem{it}
	low := deal("Low", "Diskon 10%", 0.5)
	merch := deal("Merch", "Tumbler Rp99.000", 0.9)
	merch.Category = "non_food"
	elsewhere := deal("Elsewhere", "Diskon 20%", 0.9)
	elsewhere.OutletLimits = []models.OutletLimit{{Kind: "mall", Name: "Mal Tidak Ada"}}
	shown := deal("Shown", "Diskon 30%", 0.6)
	f.parser.byItem[it.ID] = models.ParseResult{Promos: []models.ParsedPromo{low, merch, elsewhere, shown}}

	res, _ := f.run(t)
	status := map[string]string{}
	for _, p := range f.promos.got {
		status[p.Title] = p.Status
	}
	want := map[string]string{"Low": "hidden", "Merch": "hidden", "Elsewhere": "hidden", "Shown": "active"}
	for k, v := range want {
		if status[k] != v {
			t.Errorf("%s: status %q, want %q", k, status[k], v)
		}
	}
	if res.Parse.Hidden != 3 {
		t.Fatalf("hidden count: %+v", res.Parse)
	}
}

func TestParseAggregatorResolvesBrands(t *testing.T) {
	f := newParseFixture()
	it := item("src_bank", "abc")
	f.stored["new"] = []models.RawItem{it}
	tracked := deal("Diskon Kopi", "Diskon 25%", 0.9)
	tracked.BrandName = "KOPI CONTOH"
	tracked.Channel = "all"
	untracked := deal("Other", "Diskon 50%", 0.9)
	untracked.BrandName = "Warung Lain"
	notDeal := deal("Menu baru", "Rp30.000", 0.9)
	notDeal.IsDeal = false
	f.parser.byItem[it.ID] = models.ParseResult{Promos: []models.ParsedPromo{tracked, untracked, notDeal}}

	res, _ := f.run(t)
	req := f.parser.got[0]
	if req.Brand != "" || len(req.Outlets) != 0 || strings.Join(req.KnownBrands, "|") != "Kopi Contoh|Mie Contoh (Mie Contoh+)" {
		t.Fatalf("an aggregator sends the tracked brands: %+v", req)
	}
	if len(f.promos.got) != 1 || f.promos.got[0].BrandID != "brand_kopi" || f.promos.got[0].BrandText != "KOPI CONTOH" {
		t.Fatalf("only the tracked brand's deal is written: %+v", f.promos.got)
	}
	if res.Parse.Dropped != 2 {
		t.Fatalf("untracked brand and menu launch are dropped: %+v", res.Parse)
	}
}

func TestParseMergesTheSamePromoFromTwoItems(t *testing.T) {
	f := newParseFixture()
	a, b := item("src_miecontoh_katalog", "1001"), item("src_bank", "xyz")
	f.stored["new"] = []models.RawItem{a, b}
	pa := deal("Diskon 50%", "Diskon 50%", 0.7)
	pb := deal("50% off Mie Contoh", "50% off", 0.9)
	f.parser.byItem[a.ID] = models.ParseResult{Promos: []models.ParsedPromo{pa}}
	f.parser.byItem[b.ID] = models.ParseResult{Promos: []models.ParsedPromo{pb}}

	f.run(t)
	if len(f.promos.got) != 1 {
		t.Fatalf("one promo after merging: %+v", f.promos.got)
	}
	got := f.promos.got[0]
	if got.Title != "50% off Mie Contoh" || strings.Join(got.SourceIDs, ",") != "src_miecontoh_katalog,src_bank" ||
		strings.Join(got.SourceRefs, ",") != a.ID+","+b.ID {
		t.Fatalf("the more confident reading wins, sources add up: %+v", got)
	}
	if f.status(a.ID).PromoIDs[0] != got.ID || f.status(b.ID).PromoIDs[0] != got.ID {
		t.Fatal("both items point at the merged promo")
	}
}

func TestParseFailuresAndRetries(t *testing.T) {
	f := newParseFixture()
	bad := item("src_miecontoh_katalog", "bad")
	retry := item("src_miecontoh_katalog", "retry")
	retry.Status, retry.Attempts = "failed", 2
	spent := item("src_miecontoh_katalog", "spent")
	spent.Status, spent.Attempts = "failed", 4
	nobrand := item("src_newbrand", "1")
	f.stored["new"] = []models.RawItem{bad, nobrand}
	f.stored["failed"] = []models.RawItem{retry, spent}
	f.parser.errs[bad.ID] = &models.ParseError{Reason: "invalid reply after a retry: promos[0].valid_until"}

	res, err := f.run(t)
	if err != nil {
		t.Fatalf("an item failure is not a run failure: %v", err)
	}
	if st := f.status(bad.ID); st.Status != "failed" || st.Attempts != 1 || !strings.Contains(st.Error, "valid_until") {
		t.Fatalf("bad item: %+v", st)
	}
	if st := f.status(retry.ID); st.Status != "skipped" || st.Attempts != 3 || st.Error != "" {
		t.Fatalf("a retry that works clears the error: %+v", st)
	}
	if st := f.status(spent.ID); st.ID != "" {
		t.Fatalf("an item out of retries is left alone: %+v", st)
	}
	if st := f.status(nobrand.ID); st.Status != "failed" || st.Attempts != 0 || !strings.Contains(st.Error, "brand_missing") {
		t.Fatalf("a missing brand fails without using an attempt: %+v", st)
	}
	if len(f.parser.got) != 2 || res.Parse.Failed != 2 {
		t.Fatalf("no call for the missing brand or the spent item: %d calls, %+v", len(f.parser.got), res.Parse)
	}
}

func TestParseStopsAtBudget(t *testing.T) {
	f := newParseFixture()
	f.budget, f.parser.cost = 0.5, 0.3
	f.stored["new"] = []models.RawItem{item("src_miecontoh_katalog", "1"), item("src_miecontoh_katalog", "2"), item("src_miecontoh_katalog", "3")}

	res, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.parser.got) != 2 || res.Parse.Left != 1 || len(f.statuses.got) != 2 || res.Parse.CostUSD != 0.6 {
		t.Fatalf("parsing stops once the budget is reached; the rest stays new: %+v", res.Parse)
	}
}

func TestParseAPIErrorStopsAfterWritingWhatWasDone(t *testing.T) {
	f := newParseFixture()
	ok, down := item("src_miecontoh_katalog", "ok"), item("src_miecontoh_katalog", "down")
	f.stored["new"] = []models.RawItem{ok, down, item("src_miecontoh_katalog", "later")}
	f.parser.byItem[ok.ID] = models.ParseResult{Promos: []models.ParsedPromo{deal("A", "Diskon 10%", 0.9)}}
	f.parser.errs[down.ID] = errors.New("401 invalid x-api-key")

	_, err := f.run(t)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a parser that can't work fails the run: %v", err)
	}
	if len(f.parser.got) != 2 || len(f.promos.got) != 1 || len(f.statuses.got) != 1 || f.status(ok.ID).Status != "parsed" {
		t.Fatalf("what was parsed is written, the rest stays new: %d calls, %+v", len(f.parser.got), f.statuses.got)
	}
}

func TestParsePromoWriteFailureLeavesItemsNew(t *testing.T) {
	f := newParseFixture()
	it := item("src_miecontoh_katalog", "1")
	f.stored["new"] = []models.RawItem{it}
	f.parser.byItem[it.ID] = models.ParseResult{Promos: []models.ParsedPromo{deal("A", "Diskon 10%", 0.9)}}
	f.promos.err = errors.New("sheet: busy")

	_, err := f.run(t)
	if err == nil || !strings.Contains(err.Error(), "UpsertPromos") || f.statuses.got != nil {
		t.Fatalf("statuses wait for the promos: %v %+v", err, f.statuses.got)
	}
}

func TestParseThisRunsItemsFirstOnce(t *testing.T) {
	srcs := memSources{{ID: "src_miecontoh_katalog", Type: "web_page", BrandID: "brand_miecontoh", Status: "active"}}
	a := fakeAdapter{results: map[string]models.FetchResult{"src_miecontoh_katalog": {Items: []models.RawItem{{Ref: "1001", URL: "u"}}}}}
	f := newParseFixture()
	f.stored["new"] = []models.RawItem{item("src_miecontoh_katalog", "1001"), item("src_miecontoh_katalog", "0999")}
	deps := &ParseDeps{Parser: f.parser, Stored: f.stored, Statuses: f.statuses, Brands: brands, Outlets: outlets,
		Promos: f.promos, Retries: 3, BudgetUSD: 1}
	uc := NewPromoUsecase(srcs, &memStatuses{}, adapters{"web_page": a}, &memRaw{}, memRefs{}, 3, time.Minute, deps)
	if _, err := uc.RunSources(context.Background(), promo.InputRun{Now: now}); err != nil {
		t.Fatal(err)
	}
	if len(f.parser.got) != 2 || f.parser.got[0].Item.Ref != "1001" || f.parser.got[1].Item.Ref != "0999" {
		t.Fatalf("this run's item first, stored copy not parsed twice: %+v", f.parser.got)
	}
}

func TestDedupeKeyMeetsAcrossWordings(t *testing.T) {
	a := models.Promo{BrandID: "brand_x", Deal: "Diskon 50% maks. Rp25.000", ValidUntil: "2026-11-30"}
	b := models.Promo{BrandID: "brand_x", Deal: "50% off, up to Rp25.000", ValidUntil: "2026-11-30"}
	c := models.Promo{BrandID: "brand_x", Deal: "Gratis es teh", ValidUntil: "2026-11-30"}
	if dedupeKey(a) != dedupeKey(b) {
		t.Fatalf("same numbers, same promo: %s vs %s", dedupeKey(a), dedupeKey(b))
	}
	if dedupeKey(c) != "brand_x|gratisesteh||2026-11-30|" {
		t.Fatalf("no numbers falls back to the compact deal: %s", dedupeKey(c))
	}
}

func TestMatchOutletsIgnoresBareBrandNames(t *testing.T) {
	outlets := []models.Outlet{
		{ID: "out_osm_n1", Name: "Mie Contoh", Address: "Jl. Contoh 1, Jakarta Selatan"},
		{ID: "out_osm_n2", Name: "Mie Contoh Mal Contoh Raya", Address: "Jl. Contoh 2, Depok"},
		{ID: "out_osm_n3", Name: "Mie Contoh", MallName: "Mal Contoh Raya", Address: "Jl. Contoh 3, Depok"},
		{ID: "out_osm_n4", Name: "Mie Contoh", MallName: "ÆON Mall", Address: "Jl. Contoh 4, Tangerang"},
	}
	// "Pakuwon Mall" holds the letters of "ÆON Mall" (compacted "onmall"), not its words
	if ids, ok := matchOutlets([]models.OutletLimit{{Kind: "mall", Name: "Pakuwon Mall"}}, outlets); ok || len(ids) != 0 {
		t.Fatalf("matched a mall by letters: %v %v", ids, ok)
	}
	if ids, _ := matchOutlets([]models.OutletLimit{{Kind: "mall", Name: "Mie Contoh ÆON Mall"}}, outlets); strings.Join(ids, ",") != "out_osm_n4" {
		t.Fatalf("mall named inside the limit: %v", ids)
	}
	// a place outside the outlets' area, named with the brand: an outlet called just "Mie Contoh" is no match
	if ids, ok := matchOutlets([]models.OutletLimit{{Kind: "outlet", Name: "Mie Contoh Sulawesi Selatan"}}, outlets); ok || len(ids) != 0 {
		t.Fatalf("bare brand name matched: %v %v", ids, ok)
	}
	ids, ok := matchOutlets([]models.OutletLimit{{Kind: "mall", Name: "Mie Contoh Mal Contoh Raya"}}, outlets)
	if !ok || strings.Join(ids, ",") != "out_osm_n2,out_osm_n3" {
		t.Fatalf("mall by outlet name or mall_name: %v %v", ids, ok)
	}
}
