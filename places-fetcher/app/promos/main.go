// Promo fetcher for MakanDiMana: reads the sheet's sources tab, fetches every due source, and
// stores what it read in the raw_items tab. (Parsing into promos comes with M4 of the RFC.)
//
//	SHEET_URL=https://script.google.com/macros/s/.../exec SHEET_TOKEN=... go run ./app/promos
//	go run ./app/promos -dry-run                       # read sources from the sheet, write nothing
//	go run ./app/promos -dry-run -source src_bca_fnb   # one source, due or not
//	go run ./app/promos -dry-run -sources-file testdata/sources.json -items-file testdata/promo_items_2026-11-01.json
package main

import (
	"flag"
	"log"
	"strings"

	_config "places-fetcher/config"
	_job "places-fetcher/job"
	"places-fetcher/promo/source"
)

func main() {
	var opts _job.PromoOptions
	flag.BoolVar(&opts.DryRun, "dry-run", false, "don't write to the sheet; log source statuses and write -csv")
	flag.StringVar(&opts.RawCSVPath, "csv", "raw_items.csv", "CSV written in -dry-run mode (the raw items read)")
	flag.StringVar(&opts.SourceID, "source", "", "run only this source id, due or not")
	flag.StringVar(&opts.ItemsFile, "items-file", "", "replay saved raw items (JSON) instead of fetching; source types: "+strings.Join(source.Names(), ", "))
	flag.StringVar(&opts.SourcesFile, "sources-file", "", "read sources from a JSON file instead of the sheet")
	configPath := flag.String("config", ".", "directory holding config.yaml")
	flag.Parse()

	config := _config.LoadConfig(*configPath)

	err := _job.NewPromoJob(&config, opts).Run()
	if err != nil {
		log.Fatalf("%s", err.Error())
	}
}
