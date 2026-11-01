package repository

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"places-fetcher/config"
	"places-fetcher/models"
)

// The outlet sheet repository after moving to the shared sheet client: same modes, payload and results.
func TestSheetRepositoryBrandsAndUpsert(t *testing.T) {
	var posted map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		switch {
		case r.Method == http.MethodPost && mode == "upsert_outlets":
			b, _ := io.ReadAll(r.Body)
			json.Unmarshal(b, &posted)
			w.Write([]byte(`{"ok":true,"added":1,"updated":2,"closed":3}`))
		case mode == "brands":
			w.Write([]byte(`{"ok":true,"brands":[{"id":"brand_ramenya","name":"Ramen Ya","aliases":"RamenYa"}]}`))
		default:
			w.Write([]byte(`{"ok":false,"error":"unknown mode"}`))
		}
	}))
	defer srv.Close()
	repo := NewSheetRepository(config.SheetConfig{URL: srv.URL, Token: "tok", TimeoutSeconds: 5})

	brands, err := repo.GetBrands(context.Background())
	if err != nil || len(brands) != 1 || brands[0].ID != "brand_ramenya" {
		t.Fatalf("GetBrands = %+v, %v", brands, err)
	}

	res, err := repo.UpsertOutlets(context.Background(), "osm", []models.Outlet{{ID: "out_osm_n1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res != (models.UpsertResult{Added: 1, Updated: 2, Closed: 3}) {
		t.Fatalf("UpsertOutlets = %+v", res)
	}
	if string(posted["source"]) != `"osm"` || !strings.Contains(string(posted["outlets"]), "out_osm_n1") {
		t.Fatalf("posted %s", posted)
	}
}

func TestSheetRepositoryNotConfiguredHint(t *testing.T) {
	_, err := NewSheetRepository(config.SheetConfig{}).GetBrands(context.Background())
	if err == nil || !strings.Contains(err.Error(), "-dry-run with -brands-file") {
		t.Fatalf("want the dry-run hint, got %v", err)
	}
}
