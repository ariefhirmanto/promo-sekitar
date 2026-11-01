package overpass

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"places-fetcher/config"

	"github.com/pkg/errors"
)

// client returns the raw Overpass JSON for a query.
type client interface {
	Fetch(ctx context.Context, query string) ([]byte, error)
}

type httpClient struct {
	endpoints []string
	userAgent string
	retries   int
	retryWait time.Duration
	http      *http.Client
}

func newHTTPClient(cfg config.OverpassConfig) *httpClient {
	return &httpClient{
		endpoints: cfg.Endpoints,
		userAgent: cfg.UserAgent,
		retries:   cfg.Retries,
		retryWait: time.Duration(cfg.RetryWaitSeconds) * time.Second,
		http:      &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

// Fetch tries every endpoint, then waits and repeats; public servers are often at capacity.
func (c *httpClient) Fetch(ctx context.Context, query string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < c.retries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * c.retryWait
			log.Printf("[Outlet][Fetch][Overpass] All servers failed, retrying in %s", wait)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}

		for _, ep := range c.endpoints {
			body, err := c.post(ctx, ep, query)
			if err == nil {
				log.Printf("[Outlet][Fetch][Overpass] %d bytes from %s", len(body), ep)
				return body, nil
			}
			log.Printf("[Outlet][Fetch][Overpass] %s: %v", ep, err)
			lastErr = err
		}
	}

	return nil, errors.Wrap(lastErr, "overpass.httpClient.Fetch.allEndpointsFailed")
}

func (c *httpClient) post(ctx context.Context, endpoint, query string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(url.Values{"data": {query}}.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Overpass rejects requests without a descriptive User-Agent (HTTP 406).
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || len(body) == 0 || body[0] != '{' {
		return nil, fmt.Errorf("HTTP %d: %.120s", resp.StatusCode, body)
	}

	return body, nil
}

// fileClient replays a response saved earlier (-places-file), for testing without the network.
type fileClient struct {
	path string
}

func (c fileClient) Fetch(ctx context.Context, query string) ([]byte, error) {
	return os.ReadFile(c.path)
}
