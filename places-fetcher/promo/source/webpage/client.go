package webpage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"places-fetcher/config"
)

// fetcher gets one page's body; httpFetcher calls the web, fileFetcher replays saved pages in tests.
type fetcher interface {
	Get(ctx context.Context, pageURL string) ([]byte, error)
}

const maxBody = 5 << 20 // 5 MB; a promo page is far smaller

type httpFetcher struct {
	client    *http.Client
	userAgent string
	retries   int
	backoff   time.Duration
	hostDelay time.Duration

	mu     sync.Mutex
	last   map[string]time.Time // host → when its last request started
	robots map[string]*robots   // host → its robots.txt rules
}

func newHTTPFetcher(cfg config.WebPageConfig, userAgent string) *httpFetcher {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &httpFetcher{
		client:    &http.Client{Timeout: timeout},
		userAgent: userAgent,
		retries:   cfg.Retries,
		backoff:   2 * time.Second,
		hostDelay: time.Duration(cfg.HostDelayMs) * time.Millisecond,
		last:      map[string]time.Time{},
		robots:    map[string]*robots{},
	}
}

// Get waits its turn for the host, checks robots.txt, and retries 429s, 5xx and network errors
// with a growing pause. Any other non-200 answer is an error right away.
func (f *httpFetcher) Get(ctx context.Context, pageURL string) ([]byte, error) {
	u, err := url.Parse(pageURL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("bad url %q", pageURL)
	}
	rules, err := f.robotsFor(ctx, u)
	if err != nil {
		return nil, err
	}
	if !rules.allowed(u.RequestURI()) {
		return nil, fmt.Errorf("robots.txt of %s disallows %s", u.Host, u.Path)
	}

	var lastErr error
	for attempt := 0; attempt <= f.retries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, f.backoff*time.Duration(attempt)); err != nil {
				return nil, err
			}
		}
		body, status, err := f.get(ctx, u)
		switch {
		case err != nil:
			lastErr = err
		case status == http.StatusOK:
			return body, nil
		case status == http.StatusTooManyRequests || status >= 500:
			lastErr = fmt.Errorf("HTTP %d from %s", status, pageURL)
		default:
			return nil, fmt.Errorf("HTTP %d from %s", status, pageURL)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

func (f *httpFetcher) get(ctx context.Context, u *url.URL) ([]byte, int, error) {
	if err := f.wait(ctx, u.Host); err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	return body, resp.StatusCode, err
}

// wait keeps hostDelay between the starts of two requests to the same host.
func (f *httpFetcher) wait(ctx context.Context, host string) error {
	f.mu.Lock()
	next := f.last[host].Add(f.hostDelay)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	f.last[host] = next
	f.mu.Unlock()
	return sleep(ctx, time.Until(next))
}

// robotsFor reads a host's robots.txt once per run. A missing or unreadable file allows everything;
// a network error is returned, since the page itself would likely fail too.
func (f *httpFetcher) robotsFor(ctx context.Context, u *url.URL) (*robots, error) {
	f.mu.Lock()
	r, ok := f.robots[u.Host]
	f.mu.Unlock()
	if ok {
		return r, nil
	}
	robotsURL := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/robots.txt"}
	body, status, err := f.get(ctx, robotsURL)
	if err != nil {
		return nil, fmt.Errorf("robots.txt of %s: %w", u.Host, err)
	}
	r = &robots{}
	if status == http.StatusOK {
		r = parseRobots(string(body), f.userAgent)
	}
	f.mu.Lock()
	f.robots[u.Host] = r
	f.mu.Unlock()
	return r, nil
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// fileFetcher serves saved pages: pages maps a URL to a file. An unknown URL is a 404-like error.
type fileFetcher struct {
	pages map[string]string
	got   []string // URLs asked for, in order
}

func (f *fileFetcher) Get(ctx context.Context, pageURL string) ([]byte, error) {
	f.got = append(f.got, pageURL)
	path, ok := f.pages[pageURL]
	if !ok {
		return nil, fmt.Errorf("HTTP 404 from %s", pageURL)
	}
	return os.ReadFile(path)
}
