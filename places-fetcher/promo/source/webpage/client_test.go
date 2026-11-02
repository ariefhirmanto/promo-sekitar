package webpage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"places-fetcher/config"
)

func testFetcher() *httpFetcher {
	f := newHTTPFetcher(config.WebPageConfig{TimeoutSeconds: 5, Retries: 2}, "makandimana-promo-fetcher/0.1")
	f.backoff = time.Millisecond
	return f
}

func TestGetRetriesServerErrorsAndSendsUserAgent(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		calls++
		if r.UserAgent() != "makandimana-promo-fetcher/0.1" {
			t.Errorf("User-Agent %q", r.UserAgent())
		}
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	body, err := testFetcher().Get(context.Background(), srv.URL+"/promo")
	if err != nil || string(body) != "ok" || calls != 3 {
		t.Fatalf("body %q err %v calls %d", body, err, calls)
	}
}

func TestGetGivesUpAndDoesNotRetry404(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		calls++
		if r.URL.Path == "/gone" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	f := testFetcher()
	if _, err := f.Get(context.Background(), srv.URL+"/gone"); err == nil || !strings.Contains(err.Error(), "HTTP 404") || calls != 1 {
		t.Fatalf("404 fails at once: %v, calls %d", err, calls)
	}
	calls = 0
	if _, err := f.Get(context.Background(), srv.URL+"/busy"); err == nil || !strings.Contains(err.Error(), "HTTP 502") || calls != 3 {
		t.Fatalf("1 try + 2 retries, then the last error: %v, calls %d", err, calls)
	}
}

func TestGetHonoursRobotsTxt(t *testing.T) {
	robotsCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			robotsCalls++
			w.Write([]byte("User-agent: *\nDisallow: /private\n"))
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	f := testFetcher()
	if _, err := f.Get(context.Background(), srv.URL+"/private/deal"); err == nil || !strings.Contains(err.Error(), "disallows") {
		t.Fatalf("want a robots refusal, got %v", err)
	}
	if _, err := f.Get(context.Background(), srv.URL+"/public"); err != nil {
		t.Fatal(err)
	}
	if robotsCalls != 1 {
		t.Fatalf("robots.txt is read once per host, got %d", robotsCalls)
	}
}

func TestHostDelaySpacesRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer srv.Close()
	f := testFetcher()
	f.hostDelay = 50 * time.Millisecond
	start := time.Now()
	for i := 0; i < 3; i++ { // robots.txt + 3 pages = 4 requests, 3 gaps
		if _, err := f.Get(context.Background(), srv.URL+"/p"); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("requests not spaced: %s", elapsed)
	}
}

func TestRobotsRules(t *testing.T) {
	// qpon.id's robots.txt in short: everything closed to "*" except a whitelist.
	qpon := parseRobots("User-agent: Googlebot\nAllow: /\n\nUser-agent: *\nDisallow: /\nAllow: /$\nAllow: /platform/brands\nAllow: /platform/couponDetail\n", "makandimana-promo-fetcher/0.1")
	cases := map[string]bool{
		"/":                          true,
		"/platform/brandsHomepage":   true,
		"/platform/couponDetail?x=1": true,
		"/platform/searchPage":       false,
		"/admin":                     false,
	}
	for path, want := range cases {
		if got := qpon.allowed(path); got != want {
			t.Errorf("qpon %s: allowed=%v, want %v", path, got, want)
		}
	}

	ours := parseRobots("User-agent: *\nDisallow: /\n\nUser-agent: makandimana-promo-fetcher\nDisallow: /admin\n", "makandimana-promo-fetcher/0.1")
	if !ours.allowed("/deals/x") || ours.allowed("/admin/x") {
		t.Fatal("the group naming our token wins over *")
	}
	if !parseRobots("User-agent: *\nDisallow:\n", "x/1").allowed("/anything") {
		t.Fatal("an empty Disallow allows everything")
	}
	if parseRobots("User-agent: *\nDisallow: /*.pdf$\n", "x/1").allowed("/promo/terms.pdf") {
		t.Fatal("wildcard and $ anchor")
	}
}
