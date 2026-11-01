package replay

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"places-fetcher/models"
)

func TestReplayServesItemsPerSource(t *testing.T) {
	a, err := New(filepath.Join("..", "..", "..", "testdata", "promo_items_2026-11-01.json"))
	if err != nil {
		t.Fatal(err)
	}
	ad, ok := a.ForType("anything")
	if !ok || ad.Type() != Name {
		t.Fatal("replay stands in for every type")
	}
	katalog := models.Source{ID: "src_ramenya_katalog"}
	res, err := ad.FetchItems(context.Background(), models.FetchRequest{Source: katalog})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("want the 3 RamenYa items from the fixture, got %d", len(res.Items))
	}
	for _, it := range res.Items {
		if it.SourceID != "src_ramenya_katalog" {
			t.Fatalf("item from another source: %+v", it)
		}
	}
	known, _ := ad.FetchItems(context.Background(), models.FetchRequest{Source: katalog, KnownRefs: map[string]bool{"1001": true}})
	if len(known.Items) != 2 {
		t.Fatalf("known refs are skipped: got %d items", len(known.Items))
	}
	none, _ := ad.FetchItems(context.Background(), models.FetchRequest{Source: models.Source{ID: "src_not_in_file"}})
	if len(none.Items) != 0 {
		t.Fatal("a source with no saved items returns none")
	}
}

func TestReplayRejectsItemsWithoutSourceOrRef(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	os.WriteFile(path, []byte(`[{"source_id":"src_a","url":"u"}]`), 0o644)
	if _, err := New(path); err == nil {
		t.Fatal("an item without a ref must be rejected")
	}
}
