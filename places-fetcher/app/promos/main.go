// Promo fetcher for MakanDiMana: reads the sheet's sources tab, fetches every due source, stores
// what it read in the raw_items tab, then reads new items into promos with the parser that
// Parser.Name in config.yaml picks (Claude Haiku). A dry run still calls the parser (it costs a
// little); add -parse=false to only fetch, or -parser <name> to try another parser.
//
//	SHEET_URL=https://script.google.com/macros/s/.../exec SHEET_TOKEN=... go run ./app/promos
//	go run ./app/promos -dry-run                       # read sources from the sheet, write nothing
//	go run ./app/promos -dry-run -source src_bca_fnb   # one source, due or not
//	go run ./app/promos -dry-run -parse=false -sources-file testdata/sources.json -items-file testdata/promo_items_2026-11-01.json
//	go run ./app/promos -dry-run -sources-file testdata/sources.json -items-file testdata/promo_items_2026-11-01.json -brands-file testdata/brands.json
package main

import (
	"flag"
	"log"
	"strings"

	_config "places-fetcher/config"
	_job "places-fetcher/job"
	"places-fetcher/promo/parser"
	"places-fetcher/promo/source"
)

func main() {
	var opts _job.PromoOptions
	flag.BoolVar(&opts.DryRun, "dry-run", false, "don't write to the sheet; log source statuses and write -csv")
	flag.StringVar(&opts.RawCSVPath, "csv", "raw_items.csv", "CSV written in -dry-run mode (the raw items read)")
	flag.StringVar(&opts.SourceID, "source", "", "run only this source id, due or not")
	flag.StringVar(&opts.ItemsFile, "items-file", "", "replay saved raw items (JSON) instead of fetching; source types: "+strings.Join(source.Names(), ", "))
	flag.StringVar(&opts.SourcesFile, "sources-file", "", "read sources from a JSON file instead of the sheet")
	flag.BoolVar(&opts.Parse, "parse", true, "parse new raw items into promos (needs the parser's API key)")
	flag.StringVar(&opts.Parser, "parser", "", "parser to use instead of Parser.Name in config.yaml; registered: "+strings.Join(parser.Names(), ", "))
	flag.StringVar(&opts.PromoCSV, "promos-csv", "promos.csv", "CSV written in -dry-run mode (the promos read)")
	flag.StringVar(&opts.BrandsFile, "brands-file", "", "with -sources-file: read brands from a JSON file")
	flag.StringVar(&opts.OutletsFile, "outlets-file", "", "with -sources-file: read outlets from a JSON file")
	configPath := flag.String("config", ".", "directory holding config.yaml")
	flag.Parse()

	config := _config.LoadConfig(*configPath)

	err := _job.NewPromoJob(&config, opts).Run()
	if err != nil {
		log.Fatalf("%s", err.Error())
	}
}
