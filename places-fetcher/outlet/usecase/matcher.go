package usecase

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"places-fetcher/match"
	"places-fetcher/models"
	"places-fetcher/outlet"
)

// Compact is match.Compact, kept here for the places job's callers.
func Compact(s string) string { return match.Compact(s) }

// MatchBrand returns the brand and alias with the longest alias found in the place's name or
// alternative names, so "HokBen+ Kemang" picks the "HokBen+" alias over "HokBen".
func MatchBrand(p models.Place, brands []models.Brand) (models.Brand, string, bool) {
	return match.BestBrand(append([]string{p.Name}, p.AltNames...), brands)
}

// FormatFor keeps sub-brand aliases that extend the brand name ("HokBen+", "HokBen 24 Jam");
// plain spellings and old names ("Hoka Hoka Bento") collapse to the brand name.
func FormatFor(b models.Brand, alias string) string {
	ca, cb := Compact(alias), Compact(b.Name)
	if ca != cb && strings.Contains(ca, cb) {
		return alias
	}
	return b.Name
}

var mallWords = regexp.MustCompile(`(?i)\b(mall|plaza|square|city|town|center|centre|junction|place|walk|park|avenue|xchange|kasablanka|grand indonesia|pim|itc|aeon|lotte|living world|supermal|pakuwon|summarecon|gandaria|senayan|kokas|blok m|trade mall|lippo|cbd)\b`)

// MallFromName is the fallback when no mall outline contains the outlet: the name left over
// after the brand ("HokBen Mall Taman Anggrek" -> "Mall Taman Anggrek"), then the mall hints.
func MallFromName(p models.Place, b models.Brand) string {
	names := b.Names()
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })

	rest := p.Name
	for _, n := range names {
		rest = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(n)).ReplaceAllString(rest, "")
	}
	rest = strings.Trim(rest, " -–,|()")

	for _, c := range append([]string{rest}, p.MallHints...) {
		if c != "" && mallWords.MatchString(c) {
			return strings.TrimSpace(c)
		}
	}
	return ""
}

// BuildOutlets turns provider places into outlets: match a brand, find the mall, compute H3
// cells, then merge points of the same brand within mergeRadiusM.
func BuildOutlets(res models.PlaceResult, brands []models.Brand, geo outlet.GeoIndexer, mergeRadiusM float64) []models.Outlet {
	malls := NewMallIndex(res.Malls)

	var out []models.Outlet
	for _, p := range res.Places {
		b, alias, ok := MatchBrand(p, brands)
		if !ok {
			continue
		}

		name := p.Name
		if name == "" {
			name = b.Name
		}

		o := models.Outlet{
			ID:      fmt.Sprintf("out_%s_%s", p.Source, p.SourceRef),
			BrandID: b.ID,
			Name:    name,
			Address: p.Address,
			Lat:     math.Round(p.Lat*1e6) / 1e6,
			Lng:     math.Round(p.Lng*1e6) / 1e6,
			Format:  FormatFor(b, alias),
			Source:  p.Source,
		}
		if m, ok := malls.Find(p.Lat, p.Lng); ok {
			o.MallName = m.Name
		} else {
			o.MallName = MallFromName(p, b)
		}
		o.H3R8, o.H3R6 = geo.Cells(o.Lat, o.Lng)

		out = append(out, o)
	}

	return MergeNearby(out, mergeRadiusM)
}

// MergeNearby drops a point when the same brand already has one within radiusM (OSM often maps
// an outlet as both a node and a building), filling in address and mall from the dropped point.
func MergeNearby(outlets []models.Outlet, radiusM float64) []models.Outlet {
	// Ways and relations (usually better tagged) come before nodes: "out_osm_w…" > "out_osm_n…".
	sort.SliceStable(outlets, func(i, j int) bool { return outlets[i].ID > outlets[j].ID })

	var kept []models.Outlet
	for _, o := range outlets {
		dup := false
		for i := range kept {
			k := &kept[i]
			if k.BrandID == o.BrandID && DistanceM(k.Lat, k.Lng, o.Lat, o.Lng) <= radiusM {
				if k.Address == "" {
					k.Address = o.Address
				}
				if k.MallName == "" {
					k.MallName = o.MallName
				}
				dup = true
				break
			}
		}
		if !dup {
			kept = append(kept, o)
		}
	}

	sort.Slice(kept, func(i, j int) bool {
		if kept[i].BrandID != kept[j].BrandID {
			return kept[i].BrandID < kept[j].BrandID
		}
		return kept[i].Name < kept[j].Name
	})
	return kept
}

func DistanceM(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371000
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
