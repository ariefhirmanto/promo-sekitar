package job

import (
	"context"
	"os"
	"os/signal"
	"time"

	"places-fetcher/config"
	"places-fetcher/outlet"
	outletController "places-fetcher/outlet/controller"
	"places-fetcher/outlet/geo"
	"places-fetcher/outlet/provider"
	outletRepository "places-fetcher/outlet/repository"
	outletUsecase "places-fetcher/outlet/usecase"

	// Places vendor adapters register themselves; add new ones here.
	_ "places-fetcher/outlet/provider/overpass"
)

// Options come from the command line; they pick which repositories the job uses.
type Options struct {
	DryRun     bool   // write a CSV instead of the sheet
	CSVPath    string // CSV path in dry-run mode
	Provider   string // overrides Places.Provider from config
	BBox       string // overrides Places.BBox from config
	PlacesFile string // replay a saved provider response instead of calling the vendor
	BrandsFile string // read brands from a JSON file instead of the sheet
	SaveRaw    string // save the raw provider response
}

type Job struct {
	cfg  *config.MainConfig
	opts Options
}

func NewJob(cfg *config.MainConfig, opts Options) *Job {
	return &Job{cfg: cfg, opts: opts}
}

func (j *Job) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	sheetRepo := outletRepository.NewSheetRepository(j.cfg.Sheet)

	var brandRepo outlet.BrandRepository = sheetRepo
	if j.opts.BrandsFile != "" {
		brandRepo = outletRepository.NewBrandFileRepository(j.opts.BrandsFile)
	}

	providerName := j.cfg.Places.Provider
	if j.opts.Provider != "" {
		providerName = j.opts.Provider
	}
	places, err := provider.New(providerName, *j.cfg, provider.Options{File: j.opts.PlacesFile, SaveRaw: j.opts.SaveRaw})
	if err != nil {
		return err
	}

	var outletRepo outlet.Repository = sheetRepo
	if j.opts.DryRun {
		outletRepo = outletRepository.NewCSVRepository(j.opts.CSVPath)
	}

	outletUC := outletUsecase.NewOutletUsecase(brandRepo, places, outletRepo, geo.NewH3Indexer(), j.cfg.Outlet.MergeRadiusM)
	handler := outletController.NewOutletController(outletUC, j.opts.DryRun)

	bbox := j.cfg.Places.BBox
	if j.opts.BBox != "" {
		bbox = j.opts.BBox
	}
	return handler.RefreshOutlets(ctx, bbox)
}
