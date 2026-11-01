package usecase

import (
	"context"
	"testing"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/outlet"
	"places-fetcher/outlet/provider"
	_ "places-fetcher/outlet/provider/overpass"
	"places-fetcher/outlet/repository"
)

type fakeGeo struct{}

func (fakeGeo) Cells(lat, lng float64) (string, string) { return "r8", "r6" }

type memOutletRepo struct {
	source string
	got    []models.Outlet
}

func (m *memOutletRepo) UpsertOutlets(ctx context.Context, source string, outlets []models.Outlet) (models.UpsertResult, error) {
	m.source, m.got = source, outlets
	return models.UpsertResult{Added: len(outlets)}, nil
}

// fakeProvider stands in for any other vendor adapter.
type fakeProvider struct{ res models.PlaceResult }

func (f fakeProvider) Source() string { return "fake" }
func (f fakeProvider) SearchPlaces(ctx context.Context, q models.PlaceQuery) (models.PlaceResult, error) {
	return f.res, nil
}

var brands = []models.Brand{
	{ID: "brand_hokben", Name: "HokBen", Aliases: "Hokben, HokBen+, HokBen 24 Jam, Hoka Hoka Bento"},
	{ID: "brand_marugame", Name: "Marugame Udon", Aliases: "Marugame, Marugame Udon Indonesia"},
	{ID: "brand_ramenya", Name: "Ramen Ya", Aliases: "RamenYa, Ramen-Ya"},
}

func TestMatchBrand(t *testing.T) {
	cases := []struct {
		name, brand, wantBrand, wantFormat string
	}{
		{"Hoka-Hoka Bento (HokBen)", "", "brand_hokben", "HokBen"},
		{"Hoka hoka bento Japanese food", "", "brand_hokben", "HokBen"},
		{"HokBen+ Kemang", "", "brand_hokben", "HokBen+"},
		{"HokBen 24 Jam Tebet", "", "brand_hokben", "HokBen 24 Jam"},
		{"Ramen Ya!", "", "brand_ramenya", "Ramen Ya"},
		{"Marugame Udon", "Marugame Seimen", "brand_marugame", "Marugame Udon"},
		{"Warung Bu Siti", "", "", ""},
	}
	for _, c := range cases {
		p := models.Place{Name: c.name}
		if c.brand != "" {
			p.AltNames = []string{c.brand}
		}
		b, alias, ok := MatchBrand(p, brands)
		if c.wantBrand == "" {
			if ok {
				t.Errorf("%q: matched %s, want no match", c.name, b.ID)
			}
			continue
		}
		if !ok || b.ID != c.wantBrand {
			t.Errorf("%q: got %q ok=%v, want %s", c.name, b.ID, ok, c.wantBrand)
			continue
		}
		if f := FormatFor(b, alias); f != c.wantFormat {
			t.Errorf("%q: format %q, want %q", c.name, f, c.wantFormat)
		}
	}
}

func TestMallFromName(t *testing.T) {
	cases := map[string]string{
		"HokBen Mall Taman Anggrek": "Mall Taman Anggrek",
		"Hokben Ruko Tebet":         "",
		"Hoka Hoka Bento (HokBen)":  "",
	}
	for name, want := range cases {
		if got := MallFromName(models.Place{Name: name}, brands[0]); got != want {
			t.Errorf("%q: got %q, want %q", name, got, want)
		}
	}
}

func TestMallIndexPicksSmallest(t *testing.T) {
	square := func(lat, lng, d float64) [][]models.LatLng {
		return [][]models.LatLng{{
			{Lat: lat, Lng: lng}, {Lat: lat, Lng: lng + d}, {Lat: lat + d, Lng: lng + d},
			{Lat: lat + d, Lng: lng}, {Lat: lat, Lng: lng},
		}}
	}
	idx := NewMallIndex([]models.Mall{
		{SourceRef: "big", Name: "Superblock", Rings: square(-6.20, 106.80, 0.01)},
		{SourceRef: "small", Name: "Mall Inside", Rings: square(-6.198, 106.802, 0.002)},
	})
	if m, ok := idx.Find(-6.197, 106.803); !ok || m.Name != "Mall Inside" {
		t.Errorf("inner point: got %q ok=%v", m.Name, ok)
	}
	if m, ok := idx.Find(-6.192, 106.808); !ok || m.Name != "Superblock" {
		t.Errorf("outer point: got %q ok=%v", m.Name, ok)
	}
	if _, ok := idx.Find(-6.30, 106.80); ok {
		t.Error("point outside every mall matched")
	}
}

