package overpass

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"places-fetcher/models"
)

const amenityFilter = `["amenity"~"^(restaurant|fast_food|cafe|food_court)$"]`

// BuildQuery asks for every food place in the box whose name or brand tag matches an alias,
// then for every mall outline in the box (with geometry, to find which mall an outlet is in).
func BuildQuery(brands []models.Brand, bbox string) string {
	seen := map[string]bool{}
	var pats []string
	for _, b := range brands {
		for _, a := range b.Names() {
			if p := aliasPattern(a); p != "" && !seen[p] {
				seen[p] = true
				pats = append(pats, p)
			}
		}
	}
	sort.Strings(pats)
	re := strings.Join(pats, "|")

	return fmt.Sprintf(`[out:json][timeout:180];`+
		`(nwr%[1]s["name"~"%[2]s",i](%[3]s);nwr%[1]s["brand"~"%[2]s",i](%[3]s););out center tags;`+
		// "out geom" (not "out geom tags"): with tags-only verbosity relations lose their members.
		`(way["shop"="mall"](%[3]s);relation["shop"="mall"](%[3]s););out geom;`,
		amenityFilter, re, bbox)
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

// aliasPattern turns "Ramen-Ya" into "ramen[ -]?ya". Punctuation-only differences (HokBen+)
// are left to the usecase matcher, which sees every place the shorter alias already pulled in.
func aliasPattern(alias string) string {
	words := strings.Fields(nonWord.ReplaceAllString(strings.ToLower(alias), " "))
	if len(words) == 0 || len(strings.Join(words, "")) < 4 {
		return ""
	}
	return strings.Join(words, "[ -]?")
}
