// Places fetcher for MakanDiMana: fills the sheet's outlets tab
// from a places provider (OpenStreetMap through Overpass by default).
//
//	SHEET_URL=https://script.google.com/macros/s/.../exec SHEET_TOKEN=... go run ./app
//	go run ./app -dry-run                  # print outlets and write outlets.csv, no sheet write
//	go run ./app -dry-run -brands-file testdata/brands.json -places-file testdata/overpass_2026-11-01.json
package main

import (
	"flag"
	"log"
	"strings"

	_config "places-fetcher/config"
	_job "places-fetcher/job"
	"places-fetcher/outlet/provider"
)

func main() {
	var opts _job.Options
	flag.BoolVar(&opts.DryRun, "dry-run", false, "don't write to the sheet; print outlets and write -csv")
	flag.StringVar(&opts.CSVPath, "csv", "outlets.csv", "CSV written in -dry-run mode (same columns as the outlets tab)")
	flag.StringVar(&opts.Provider, "provider", "", "places provider: "+strings.Join(provider.Names(), ", ")+" (default: Places.Provider in config.yaml)")
	flag.StringVar(&opts.BBox, "bbox", "", "south,west,north,east (default: Places.BBox in config.yaml)")
	flag.StringVar(&opts.PlacesFile, "places-file", "", "replay a saved provider response instead of calling the vendor")
	flag.StringVar(&opts.BrandsFile, "brands-file", "", "read brands from a JSON file ([{id,name,aliases}]) instead of the sheet")
	flag.StringVar(&opts.SaveRaw, "save-raw", "", "save the raw provider response to this file")
	configPath := flag.String("config", ".", "directory holding config.yaml")
	flag.Parse()

	config := _config.LoadConfig(*configPath)

	err := _job.NewJob(&config, opts).Run()
	if err != nil {
		log.Fatalf("%s", err.Error())
	}
}