func TestMergeNearby(t *testing.T) {
	got := MergeNearby([]models.Outlet{
		{ID: "out_osm_n1", BrandID: "b", Lat: -6.2, Lng: 106.8},
		{ID: "out_osm_w2", BrandID: "b", Lat: -6.2002, Lng: 106.8, Address: "Jl. A"}, // ~22 m away
		{ID: "out_osm_n3", BrandID: "c", Lat: -6.2, Lng: 106.8},                      // other brand
	}, 50)
	if len(got) != 2 {
		t.Fatalf("got %d outlets, want 2", len(got))
	}
	if got[0].ID != "out_osm_w2" || got[0].Address != "Jl. A" {
		t.Errorf("kept %+v, want the way", got[0])
	}
}

// Runs the whole usecase on a real Overpass response (1 Nov 2026, Jabodetabek, outlets + malls).
func TestRefreshOutletsFixture(t *testing.T) {
	places, err := provider.New("overpass", config.MainConfig{}, provider.Options{File: "../../testdata/overpass_2026-11-01.json"})
	if err != nil {
		t.Fatal(err)
	}
	out := &memOutletRepo{}
	uc := NewOutletUsecase(repository.NewBrandFileRepository("../../testdata/brands.json"), places, out, fakeGeo{}, 50)

	resp, err := uc.RefreshOutlets(context.Background(), outlet.InputRefresh{BBox: "-6.80,106.40,-5.95,107.30"})
	if err != nil {
		t.Fatal(err)
	}
	if out.source != "osm" {
		t.Errorf("upsert source %q, want osm", out.source)
	}
	if resp.Malls < 200 {
		t.Errorf("got %d malls, want the fixture's ~280", resp.Malls)
	}
	for _, id := range []string{"brand_hokben", "brand_marugame", "brand_ramenya"} {
		if resp.PerBrand[id] == 0 {
			t.Errorf("no outlets for %s", id)
		}
	}
	inMall := 0
	for _, o := range out.got {
		if o.MallName != "" {
			inMall++
		}
	}
	t.Logf("%d places, %d malls -> %d outlets (%v), %d with mall_name", resp.Places, resp.Malls, len(out.got), resp.PerBrand, inMall)
	if inMall <= 1 {
		t.Errorf("only %d outlets got a mall_name; mall outlines aren't being used", inMall)
	}
}

// Any adapter works without usecase changes; ids and the upsert scope follow its Source().
func TestRefreshOutletsWithAnotherProvider(t *testing.T) {
	places := fakeProvider{res: models.PlaceResult{
		Places: []models.Place{{Source: "fake", SourceRef: "abc", Name: "Marugame Udon Kokas", Lat: -6.22, Lng: 106.84}},
		Malls: []models.Mall{{Source: "fake", Name: "Kota Kasablanka", Rings: [][]models.LatLng{{
			{Lat: -6.23, Lng: 106.83}, {Lat: -6.23, Lng: 106.85}, {Lat: -6.21, Lng: 106.85}, {Lat: -6.21, Lng: 106.83},
		}}}},
	}}
	out := &memOutletRepo{}
	uc := NewOutletUsecase(repository.NewBrandFileRepository("../../testdata/brands.json"), places, out, fakeGeo{}, 50)

	if _, err := uc.RefreshOutlets(context.Background(), outlet.InputRefresh{BBox: "-6.80,106.40,-5.95,107.30"}); err != nil {
		t.Fatal(err)
	}
	if out.source != "fake" || len(out.got) != 1 {
		t.Fatalf("source %q, outlets %+v", out.source, out.got)
	}
	o := out.got[0]
	if o.ID != "out_fake_abc" || o.Source != "fake" || o.BrandID != "brand_marugame" || o.MallName != "Kota Kasablanka" {
		t.Errorf("outlet %+v", o)
	}
}

func TestRefreshOutletsRejectsBadBBox(t *testing.T) {
	uc := NewOutletUsecase(nil, nil, nil, fakeGeo{}, 50)
	if _, err := uc.RefreshOutlets(context.Background(), outlet.InputRefresh{BBox: "1,2,3"}); err == nil {
		t.Error("want an error for a 3-value bbox")
	}
}
