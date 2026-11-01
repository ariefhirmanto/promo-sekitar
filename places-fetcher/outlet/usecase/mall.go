package usecase

import (
	"math"

	"places-fetcher/models"
)

type mallEntry struct {
	mall           models.Mall
	minLat, minLng float64
	maxLat, maxLng float64
	area           float64
}

// MallIndex answers "which mall outline is this point in?".
type MallIndex struct {
	entries []mallEntry
}

func NewMallIndex(malls []models.Mall) *MallIndex {
	idx := &MallIndex{}
	for _, m := range malls {
		e := mallEntry{mall: m, minLat: math.Inf(1), minLng: math.Inf(1), maxLat: math.Inf(-1), maxLng: math.Inf(-1)}
		for _, ring := range m.Rings {
			for _, p := range ring {
				e.minLat, e.maxLat = math.Min(e.minLat, p.Lat), math.Max(e.maxLat, p.Lat)
				e.minLng, e.maxLng = math.Min(e.minLng, p.Lng), math.Max(e.maxLng, p.Lng)
			}
			e.area += ringArea(ring)
		}
		idx.entries = append(idx.entries, e)
	}
	return idx
}

// Find returns the smallest mall containing the point, so a mall mapped inside a larger
// complex (e.g. a wing inside a superblock) wins over the complex.
func (idx *MallIndex) Find(lat, lng float64) (models.Mall, bool) {
	var best *mallEntry
	for i := range idx.entries {
		e := &idx.entries[i]
		if lat < e.minLat || lat > e.maxLat || lng < e.minLng || lng > e.maxLng {
			continue
		}
		for _, ring := range e.mall.Rings {
			if PointInRing(lat, lng, ring) {
				if best == nil || e.area < best.area {
					best = e
				}
				break
			}
		}
	}
	if best == nil {
		return models.Mall{}, false
	}
	return best.mall, true
}

// PointInRing is the even-odd ray casting test; the ring is treated as closed.
func PointInRing(lat, lng float64, ring []models.LatLng) bool {
	in := false
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		a, b := ring[i], ring[j]
		if (a.Lat > lat) != (b.Lat > lat) &&
			lng < (b.Lng-a.Lng)*(lat-a.Lat)/(b.Lat-a.Lat)+a.Lng {
			in = !in
		}
	}
	return in
}

// ringArea is the shoelace area in squared degrees; only used to compare malls.
func ringArea(ring []models.LatLng) float64 {
	s := 0.0
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		s += (ring[j].Lng + ring[i].Lng) * (ring[j].Lat - ring[i].Lat)
	}
	return math.Abs(s / 2)
}
