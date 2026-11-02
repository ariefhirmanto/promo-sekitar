//go:build golden

// Golden set: real items with hand-checked answers, run against a live parser. It builds the
// parser config.yaml names (Parser.Name), or PARSER=<name>, through the registry, so comparing
// providers needs no code change:
//
//	export ANTHROPIC_API_KEY=...   # in ~/.profile, never in a repo file
//	go test -tags golden -v ./promo/parser/
//	PARSER=<name> go test -tags golden -v ./promo/parser/
//
// The cases are real posts, so they live in the gitignored testdata/live/golden/ (rule 12 in
// .claude/rules/adapters.md), one JSON file per item:
//
//	{
//	  "request": {"Item": {"ID": "...", "URL": "...", "PostedAt": "...", "Text": "..."},
//	              "Today": "2026-11-02", "Brand": "HokBen", "Outlets": ["..."], "KnownBrands": []},
//	  "want": [{"deal_numbers": "30_50_000", "channel": "all", "valid_from": "", "valid_until": "2026-11-30"}]
//	}
//
// "want" lists the deals (is_deal true) the item announces; an empty list means none. A case
// passes when the parser returns exactly those deals: same count, and each matches one wanted
// deal on the numbers in its deal text, channel and dates. The set passes at 15 of 17 (88%).
package parser_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/promo/parser"

	// every parser the golden set can run; add new ones here too
	_ "places-fetcher/promo/parser/claude"
)

const goldenPass = 15.0 / 17.0

type goldenCase struct {
	Request models.ParseRequest `json:"request"`
	Want    []goldenDeal        `json:"want"`
}

type goldenDeal struct {
	DealNumbers string `json:"deal_numbers"`
	Channel     string `json:"channel"`
	ValidFrom   string `json:"valid_from"`
	ValidUntil  string `json:"valid_until"`
}

var numbers = regexp.MustCompile(`[0-9]+`)

func dealOf(p models.ParsedPromo) goldenDeal {
	return goldenDeal{DealNumbers: strings.Join(numbers.FindAllString(p.Deal, -1), "_"), Channel: p.Channel,
		ValidFrom: p.ValidFrom, ValidUntil: p.ValidUntil}
}

func TestGoldenSet(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/live/golden/*.json")
	if len(files) == 0 {
		t.Skip("no cases in testdata/live/golden/")
	}
	cfg := config.LoadConfig("../..")
	name := cfg.Parser.Name
	if env := os.Getenv("PARSER"); env != "" {
		name = env
	}
	p, err := parser.New(name, cfg)
	if err != nil {
		t.Fatal(err)
	}

	passed, cost := 0, 0.0
	var cacheRead int64
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var c goldenCase
		if err = json.Unmarshal(b, &c); err != nil {
			t.Fatalf("%s: %v", f, err)
		}

		res, err := p.Parse(context.Background(), c.Request)
		cost += res.CostUSD
		cacheRead += res.CacheReadTokens
		if err != nil {
			t.Logf("FAIL %s: %v", filepath.Base(f), err)
			continue
		}
		var got []goldenDeal
		for _, pp := range res.Promos {
			if pp.IsDeal {
				got = append(got, dealOf(pp))
			}
		}
		if sameDeals(got, c.Want) {
			passed++
			continue
		}
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(c.Want)
		t.Logf("FAIL %s:\n  got  %s\n  want %s", filepath.Base(f), g, w)
	}

	t.Logf("parser %s: %d of %d cases passed; spent $%.4f, cache read %d tokens", name, passed, len(files), cost, cacheRead)
	if float64(passed) < goldenPass*float64(len(files)) {
		t.Fatalf("golden set below %.0f%%", goldenPass*100)
	}
}

func sameDeals(got, want []goldenDeal) bool {
	if len(got) != len(want) {
		return false
	}
	used := make([]bool, len(got))
	for _, w := range want {
		found := false
		for i, g := range got {
			if !used[i] && g == w {
				used[i], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
