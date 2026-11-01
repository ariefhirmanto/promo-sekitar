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
	res, err := ad.FetchItems(context.Background(), models.Source{ID: "src_ramenya_katalog"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) == 0 {
		t.Fatal("want the RamenYa items from the fixture")
	}
	for _, it := range res.Items {
		if it.SourceID != "src_ramenya_katalog" {
			t.Fatalf("item from another source: %+v", it)
		}
	}
	none, _ := ad.FetchItems(context.Background(), models.Source{ID: "src_not_in_file"})
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
