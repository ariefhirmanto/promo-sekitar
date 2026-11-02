package repository

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"places-fetcher/config"
	"places-fetcher/models"
)

// fakeSheet records each call and answers with the canned JSON for its mode.
type fakeSheet struct {
	answers map[string]string
	calls   []call
}

type call struct {
	method, mode, status string
	body                 map[string]json.RawMessage
}

func (f *fakeSheet) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := call{method: r.Method, mode: r.URL.Query().Get("mode"), status: r.URL.Query().Get("status")}
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			json.Unmarshal(b, &c.body)
		}
		f.calls = append(f.calls, c)
		w.Write([]byte(f.answers[c.mode]))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newRepo(srv *httptest.Server) *sheetRepository {
	return NewSheetRepository(config.SheetConfig{URL: srv.URL, Token: "tok", TimeoutSeconds: 5})
}

func TestGetSourcesDecodesCodeGsShape(t *testing.T) {
	f := &fakeSheet{answers: map[string]string{"sources": `{"ok":true,"sources":[{"id":"src_bca_fnb","type":"web_page",
		"target":"https://promo.bca.co.id/id/all/c/fnb","brand_id":"","is_aggregator":true,"fetch_every_hours":24,
		"status":"active","last_fetched_at":"2026-11-01T06:00:00+07:00","last_seen_ref":"","error_count":1,"last_error":"x","notes":""}]}`}}
	got, err := newRepo(f.server(t)).GetSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []models.Source{{ID: "src_bca_fnb", Type: "web_page", Target: "https://promo.bca.co.id/id/all/c/fnb",
		IsAggregator: true, FetchEveryHours: 24, Status: "active", LastFetchedAt: "2026-11-01T06:00:00+07:00",
		ErrorCount: 1, LastError: "x"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestUpdateSourcesSendsClearingFields(t *testing.T) {
	f := &fakeSheet{answers: map[string]string{"update_sources": `{"ok":true,"updated":1,"skipped":0}`}}
	res, err := newRepo(f.server(t)).UpdateSources(context.Background(), []models.SourceStatus{{ID: "src_a", LastFetchedAt: "t"}})
	if err != nil || res.Updated != 1 {
		t.Fatalf("res %+v err %v", res, err)
	}
	body := string(f.calls[0].body["sources"])
	for _, want := range []string{`"error_count":0`, `"last_error":""`, `"last_fetched_at":"t"`} {
		if !strings.Contains(body, want) {
			t.Errorf("payload %s lacks %s", body, want)
		}
	}
	if strings.Contains(body, `"status"`) {
		t.Errorf("an empty status must not be sent: %s", body)
	}
}

func TestUpsertRawItemsOmitsParseFields(t *testing.T) {
	f := &fakeSheet{answers: map[string]string{"upsert_raw_items": `{"ok":true,"added":1,"updated":0,"pruned":2}`}}
	res, err := newRepo(f.server(t)).UpsertRawItems(context.Background(), []models.RawItem{
		{ID: "src_a:1", SourceID: "src_a", Ref: "1", URL: "u", Text: "t"},
	})
	if err != nil || res.Added != 1 || res.Pruned != 2 {
		t.Fatalf("res %+v err %v", res, err)
	}
	body := string(f.calls[0].body["raw_items"])
	for _, field := range []string{"status", "attempts", "error", "promo_ids", "fetched_at"} {
		if strings.Contains(body, `"`+field+`"`) {
			t.Errorf("fetch-stage payload must not send %s: %s", field, body)
		}
	}
}

func TestGetRawItemsSplitsPromoIDs(t *testing.T) {
	f := &fakeSheet{answers: map[string]string{"raw_items": `{"ok":true,"raw_items":[{"id":"src_a:1","source_id":"src_a",
		"ref":"1","url":"u","posted_at":"","text":"t","status":"failed","attempts":2,"error":"timeout",
		"promo_ids":"promo_a, promo_b","fetched_at":"2026-11-01T06:00:00+07:00"}]}`}}
	got, err := newRepo(f.server(t)).GetRawItems(context.Background(), "failed")
	if err != nil {
		t.Fatal(err)
	}
	if f.calls[0].status != "failed" {
		t.Fatalf("status param %q", f.calls[0].status)
	}
	if len(got) != 1 || got[0].Attempts != 2 || !reflect.DeepEqual(got[0].PromoIDs, []string{"promo_a", "promo_b"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestGetRawItemRefs(t *testing.T) {
	f := &fakeSheet{answers: map[string]string{"raw_item_refs": `{"ok":true,"refs":{"src_a":["1","2"]}}`}}
	got, err := newRepo(f.server(t)).GetRawItemRefs(context.Background())
	if err != nil || !reflect.DeepEqual(got, map[string][]string{"src_a": {"1", "2"}}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestUpsertPromosAndSheetErrors(t *testing.T) {
	f := &fakeSheet{answers: map[string]string{"upsert_promos": `{"ok":false,"error":"promo id must look like promo_<a-z0-9_>: x"}`}}
	_, err := newRepo(f.server(t)).UpsertPromos(context.Background(), []models.Promo{{ID: "x"}})
	if err == nil || !strings.Contains(err.Error(), "promo id must look like") {
		t.Fatalf("want the sheet's error, got %v", err)
	}

	_, err = NewSheetRepository(config.SheetConfig{}).GetSources(context.Background())
	if err == nil || !strings.Contains(err.Error(), "-dry-run with -sources-file") {
		t.Fatalf("want the dry-run hint, got %v", err)
	}
}
