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

var now = time.Date(2026, 11, 2, 6, 0, 0, 0, models.WIB)

func at(d time.Duration) string { return now.Add(-d).Format(models.TimeLayout) }

func TestIsDue(t *testing.T) {
	cases := []struct {
		name string
		src  models.Source
		want bool
	}{
		{"never fetched", models.Source{Status: "active", FetchEveryHours: 24}, true},
		{"fetched 25h ago", models.Source{Status: "active", FetchEveryHours: 24, LastFetchedAt: at(25 * time.Hour)}, true},
		{"daily run a few minutes early", models.Source{Status: "active", FetchEveryHours: 24, LastFetchedAt: at(23*time.Hour + 55*time.Minute)}, true},
		{"22h ago is the edge", models.Source{Status: "active", FetchEveryHours: 24, LastFetchedAt: at(22 * time.Hour)}, true},
		{"fetched 3h ago", models.Source{Status: "active", FetchEveryHours: 24, LastFetchedAt: at(3 * time.Hour)}, false},
		{"weekly, fetched 2 days ago", models.Source{Status: "active", FetchEveryHours: 168, LastFetchedAt: at(48 * time.Hour)}, false},
		{"6-hourly, 5h30m ago", models.Source{Status: "active", FetchEveryHours: 6, LastFetchedAt: at(5*time.Hour + 30*time.Minute)}, true},
		{"6-hourly, 5h ago", models.Source{Status: "active", FetchEveryHours: 6, LastFetchedAt: at(5 * time.Hour)}, false},
		{"empty interval means daily", models.Source{Status: "active", LastFetchedAt: at(3 * time.Hour)}, false},
		{"unreadable timestamp", models.Source{Status: "active", FetchEveryHours: 24, LastFetchedAt: "yesterday"}, true},
		{"paused", models.Source{Status: "paused", FetchEveryHours: 24}, false},
		{"error", models.Source{Status: "error", FetchEveryHours: 24}, false},
	}
	for _, c := range cases {
		if got := IsDue(c.src, now); got != c.want {
			t.Errorf("%s: IsDue = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDueSourcesWithSourceID(t *testing.T) {
	srcs := []models.Source{
		{ID: "src_a", Status: "active", FetchEveryHours: 24, LastFetchedAt: at(time.Hour)},
		{ID: "src_b", Status: "paused"},
	}
	got, err := DueSources(srcs, "src_b", now)
	if err != nil || len(got) != 1 || got[0].ID != "src_b" {
		t.Fatalf("a named source runs even when paused: %+v, %v", got, err)
	}
	if _, err := DueSources(srcs, "src_missing", now); err == nil {
		t.Fatal("an unknown source id must be an error")
	}
	if got, _ := DueSources(srcs, "", now); len(got) != 0 {
		t.Fatalf("nothing is due: %+v", got)
	}
}

// fakes for the ports

type memSources []models.Source

func (m memSources) GetSources(ctx context.Context) ([]models.Source, error) { return m, nil }

type memStatuses struct {
	got []models.SourceStatus
	err error
}

func (m *memStatuses) UpdateSources(ctx context.Context, s []models.SourceStatus) (models.WriteResult, error) {
	m.got = s
	return models.WriteResult{Updated: len(s)}, m.err
}

type memRaw struct {
	got []models.RawItem
	err error
}

func (m *memRaw) UpsertRawItems(ctx context.Context, items []models.RawItem) (models.WriteResult, error) {
	m.got = items
	return models.WriteResult{Added: len(items)}, m.err
}

type fakeAdapter struct {
	results map[string]models.FetchResult
	errs    map[string]error
}

func (f fakeAdapter) Type() string { return "web_page" }
func (f fakeAdapter) FetchItems(ctx context.Context, src models.Source) (models.FetchResult, error) {
	return f.results[src.ID], f.errs[src.ID]
}

type adapters map[string]promo.SourceAdapter

func (a adapters) ForType(t string) (promo.SourceAdapter, bool) { s, ok := a[t]; return s, ok }

func run(t *testing.T, srcs memSources, a fakeAdapter, sourceID string) (models.PromoRunResult, error, *memStatuses, *memRaw) {
	t.Helper()
	st, raw := &memStatuses{}, &memRaw{}
	uc := NewPromoUsecase(srcs, st, adapters{"web_page": a}, raw, 3, time.Minute)
	res, err := uc.RunSources(context.Background(), promo.InputRun{SourceID: sourceID, Now: now})
	return res, err, st, raw
}

func TestRunSourcesStoresItemsAndClearsErrors(t *testing.T) {
	srcs := memSources{
		{ID: "src_ramenya_katalog", Type: "web_page", Status: "active", FetchEveryHours: 24, ErrorCount: 2, LastError: "old"},
		{ID: "src_later", Type: "web_page", Status: "active", FetchEveryHours: 24, LastFetchedAt: at(time.Hour)},
	}
	a := fakeAdapter{results: map[string]models.FetchResult{"src_ramenya_katalog": {
		Items:       []models.RawItem{{Ref: "691830", URL: "u1"}, {Ref: "592315", URL: "u2"}, {Ref: "691830", URL: "dup"}},
		LastSeenRef: "691830",
	}}}
	res, err, st, raw := run(t, srcs, a, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Sources) != 1 || res.Sources[0].SourceID != "src_ramenya_katalog" || res.Fetched != 2 {
		t.Fatalf("only the due source runs, duplicates dropped: %+v", res)
	}
	if len(raw.got) != 2 || raw.got[0].ID != "src_ramenya_katalog:691830" || raw.got[0].SourceID != "src_ramenya_katalog" {
		t.Fatalf("raw items get id and source_id: %+v", raw.got)
	}
	if raw.got[0].Status != "" {
		t.Fatal("the fetch stage never sets a raw item status")
	}
	want := models.SourceStatus{ID: "src_ramenya_katalog", LastFetchedAt: "2026-11-02T06:00:00+07:00", LastSeenRef: "691830"}
	if len(st.got) != 1 || st.got[0] != want {
		t.Fatalf("success clears error_count and last_error: %+v", st.got)
	}
}

func TestRunSourcesOneFailureDoesNotStopOthers(t *testing.T) {
	srcs := memSources{
		{ID: "src_bad", Type: "web_page", Status: "active", ErrorCount: 0},
		{ID: "src_ok", Type: "web_page", Status: "active"},
		{ID: "src_unknown", Type: "instagram", Status: "active"},
	}
	a := fakeAdapter{
		results: map[string]models.FetchResult{"src_ok": {Items: []models.RawItem{{Ref: "1"}}}},
		errs:    map[string]error{"src_bad": errors.New("HTTP 503")},
	}
	res, err, st, _ := run(t, srcs, a, "")
	if err != nil {
		t.Fatalf("a partial failure is not a run failure: %v", err)
	}
	if res.Failed() != 2 {
		t.Fatalf("want 2 failed, got %+v", res.Sources)
	}
	byID := map[string]models.SourceStatus{}
	for _, s := range st.got {
		byID[s.ID] = s
	}
	if byID["src_bad"].ErrorCount != 1 || byID["src_bad"].LastError != "HTTP 503" || byID["src_bad"].LastFetchedAt != "" {
		t.Fatalf("failure bumps error_count and keeps last_fetched_at: %+v", byID["src_bad"])
	}
	if !strings.Contains(byID["src_unknown"].LastError, `unknown source type "instagram"`) {
		t.Fatalf("unknown type: %+v", byID["src_unknown"])
	}
	if byID["src_ok"].LastFetchedAt == "" {
		t.Fatal("the healthy source still updates")
	}
}

func TestRunSourcesDisablesAfterMaxErrors(t *testing.T) {
	srcs := memSources{{ID: "src_bad", Type: "web_page", Status: "active", ErrorCount: 2}}
	a := fakeAdapter{errs: map[string]error{"src_bad": errors.New("timeout")}}
	res, err, st, _ := run(t, srcs, a, "")
	if err == nil || !strings.Contains(err.Error(), "all 1 due sources failed") {
		t.Fatalf("every due source failed must fail the run: %v", err)
	}
	if st.got[0].ErrorCount != 3 || st.got[0].Status != "error" || !res.Sources[0].Disabled {
		t.Fatalf("third failure in a row moves it to error: %+v", st.got[0])
	}
}

func TestRunSourcesPausedPreviewNeverDisables(t *testing.T) {
	srcs := memSources{{ID: "src_paused", Type: "web_page", Status: "paused", ErrorCount: 5}}
	a := fakeAdapter{errs: map[string]error{"src_paused": errors.New("timeout")}}
	_, _, st, _ := run(t, srcs, a, "src_paused")
	if st.got[0].Status != "" {
		t.Fatalf("only an active source can move to error: %+v", st.got[0])
	}
}

func TestRunSourcesRejectsItemWithoutRef(t *testing.T) {
	srcs := memSources{{ID: "src_a", Type: "web_page", Status: "active"}}
	a := fakeAdapter{results: map[string]models.FetchResult{"src_a": {Items: []models.RawItem{{URL: "u"}}}}}
	res, _, _, raw := run(t, srcs, a, "")
	if res.Failed() != 1 || len(raw.got) != 0 || !strings.Contains(res.Sources[0].Err, "without a ref") {
		t.Fatalf("an item without a ref fails its source: %+v", res)
	}
}

func TestRunSourcesSheetWriteFailureLeavesStatusesAlone(t *testing.T) {
	srcs := memSources{{ID: "src_a", Type: "web_page", Status: "active"}}
	a := fakeAdapter{results: map[string]models.FetchResult{"src_a": {Items: []models.RawItem{{Ref: "1"}}}}}
	st, raw := &memStatuses{}, &memRaw{err: errors.New("sheet: busy, retry later")}
	uc := NewPromoUsecase(srcs, st, adapters{"web_page": a}, raw, 3, time.Minute)
	_, err := uc.RunSources(context.Background(), promo.InputRun{Now: now})
	if err == nil || !strings.Contains(err.Error(), "UpsertRawItems") {
		t.Fatalf("want the raw items error, got %v", err)
	}
	if st.got != nil {
		t.Fatal("statuses must not advance when raw items weren't stored")
	}
}

func TestRunSourcesNothingDue(t *testing.T) {
	srcs := memSources{{ID: "src_a", Type: "web_page", Status: "paused"}}
	res, err, st, _ := run(t, srcs, fakeAdapter{}, "")
	if err != nil || len(res.Sources) != 0 || st.got != nil {
		t.Fatalf("nothing due is a quiet success: %+v %v", res, err)
	}
}

func TestRunSourcesValidatesInput(t *testing.T) {
	uc := NewPromoUsecase(memSources{}, &memStatuses{}, adapters{}, &memRaw{}, 3, time.Minute)
	if _, err := uc.RunSources(context.Background(), promo.InputRun{}); err == nil {
		t.Fatal("a zero clock must be rejected")
	}
	if _, err := uc.RunSources(context.Background(), promo.InputRun{Now: now, SourceID: "bca"}); err == nil {
		t.Fatal("a malformed source id must be rejected")
	}
}
