package job

import (
	"context"
	"os"
	"os/signal"
	"time"

	"places-fetcher/config"
	"places-fetcher/promo"
	promoController "places-fetcher/promo/controller"
	promoRepository "places-fetcher/promo/repository"
	"places-fetcher/promo/source"
	"places-fetcher/promo/source/replay"
	promoUsecase "places-fetcher/promo/usecase"
	// Source type adapters register themselves; add new ones here (M3: webpage).
)

// PromoOptions come from the command line of app/promos; they pick the repositories and adapters.
type PromoOptions struct {
	DryRun      bool   // print and write CSV instead of the sheet
	RawCSVPath  string // raw items CSV in dry-run mode
	SourceID    string // run only this source
	ItemsFile   string // replay saved raw items instead of fetching
	SourcesFile string // read sources from a JSON file instead of the sheet
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
	if j.opts.SourcesFile != "" {
		sources = promoRepository.NewSourceFileRepository(j.opts.SourcesFile)
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

	promoUC := promoUsecase.NewPromoUsecase(sources, statuses, adapters, rawItems,
		j.cfg.Promo.MaxErrors, time.Duration(j.cfg.Promo.FetchTimeoutSeconds)*time.Second)
	handler := promoController.NewPromoController(promoUC)

	return handler.RunSources(ctx, j.opts.SourceID)
}
