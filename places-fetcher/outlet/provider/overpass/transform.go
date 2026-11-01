package overpass

import (
	"encoding/json"
	"fmt"
	"strings"

	"places-fetcher/models"

	"github.com/pkg/errors"
)

// Raw Overpass JSON; these types never leave this package.

type osmPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type osmBounds struct {
	MinLat float64 `json:"minlat"`
	MinLon float64 `json:"minlon"`
	MaxLat float64 `json:"maxlat"`
	MaxLon float64 `json:"maxlon"`
}

type osmMember struct {
	Type     string     `json:"type"`
	Role     string     `json:"role"`
	Geometry []osmPoint `json:"geometry"`
}

type osmElement struct {
	Type     string            `json:"type"`
	ID       int64             `json:"id"`
	Lat      float64           `json:"lat"`
	Lon      float64           `json:"lon"`
	Center   *osmPoint         `json:"center"`
	Geometry []osmPoint        `json:"geometry"`
	Members  []osmMember       `json:"members"`
	Bounds   *osmBounds        `json:"bounds"`
	Tags     map[string]string `json:"tags"`
}

// Transform turns an Overpass response into vendor-neutral food places and named mall outlines.
func Transform(raw []byte) (resp models.PlaceResult, err error) {
	var r struct {
		Remark   string       `json:"remark"`
		Elements []osmElement `json:"elements"`
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return resp, errors.Wrap(err, "overpass.Transform.json.Unmarshal")
	}

	// A timed-out query still returns 200 with a remark and partial data; treat it as a failure
	// so a half-empty run never marks real outlets as closed.
	if strings.Contains(r.Remark, "runtime error") {
		return resp, fmt.Errorf("overpass remark: %s", r.Remark)
	}

	for _, e := range r.Elements {
		if e.Tags["shop"] == "mall" && e.Type != "node" {
			if m, ok := toMall(e); ok {
				resp.Malls = append(resp.Malls, m)
			}
			continue
		}
		if p, ok := toPlace(e); ok {
			resp.Places = append(resp.Places, p)
		}
	}

	return resp, nil
}

func toPlace(e osmElement) (models.Place, bool) {
	p := models.Place{
		Source:    source,
		SourceRef: fmt.Sprintf("%c%d", e.Type[0], e.ID), // n123, w456, r789
		Name:      strings.TrimSpace(e.Tags["name"]),
		Address:   addressFor(e.Tags),
		Lat:       e.Lat,
		Lng:       e.Lon,
	}
	if e.Center != nil {
		p.Lat, p.Lng = e.Center.Lat, e.Center.Lon
	}
	if p.Lat == 0 && p.Lng == 0 {
		return p, false
	}

	for _, k := range []string{"brand", "name:en", "official_name"} {
		if v := strings.TrimSpace(e.Tags[k]); v != "" {
			p.AltNames = append(p.AltNames, v)
		}
	}
	for _, k := range []string{"addr:housename", "addr:place"} {
		if v := strings.TrimSpace(e.Tags[k]); v != "" {
			p.MallHints = append(p.MallHints, v)
		}
	}

	return p, true
}

func addressFor(tags map[string]string) string {
	if v := tags["addr:full"]; v != "" {
		return v
	}

	street := strings.TrimSpace(tags["addr:street"] + " " + tags["addr:housenumber"])
	var parts []string
	for _, p := range []string{tags["addr:housename"], street, tags["addr:subdistrict"], tags["addr:district"], tags["addr:city"]} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

func toMall(e osmElement) (models.Mall, bool) {
	m := models.Mall{
		Source:    source,
		SourceRef: fmt.Sprintf("%c%d", e.Type[0], e.ID),
		Name:      strings.TrimSpace(e.Tags["name"]),
	}
	if m.Name == "" {
		return m, false
	}

	if e.Type == "way" {
		if len(e.Geometry) >= 4 {
			m.Rings = [][]models.LatLng{toLatLngs(e.Geometry)}
		}
	} else {
		var parts [][]models.LatLng
		for _, mem := range e.Members {
			if mem.Type == "way" && (mem.Role == "outer" || mem.Role == "") && len(mem.Geometry) >= 2 {
				parts = append(parts, toLatLngs(mem.Geometry))
			}
		}
		m.Rings = StitchRings(parts)
	}

	// No usable outline (e.g. a relation sent without members): fall back to its bounding box.
	if len(m.Rings) == 0 && e.Bounds != nil {
		b := e.Bounds
		m.Rings = [][]models.LatLng{{
			{Lat: b.MinLat, Lng: b.MinLon}, {Lat: b.MinLat, Lng: b.MaxLon},
			{Lat: b.MaxLat, Lng: b.MaxLon}, {Lat: b.MaxLat, Lng: b.MinLon}, {Lat: b.MinLat, Lng: b.MinLon},
		}}
	}

	return m, len(m.Rings) > 0
}

func toLatLngs(pts []osmPoint) []models.LatLng {
	out := make([]models.LatLng, len(pts))
	for i, p := range pts {
		out[i] = models.LatLng{Lat: p.Lat, Lng: p.Lon}
	}
	return out
}

// StitchRings joins a relation's outer ways end to end into rings. A ring that can't be
// closed is still kept; point-in-polygon treats it as closed.
func StitchRings(parts [][]models.LatLng) [][]models.LatLng {
	var rings [][]models.LatLng
	for len(parts) > 0 {
		ring := append([]models.LatLng(nil), parts[0]...)
		parts = parts[1:]

		for ring[0] != ring[len(ring)-1] {
			last := ring[len(ring)-1]
			found := -1
			for i, p := range parts {
				if p[0] == last {
					ring = append(ring, p[1:]...)
					found = i
					break
				}
				if p[len(p)-1] == last {
					for j := len(p) - 2; j >= 0; j-- {
						ring = append(ring, p[j])
					}
					found = i
					break
				}
			}
			if found < 0 {
				break
			}
			parts = append(parts[:found], parts[found+1:]...)
		}

		if len(ring) >= 4 {
			rings = append(rings, ring)
		}
	}
	return rings
}
