package webpage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/promo/source"
)

func fetch(t *testing.T, pages map[string]string, src models.Source, known map[string]bool, maxNew int) (models.FetchResult, error, *fileFetcher) {
	t.Helper()
	f := &fileFetcher{pages: pages}
	res, err := newAdapter(f, maxNew).FetchItems(context.Background(), models.FetchRequest{Source: src, KnownRefs: known})
	return res, err, f
}

var katalogSrc = models.Source{ID: "src_mie_katalog", Type: Name, Target: "https://katalogpromosi.com/restoran-dan-cafe/mie-contoh/"}

func katalogPages(t *testing.T) map[string]string {
	return map[string]string{
		katalogSrc.Target: fx("katalog_listing.html"),
		"https://katalogpromosi.com/promo-mie-contoh-es-krim/":       fx("katalog_post_1001.html"),
		"https://katalogpromosi.com/promo-mie-contoh-paket-pelajar/": fx("katalog_post_1002.html"),
		"https://katalogpromosi.com/promo-toko-lain/":                fx("generic.html"),
	}
}

func TestKatalogReadsTheBrandsPostsOnly(t *testing.T) {
	res, err, f := fetch(t, katalogPages(t), katalogSrc, nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("want the brand's 2 posts (deduped, sidebar left out), got %d: %+v", len(res.Items), res.Items)
	}
	post := res.Items[0]
	if post.Ref != "1001" || post.URL != "https://katalogpromosi.com/promo-mie-contoh-es-krim/" ||
		post.PostedAt != "2026-10-29T12:00:00+07:00" {
		t.Fatalf("ref, url and posted_at: %+v", post)
	}
	want := "Promo Mie Contoh Es Krim Rp5.000\nEs krim tiga rasa Rp5.000 di dua outlet.\nLokasi\n- Outlet Satu\n- Outlet Dua\nFAQ\nQ: Berlaku kelipatan?\nA: Ya."
	if post.Text != want {
		t.Fatalf("text without share buttons, related posts or banners:\n%s\nwant:\n%s", post.Text, want)
	}
	for _, u := range f.got {
		if strings.Contains(u, "toko-lain") {
			t.Fatal("a sidebar post was opened")
		}
	}
}

func TestKatalogSkipsKnownPostsWithoutOpeningThem(t *testing.T) {
	res, err, f := fetch(t, katalogPages(t), katalogSrc, map[string]bool{"1001": true}, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Ref != "1002" {
		t.Fatalf("only the new post: %+v", res.Items)
	}
	if len(f.got) != 2 {
		t.Fatalf("listing + 1 new post, got requests %v", f.got)
	}
}

func TestKatalogMaxNewItems(t *testing.T) {
	res, err, _ := fetch(t, katalogPages(t), katalogSrc, nil, 1)
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("maxNew 1 opens one post: %+v, %v", res.Items, err)
	}
}

func TestKatalogLayoutChecks(t *testing.T) {
	pages := map[string]string{katalogSrc.Target: fx("generic.html")}
	if _, err, _ := fetch(t, pages, katalogSrc, nil, 25); err == nil || !strings.Contains(err.Error(), "#main-content") {
		t.Fatalf("a page without #main-content is a layout change: %v", err)
	}
	other := katalogSrc
	other.Target = "https://katalogpromosi.com/restoran-dan-cafe/kedai-lain/"
	pages = map[string]string{other.Target: fx("katalog_listing.html")}
	if _, err, _ := fetch(t, pages, other, nil, 25); err == nil || !strings.Contains(err.Error(), `no posts in category "kedai-lain"`) {
		t.Fatalf("a category with no posts is an error: %v", err)
	}
}

var gilaSrc = models.Source{ID: "src_mie_giladiskon", Type: Name, Target: "https://www.giladiskon.com/merchant/mie-contoh"}

