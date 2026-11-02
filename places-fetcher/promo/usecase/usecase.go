package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"places-fetcher/models"
	"places-fetcher/promo"

	"github.com/pkg/errors"
)

const maxErrorLen = 500 // last_error is for a person reading the sheet

type usecase struct {
	sources      promo.SourceReader
	statuses     promo.SourceStatusWriter
	adapters     promo.SourceAdapters
	rawItems     promo.RawItemWriter
	knownRefs    promo.RawItemRefReader
	maxErrors    int
	fetchTimeout time.Duration
	parseDeps    *ParseDeps
}

// Promo Usecase constructor; adapters is the source-type registry, or a replay file. parse nil
// runs the fetch stage only.
func NewPromoUsecase(sources promo.SourceReader, statuses promo.SourceStatusWriter, adapters promo.SourceAdapters,
	rawItems promo.RawItemWriter, knownRefs promo.RawItemRefReader, maxErrors int, fetchTimeout time.Duration,
	parse *ParseDeps) *usecase {
	if maxErrors <= 0 {
		maxErrors = 3
	}
	if fetchTimeout <= 0 {
		fetchTimeout = 2 * time.Minute
	}
	if parse != nil {
		if parse.Retries < 0 {
			parse.Retries = 0
		}
		if parse.HideBelow <= 0 {
			parse.HideBelow = 0.6
		}
	}
	return &usecase{sources: sources, statuses: statuses, adapters: adapters, rawItems: rawItems,
		knownRefs: knownRefs, maxErrors: maxErrors, fetchTimeout: fetchTimeout, parseDeps: parse}
}

// RunSources fetches every due source, stores what it read as raw items, writes each source's
// status back, then parses the new items into promos. One source or item failing never stops the
// others; the run fails only when the sheet can't be read or written, every due source failed,
// or the parser can't work at all.
func (u *usecase) RunSources(ctx context.Context, input promo.InputRun) (resp models.PromoRunResult, err error) {
	err = input.ValidateInput()
	if err != nil {
		log.Println("[Promo][RunSources][Usecase] Invalid input, err: ", err.Error())
		return resp, errors.WithMessage(err, "promoUC.RunSources.ValidateInput")
	}

	sources, err := u.sources.GetSources(ctx)
	if err != nil {
		log.Println("[Promo][RunSources][Usecase] Problem getting sources, err: ", err.Error())
		return resp, errors.WithMessage(err, "promoUC.RunSources.GetSources")
	}

	due, err := DueSources(sources, input.SourceID, input.Now)
	if err != nil {
		return resp, errors.WithMessage(err, "promoUC.RunSources.DueSources")
	}

	var items []models.RawItem
	if len(due) == 0 {
		log.Println("[Promo][RunSources][Usecase] No source is due")
	} else {
		resp, items, err = u.fetchSources(ctx, due, input)
		if err != nil {
			return resp, err
		}
	}

	if u.parseDeps != nil {
		resp.Parse, err = u.parse(ctx, sources, items, input.Now.In(models.WIB).Format("2006-01-02"))
		if err != nil {
			return resp, errors.WithMessage(err, "promoUC.RunSources")
		}
	}

	if len(due) > 0 && resp.Failed() == len(due) {
		return resp, fmt.Errorf("promoUC.RunSources: all %d due sources failed", len(due))
	}
	return resp, nil
}

// fetchSources runs the fetch stage: every due source's adapter, then the raw items and the
// source statuses are written.
func (u *usecase) fetchSources(ctx context.Context, due []models.Source, input promo.InputRun) (resp models.PromoRunResult, items []models.RawItem, err error) {
	refs, err := u.knownRefs.GetRawItemRefs(ctx)
	if err != nil {
		log.Println("[Promo][RunSources][Usecase] Problem getting known refs, err: ", err.Error())
		return resp, nil, errors.WithMessage(err, "promoUC.RunSources.GetRawItemRefs")
	}

	now := input.Now.In(models.WIB).Format(models.TimeLayout)
	var statuses []models.SourceStatus
	for _, src := range due {
		got, ferr := u.fetch(ctx, models.FetchRequest{Source: src, KnownRefs: toSet(refs[src.ID])})
		run := models.SourceRun{SourceID: src.ID, Type: src.Type, Items: len(got.Items)}
		status := models.SourceStatus{ID: src.ID}
		if ferr != nil {
			log.Printf("[Promo][RunSources][Usecase] Source %s failed, err: %s", src.ID, ferr.Error())
			run.Err = ferr.Error()
			status.ErrorCount = src.ErrorCount + 1
			status.LastError = truncate(ferr.Error(), maxErrorLen)
			if status.ErrorCount >= u.maxErrors && src.Status == "active" {
				status.Status = "error"
				run.Disabled = true
			}
		} else {
			items = append(items, got.Items...)
			status.LastFetchedAt = now
			status.LastSeenRef = got.LastSeenRef
		}
		resp.Sources = append(resp.Sources, run)
		statuses = append(statuses, status)
	}
	resp.Fetched = len(items)

	// Raw items first: if they can't be stored, last_fetched_at stays old and the next run fetches again.
	if len(items) > 0 {
		resp.RawItems, err = u.rawItems.UpsertRawItems(ctx, items)
		if err != nil {
			log.Println("[Promo][RunSources][Usecase] Problem upserting raw items, err: ", err.Error())
			return resp, nil, errors.WithMessage(err, "promoUC.RunSources.UpsertRawItems")
		}
	}

	resp.Statuses, err = u.statuses.UpdateSources(ctx, statuses)
	if err != nil {
		log.Println("[Promo][RunSources][Usecase] Problem updating source statuses, err: ", err.Error())
		return resp, nil, errors.WithMessage(err, "promoUC.RunSources.UpdateSources")
	}
	return resp, items, nil
}

// fetch runs one source's adapter with its own timeout and checks what came back. Items whose ref
// is already stored are dropped even if the adapter returned them, so a re-run never resends them.
func (u *usecase) fetch(ctx context.Context, req models.FetchRequest) (resp models.FetchResult, err error) {
	src := req.Source
	adapter, ok := u.adapters.ForType(src.Type)
	if !ok {
		return resp, fmt.Errorf("unknown source type %q", src.Type)
	}

	ctx, cancel := context.WithTimeout(ctx, u.fetchTimeout)
	defer cancel()
	resp, err = adapter.FetchItems(ctx, req)
	if err != nil {
		return resp, err
	}

	seen := map[string]bool{}
	items := make([]models.RawItem, 0, len(resp.Items))
	for _, it := range resp.Items {
		if it.Ref == "" {
			return resp, fmt.Errorf("adapter %s returned an item without a ref (%s)", adapter.Type(), it.URL)
		}
		if seen[it.Ref] || req.KnownRefs[it.Ref] {
			continue
		}
		seen[it.Ref] = true
		it.SourceID = src.ID
		it.ID = models.RawItemID(src.ID, it.Ref)
		items = append(items, it)
	}
	resp.Items = items
	return resp, nil
}

func toSet(refs []string) map[string]bool {
	set := make(map[string]bool, len(refs))
	for _, r := range refs {
		set[r] = true
	}
	return set
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
