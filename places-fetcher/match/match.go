// Package match holds the brand matching both jobs use: the places job on OSM names, the promo
// job on the brand a promo names.
package match

import (
	"regexp"
	"strings"

	"places-fetcher/models"
)

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

// Compact lowercases and drops everything but letters and digits; "+" becomes "plus" so
// "HokBen+" stays distinct from "HokBen".
func Compact(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "+", "plus")
	return nonWord.ReplaceAllString(s, "")
}

// BestBrand returns the brand and alias with the longest alias found in any of names, so
// "HokBen+ Kemang" picks the "HokBen+" alias over "HokBen". Aliases shorter than 4 characters
// (after Compact) never match, to keep "KFC"-like noise out of longer words.
func BestBrand(names []string, brands []models.Brand) (models.Brand, string, bool) {
	var hay []string
	for _, n := range names {
		if v := Compact(n); v != "" {
			hay = append(hay, v)
		}
	}

	var best models.Brand
	bestAlias := ""
	for _, b := range brands {
		for _, a := range b.Names() {
			ca := Compact(a)
			if len(ca) < 4 || len(ca) <= len(Compact(bestAlias)) {
				continue
			}
			for _, h := range hay {
				if strings.Contains(h, ca) {
					best, bestAlias = b, a
					break
				}
			}
		}
	}
	return best, bestAlias, bestAlias != ""
}
