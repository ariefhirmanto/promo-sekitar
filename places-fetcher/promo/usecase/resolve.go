package usecase

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"places-fetcher/match"
	"places-fetcher/models"
)

var digits = regexp.MustCompile(`[0-9]+`)

// outletNames lists a brand's outlet and mall names for the prompt: sorted and without
// duplicates, so the cached part of the request stays the same for every item of the brand.
func outletNames(outlets []models.Outlet) []string {
	seen := map[string]bool{}
	var names []string
	for _, o := range outlets {
		for _, n := range []string{o.Name, o.MallName} {
			if n = strings.TrimSpace(n); n != "" && !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	sort.Strings(names)
	return names
}

// brandLabel is the brand line of the prompt: "HokBen (Hoka Hoka Bento, HokBen+)".
func brandLabel(b models.Brand) string {
	names := b.Names()
	if len(names) == 1 {
		return names[0]
	}
	return names[0] + " (" + strings.Join(names[1:], ", ") + ")"
}

// matchOutlets turns the places a promo is limited to into the brand's outlet ids. A limit
// that matches no outlet makes ok false: the promo is then saved hidden, since showing it at
// every outlet would be wrong.
func matchOutlets(limits []models.OutletLimit, outlets []models.Outlet) (ids []string, ok bool) {
	ok = true
	set := map[string]bool{}
	for _, l := range limits {
		want := match.Compact(l.Name)
		if len(want) < 3 {
			ok = false
			continue
		}
		found := false
		for _, o := range outlets {
			if outletMatches(l.Kind, l.Name, want, o) {
				set[o.ID] = true
				found = true
			}
		}
		if !found {
			ok = false
		}
	}
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, ok
}

// outletMatches checks one limit against one outlet. want is the limit's name compacted.
func outletMatches(kind, name, want string, o models.Outlet) bool {
	contains := func(field string) bool {
		return strings.Contains(match.Compact(field), want)
	}
	switch kind {
	case "city":
		return contains(o.Address)
	case "format":
		return match.Compact(o.Format) == want
	default: // mall or outlet
		// The other way round only for whole words of the mall: "HokBen Grand Indonesia" names
		// the mall "Grand Indonesia", but "Pakuwon Mall" doesn't name "ÆON Mall". Never for the
		// outlet name, which is often just the brand that every place the promo names contains.
		return contains(o.Name) || contains(o.MallName) || hasWords(name, o.MallName)
	}
}

// hasWords reports whether the words of part appear in s, in order and next to each other.
func hasWords(s, part string) bool {
	w := func(v string) string {
		return " " + strings.Join(strings.FieldsFunc(strings.ToLower(v), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}), " ") + " "
	}
	p := w(part)
	return p != "  " && strings.Contains(w(s), p)
}

// dedupeKey identifies a promo across sources and runs: the brand, the deal's numbers (so
// "Diskon 50%" and "50% off" meet), the dates and the outlets. Without numbers the deal's
// compact text stands in.
func dedupeKey(p models.Promo) string {
	deal := strings.Join(digits.FindAllString(p.Deal, -1), "_")
	if deal == "" {
		deal = match.Compact(p.Deal)
	}
	return strings.Join([]string{p.BrandID, deal, p.ValidFrom, p.ValidUntil, strings.Join(p.OutletIDs, ",")}, "|")
}

func promoID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "promo_" + hex.EncodeToString(sum[:])[:16]
}

// mergePromo folds a promo read again (from another source, or another item of the same
// source) into the one already kept: sources and refs add up, the more confident reading wins,
// and a promo shown by any reading stays shown.
func mergePromo(kept, again models.Promo) models.Promo {
	out := kept
	if again.Confidence > kept.Confidence {
		out = again
	}
	out.SourceIDs = union(kept.SourceIDs, again.SourceIDs)
	out.SourceRefs = union(kept.SourceRefs, again.SourceRefs)
	if kept.Status == "active" || again.Status == "active" {
		out.Status = "active"
	}
	if kept.PostedAt != "" && (again.PostedAt == "" || kept.PostedAt < again.PostedAt) {
		out.PostedAt = kept.PostedAt
	}
	return out
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range append(append([]string{}, a...), b...) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
