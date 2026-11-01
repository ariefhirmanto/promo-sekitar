package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"places-fetcher/config"
	"places-fetcher/models"

	"github.com/pkg/errors"
)

// sheetRepository talks to the Apps Script web app bound to the makandimana spreadsheet (Code.gs).
type sheetRepository struct {
	url    string
	token  string
	client *http.Client
}

func NewSheetRepository(cfg config.SheetConfig) *sheetRepository {
	return &sheetRepository{
		url:    cfg.URL,
		token:  cfg.Token,
		client: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

func (r *sheetRepository) GetBrands(ctx context.Context) (resp []models.Brand, err error) {
	u, err := r.endpoint("brands")
	if err != nil {
		return resp, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return resp, err
	}

	var body struct {
		OK     bool           `json:"ok"`
		Error  string         `json:"error"`
		Brands []models.Brand `json:"brands"`
	}
	if err = r.do(req, &body); err != nil {
		log.Println("[Outlet][GetBrands][Repository] Problem reading brands from sheet, err: ", err.Error())
		return resp, errors.Wrap(err, "sheetRepository.GetBrands")
	}
	if !body.OK {
		return resp, fmt.Errorf("sheet: %s", body.Error)
	}

	return body.Brands, nil
}

// UpsertOutlets replaces this source's rows in the outlets tab. Apps Script answers a POST with
// a redirect to the result; Go's client follows it as a GET, which is what Apps Script expects.
func (r *sheetRepository) UpsertOutlets(ctx context.Context, source string, outlets []models.Outlet) (resp models.UpsertResult, err error) {
	u, err := r.endpoint("upsert_outlets")
	if err != nil {
		return resp, err
	}

	payload, err := json.Marshal(map[string]interface{}{"source": source, "outlets": outlets})
	if err != nil {
		return resp, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return resp, err
	}
	req.Header.Set("Content-Type", "application/json")

	var body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		models.UpsertResult
	}
	if err = r.do(req, &body); err != nil {
		log.Println("[Outlet][UpsertOutlets][Repository] Problem writing outlets to sheet, err: ", err.Error())
		return resp, errors.Wrap(err, "sheetRepository.UpsertOutlets")
	}
	if !body.OK {
		return resp, fmt.Errorf("sheet: %s", body.Error)
	}

	return body.UpsertResult, nil
}

func (r *sheetRepository) endpoint(mode string) (string, error) {
	if r.url == "" || r.token == "" {
		return "", errors.New("SHEET_URL and SHEET_TOKEN must be set (or use -dry-run with -brands-file)")
	}
	return r.url + "?" + url.Values{"token": {r.token}, "mode": {mode}}.Encode(), nil
}

func (r *sheetRepository) do(req *http.Request, v interface{}) error {
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		// A non-JSON answer is usually the Google sign-in page: the web app isn't deployed
		// with "Who has access: Anyone".
		return fmt.Errorf("HTTP %d, not JSON: %.200s", resp.StatusCode, b)
	}
	return nil
}