func TestGiladiskonSkipsAFailedDealPage(t *testing.T) {
	pages := map[string]string{
		gilaSrc.Target: fx("giladiskon_merchant.html"),
		"https://www.giladiskon.com/deals/paket-hemat-berdua": fx("giladiskon_deal.html"),
		// promo-sudah-hilang is missing: a 404
	}
	res, err, f := fetch(t, pages, gilaSrc, nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("one deal read, the gone one skipped: %+v", res.Items)
	}
	it := res.Items[0]
	if it.Ref != "paket-hemat-berdua" || it.URL != "https://www.giladiskon.com/deals/paket-hemat-berdua" {
		t.Fatalf("ref and url: %+v", it)
	}
	want := "Paket Hemat Berdua Rp40.000\nBerlaku sampai: 2026-11-30\nSenin–Jumat, 1 sampai 30 November 2026, di semua outlet.\nSyarat:\n- Makan di tempat.\n- Tidak bisa digabung."
	if it.Text != want {
		t.Fatalf("text:\n%s\nwant:\n%s", it.Text, want)
	}
	for _, u := range f.got {
		if strings.Contains(u, "link-menu") {
			t.Fatal("a link outside #deal-feed was opened")
		}
	}
}

func TestGiladiskonAllDealPagesFailing(t *testing.T) {
	pages := map[string]string{gilaSrc.Target: fx("giladiskon_merchant.html")}
	if _, err, _ := fetch(t, pages, gilaSrc, nil, 25); err == nil || !strings.Contains(err.Error(), "every new item page failed") {
		t.Fatalf("every page failing fails the source: %v", err)
	}
	known := map[string]bool{"paket-hemat-berdua": true, "promo-sudah-hilang": true}
	res, err, _ := fetch(t, pages, gilaSrc, known, 25)
	if err != nil || len(res.Items) != 0 {
		t.Fatalf("nothing new is a quiet success: %+v, %v", res.Items, err)
	}
	pages = map[string]string{gilaSrc.Target: fx("generic.html")}
	if _, err, _ := fetch(t, pages, gilaSrc, nil, 25); err == nil || !strings.Contains(err.Error(), "#deal-feed") {
		t.Fatalf("a page without #deal-feed is a layout change: %v", err)
	}
}

// BCA pages are Next.js streams; these are built from script chunks so lengths are exact.

func nextPage(t *testing.T, chunks ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><title>Promo</title></head><body>")
	for _, c := range chunks {
		q, _ := json.Marshal(c)
		fmt.Fprintf(&b, `<script>self.__next_f.push([1,%s])</script>`, q)
	}
	b.WriteString("</body></html>")
	return page(t, b.String())
}

func card(id, brand, title, link, start, end string) string {
	return fmt.Sprintf(`{"versionPage":1,"slug":"x","linkPage":%q,"idPromo":%q,"brandPromo":%q,"titlePromo":%q,`+
		`"summaryPromo":%q,"publishDate":"2026-10-30 10:51:03.256","startDate":%q,"endDate":%q}`, link, id, brand, title, title, start, end)
}

var bcaSrc = models.Source{ID: "src_bca_fnb", Type: Name, Target: "https://promo.bca.co.id/id/all/c/fnb", IsAggregator: true}

