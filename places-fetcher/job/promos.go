package job

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"time"

	"places-fetcher/config"
	outletRepository "places-fetcher/outlet/repository"
	"places-fetcher/promo"
	promoController "places-fetcher/promo/controller"
	"places-fetcher/promo/parser"
	promoRepository "places-fetcher/promo/repository"
	"places-fetcher/promo/source"
	"places-fetcher/promo/source/replay"
	promoUsecase "places-fetcher/promo/usecase"

	// Parsers and source type adapters register themselves; add new ones here.
	_ "places-fetcher/promo/parser/claude"
	_ "places-fetcher/promo/source/webpage"
)

// PromoOptions come from the command line of app/promos; they pick the repositories and adapters.
type PromoOptions struct {
	DryRun      bool   // print and write CSV instead of the sheet
	RawCSVPath  string // raw items CSV in dry-run mode
	SourceID    string // run only this source
	ItemsFile   string // replay saved raw items instead of fetching
	SourcesFile string // read sources from a JSON file instead of the sheet
	Parse       bool   // run the parse stage after fetching
	Parser      string // parser name, overriding Parser.Name in config.yaml
	PromoCSV    string // promos CSV in dry-run mode
	BrandsFile  string // with -sources-file: brands JSON (the GET mode=brands shape)
	OutletsFile string // with -sources-file: outlets JSON (the GET mode=outlets shape); none = no outlets
}

type PromoJob struct {
	cfg  *config.MainConfig
	opts PromoOptions
}

func NewPromoJob(cfg *config.MainConfig, opts PromoOptions) *PromoJob {
	return &PromoJob{cfg: cfg, opts: opts}
}

func (j *PromoJob) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	sheetRepo := promoRepository.NewSheetRepository(j.cfg.Sheet)

	var sources promo.SourceReader = sheetRepo
	var knownRefs promo.RawItemRefReader = sheetRepo
	if j.opts.SourcesFile != "" {
		sources = promoRepository.NewSourceFileRepository(j.opts.SourcesFile)
		knownRefs = promoRepository.NewNoRefs()
	}

	var adapters promo.SourceAdapters
	if j.opts.ItemsFile != "" {
		a, err := replay.New(j.opts.ItemsFile)
		if err != nil {
			return err
		}
		adapters = a
	} else {
		set, err := source.NewSet(*j.cfg)
		if err != nil {
			return err
		}
		adapters = set
	}

	var statuses promo.SourceStatusWriter = sheetRepo
	var rawItems promo.RawItemWriter = sheetRepo
	if j.opts.DryRun {
		statuses = promoRepository.NewLogStatusWriter()
		rawItems = promoRepository.NewRawItemCSVRepository(j.opts.RawCSVPath)
	}

	parse, err := j.parseDeps(sheetRepo)
	if err != nil {
		return err
	}

	promoUC := promoUsecase.NewPromoUsecase(sources, statuses, adapters, rawItems, knownRefs,
		j.cfg.Promo.MaxErrors, time.Duration(j.cfg.Promo.FetchTimeoutSeconds)*time.Second, parse)
	handler := promoController.NewPromoController(promoUC)

	return handler.RunSources(ctx, j.opts.SourceID)
}

// parseDeps picks the parse stage's parser and stores; nil when -parse=false.
func (j *PromoJob) parseDeps(sheetRepo promo.PromoRepository) (*promoUsecase.ParseDeps, error) {
	if !j.opts.Parse {
		return nil, nil
	}
	name := j.cfg.Parser.Name
	if j.opts.Parser != "" {
		name = j.opts.Parser
	}
	p, err := parser.New(name, *j.cfg)
	if err != nil {
		return nil, err
	}

	repo := promoRepository.NewSheetRepository(j.cfg.Sheet)
	d := &promoUsecase.ParseDeps{
		Parser: p, Stored: repo, Statuses: repo, Brands: outletRepository.NewSheetRepository(j.cfg.Sheet),
		Outlets: repo, Promos: sheetRepo,
		Retries: j.cfg.Parser.Retries, BudgetUSD: j.cfg.Parser.BudgetUSD, HideBelow: j.cfg.Parser.HideBelow,
	}
	if j.opts.SourcesFile != "" {
		if j.opts.BrandsFile == "" {
			return nil, errors.New("-sources-file needs -brands-file to parse (or -parse=false)")
		}
		d.Stored = promoRepository.NewNoRefs()
		d.Brands = outletRepository.NewBrandFileRepository(j.opts.BrandsFile)
		d.Outlets = promoRepository.NewOutletFileRepository(j.opts.OutletsFile)
	}
	if j.opts.DryRun {
		d.Statuses = promoRepository.NewLogRawStatusWriter()
		d.Promos = promoRepository.NewPromoCSVRepository(j.opts.PromoCSV)
	}
	return d, nil
}
