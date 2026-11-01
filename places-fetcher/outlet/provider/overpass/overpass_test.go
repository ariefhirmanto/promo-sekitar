package overpass

import (
	"context"
	"strings"
	"testing"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/outlet/provider"
)

func TestRegistered(t *testing.T) {
	p, err := provider.New(Name, config.MainConfig{}, provider.Options{File: "../../../testdata/overpass_2026-11-01.json"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Source() != "osm" {
		t.Errorf("source %q, want osm", p.Source())
	}
	res, err := p.SearchPlaces(context.Background(), models.PlaceQuery{BBox: "1,2,3,4"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Places) == 0 || len(res.Malls) == 0 {
		t.Errorf("fixture gave %d places, %d malls", len(res.Places), len(res.Malls))
	}
}

func TestAliasPattern(t *testing.T) {
	cases := map[string]string{
		"Ramen-Ya":      "ramen[ -]?ya",
		"HokBen+":       "hokben",
		"HokBen 24 Jam": "hokben[ -]?24[ -]?jam",
		"Ya":            "", // too short to search for
	}
	for in, want := range cases {
		if got := aliasPattern(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestBuildQueryAsksForMalls(t *testing.T) {
	q := BuildQuery([]models.Brand{{ID: "b", Name: "HokBen", Aliases: "HokBen+"}}, "1,2,3,4")
	for _, want := range []string{`"name"~"hokben",i](1,2,3,4)`, `way["shop"="mall"](1,2,3,4)`, "out geom;"} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %s:\n%s", want, q)
		}
	}
}

func TestStitchRings(t *testing.T) {
	a, b := models.LatLng{Lat: 0, Lng: 0}, models.LatLng{Lat: 0, Lng: 1}
	c, d := models.LatLng{Lat: 1, Lng: 1}, models.LatLng{Lat: 1, Lng: 0}
	// Two halves of a square, the second one drawn backwards.
	rings := StitchRings([][]models.LatLng{{a, b, c}, {a, d, c}})
	if len(rings) != 1 {
		t.Fatalf("got %d rings, want 1", len(rings))
	}
	r := rings[0]
	if len(r) != 5 || r[0] != r[len(r)-1] {
		t.Errorf("ring not closed: %v", r)
	}
}

func TestTransform(t *testing.T) {
	raw := `{"elements":[
	  {"type":"node","id":1,"lat":-6.2,"lon":106.8,"tags":{"name":"HokBen","brand":"HokBen","addr:street":"Jalan A","addr:housenumber":"5","addr:housename":"Plaza X"}},
	  {"type":"way","id":2,"center":{"lat":-6.3,"lon":106.9},"tags":{"name":"Marugame Udon"}},
	  {"type":"way","id":3,"geometry":[{"lat":0,"lon":0},{"lat":0,"lon":1},{"lat":1,"lon":1},{"lat":0,"lon":0}],"tags":{"shop":"mall","name":"Mall A"}},
	  {"type":"way","id":4,"geometry":[{"lat":0,"lon":0},{"lat":0,"lon":1},{"lat":1,"lon":1},{"lat":0,"lon":0}],"tags":{"shop":"mall"}},
	  {"type":"relation","id":5,"bounds":{"minlat":2,"minlon":2,"maxlat":3,"maxlon":3},"tags":{"shop":"mall","name":"Mall B"}}
	]}`
	res, err := Transform([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Places) != 2 {
		t.Fatalf("places: %+v", res.Places)
	}
	p := res.Places[0]
	if p.Source != "osm" || p.SourceRef != "n1" || p.Address != "Plaza X, Jalan A 5" ||
		len(p.AltNames) != 1 || len(p.MallHints) != 1 || p.MallHints[0] != "Plaza X" {
		t.Errorf("node transform: %+v", p)
	}
	if res.Places[1].SourceRef != "w2" || res.Places[1].Lat != -6.3 {
		t.Errorf("way center transform: %+v", res.Places[1])
	}

	if len(res.Malls) != 2 || res.Malls[0].Name != "Mall A" || res.Malls[1].Name != "Mall B" {
		t.Fatalf("malls (unnamed one must be skipped): %+v", res.Malls)
	}
	if len(res.Malls[1].Rings[0]) != 5 {
		t.Errorf("relation without members should fall back to its bounds: %+v", res.Malls[1].Rings)
	}
}

func TestTransformTimeout(t *testing.T) {
	raw := `{"remark":"runtime error: Query timed out in \"query\" at line 1 after 181 seconds.","elements":[]}`
	if _, err := Transform([]byte(raw)); err == nil {
		t.Error("want an error for a timed-out query")
	}
}