func TestBCAReadsCardsAndTerms(t *testing.T) {
	mie := card("AAAA0001", "Mie Contoh", "Diskon 20%", "all/2026/10/30/1/mie-contoh", "2026-11-02 00:01:00.000", "2026-11-25 23:59:00.000")
	kedai := card("AAAA0002", "Kedai Contoh", "Diskon 15%", "all/2026/10/30/2/kedai-contoh", "", "2026-11-27 23:59:00.000")
	listing := `0:["$","div",null,{"cards":[` + mie + `,` + kedai + `]}]`
	split := len(listing) / 2 // a card cut across two script chunks

	terms := `<div class="promo"><p><strong>Diskon 20%</strong></p><ul><li>Bayar pakai QRIS, Senin - Rabu</li><li>Periode promo: 2 - 25 November 2026</li></ul></div>`
	miePage := nextPage(t,
		`1:{"description":"Teks kategori tanpa markup"}`+"\n",
		`2:["$","$L29",null,{"description":"$2a","brand":"Mie Contoh"}]`+"\n",
		fmt.Sprintf("2a:T%x,%s", len(terms), terms))
	kedaiPage := nextPage(t, `3:["$","$L2a",null,{"description":"<div class=\"promo\"><p>Diskon 15% di semua outlet</p></div>"}]`)

	pages := map[string]string{
		bcaSrc.Target: nextPage(t, listing[:split], listing[split:]),
		"https://promo.bca.co.id/id/all/2026/10/30/1/mie-contoh":   miePage,
		"https://promo.bca.co.id/id/all/2026/10/30/2/kedai-contoh": kedaiPage,
	}
	res, err, _ := fetch(t, pages, bcaSrc, nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("want 2 cards, got %+v", res.Items)
	}
	m := res.Items[0]
	if m.Ref != "AAAA0001" || m.URL != "https://promo.bca.co.id/id/all/2026/10/30/1/mie-contoh" || m.PostedAt != "2026-10-30T10:51:03+07:00" {
		t.Fatalf("ref, url and posted_at: %+v", m)
	}
	want := "Brand: Mie Contoh\nPromo: Diskon 20%\nPeriode: 2026-11-02 sampai 2026-11-25\nDiskon 20%\n- Bayar pakai QRIS, Senin - Rabu\n- Periode promo: 2 - 25 November 2026"
	if m.Text != want {
		t.Fatalf("text:\n%s\nwant:\n%s", m.Text, want)
	}
	if !strings.Contains(res.Items[1].Text, "Periode:  sampai 2026-11-27") || !strings.Contains(res.Items[1].Text, "Diskon 15% di semua outlet") {
		t.Fatalf("an inline description and a missing start date: %s", res.Items[1].Text)
	}
}

func TestBCAFailsWithoutCards(t *testing.T) {
	pages := map[string]string{bcaSrc.Target: nextPage(t, `0:["$","div",null,{"children":"no promos"}]`)}
	if _, err, _ := fetch(t, pages, bcaSrc, nil, 25); err == nil || !strings.Contains(err.Error(), "no promo cards") {
		t.Fatalf("a listing with no cards is a layout change: %v", err)
	}
	pages = map[string]string{bcaSrc.Target: fx("generic.html")}
	if _, err, _ := fetch(t, pages, bcaSrc, nil, 25); err == nil || !strings.Contains(err.Error(), "no self.__next_f data") {
		t.Fatalf("a page that isn't a Next.js stream: %v", err)
	}
}

func TestGenericPageIsOneItemByHash(t *testing.T) {
	src := models.Source{ID: "src_dompet", Type: Name, Target: "https://dompet.example/promo/kuliner"}
	pages := map[string]string{src.Target: fx("generic.html")}
	res, err, _ := fetch(t, pages, src, nil, 25)
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("one item: %+v, %v", res.Items, err)
	}
	it := res.Items[0]
	if it.Text != "Promo Dompet Kuliner November\nCashback 20% di merchant kuliner\nBerlaku 1 - 30 November 2026, maksimal Rp20.000." {
		t.Fatalf("text from <main> only:\n%s", it.Text)
	}
	if len(it.Ref) != 16 {
		t.Fatalf("ref is a 16-char hash: %q", it.Ref)
	}
	again, _, _ := fetch(t, pages, src, map[string]bool{it.Ref: true}, 25)
	if len(again.Items) != 0 {
		t.Fatal("an unchanged page is skipped")
	}
}

func TestQponAndBadTargets(t *testing.T) {
	_, err, f := fetch(t, nil, models.Source{ID: "src_mie_qpon", Target: "https://qpon.id/"}, nil, 25)
	if err == nil || !strings.Contains(err.Error(), "headless rendering") || len(f.got) != 0 {
		t.Fatalf("qpon refuses without fetching: %v %v", err, f.got)
	}
	for _, target := range []string{"", "a_handle", "ftp://example.com/x"} {
		if _, err, _ := fetch(t, nil, models.Source{ID: "src_x", Target: target}, nil, 25); err == nil {
			t.Errorf("target %q must be rejected", target)
		}
	}
}

func TestRegistered(t *testing.T) {
	cfg := config.MainConfig{Promo: config.PromoConfig{UserAgent: "makandimana-promo-fetcher/0.1"}}
	set, err := source.NewSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := set.ForType(Name); !ok || a.Type() != Name {
		t.Fatal("web_page is registered")
	}
	if _, err := source.NewSet(config.MainConfig{}); err == nil {
		t.Fatal("an empty Promo.UserAgent fails at startup")
	}
}
