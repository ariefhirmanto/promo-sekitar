// Package sheet is the HTTP client for the Apps Script web app bound to the makandimana
// spreadsheet (apps-script/Code.gs). Both fetchers' repositories use it; it knows the call shape
// (token + mode in the query, JSON in and out, the {ok, error} envelope), not any tab.
package sheet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"places-fetcher/config"

	"github.com/pkg/errors"
)

// ErrNotConfigured means SHEET_URL or SHEET_TOKEN is empty.
var ErrNotConfigured = errors.New("SHEET_URL and SHEET_TOKEN must be set")

type Client struct {
	url    string
	token  string
	client *http.Client
}

func NewClient(cfg config.SheetConfig) *Client {
	return &Client{
		url:    cfg.URL,
		token:  cfg.Token,
		client: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

// Get calls mode with extra query params and decodes the answer into v.
func (c *Client) Get(ctx context.Context, mode string, params url.Values, v interface{}) error {
	u, err := c.endpoint(mode, params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return c.do(req, v)
}

// Post sends payload as JSON to mode and decodes the answer into v. Apps Script answers a POST
// with a redirect to the result; Go's client follows it as a GET, which is what Apps Script expects.
func (c *Client) Post(ctx context.Context, mode string, payload interface{}, v interface{}) error {
	u, err := c.endpoint(mode, nil)
	if err != nil {
		return err
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, v)
}

func (c *Client) endpoint(mode string, params url.Values) (string, error) {
	if c.url == "" || c.token == "" {
		return "", ErrNotConfigured
	}
	q := url.Values{"token": {c.token}, "mode": {mode}}
	for k, vs := range params {
		q[k] = vs
	}
	return c.url + "?" + q.Encode(), nil
}

// do decodes the {ok, error} envelope first, then the whole body into v.
func (c *Client) do(req *http.Request, v interface{}) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var env struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		// A non-JSON answer is usually the Google sign-in page: the web app isn't deployed
		// with "Who has access: Anyone".
		return fmt.Errorf("HTTP %d, not JSON: %.200s", resp.StatusCode, b)
	}
	if !env.OK {
		return fmt.Errorf("sheet: %s", env.Error)
	}
	return json.Unmarshal(b, v)
}
