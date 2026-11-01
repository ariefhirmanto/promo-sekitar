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
	maxErrors    int
	fetchTimeout time.Duration
}

// Promo Usecase constructor; adapters is the source-type registry, or a replay file.
func NewPromoUsecase(sources promo.SourceReader, statuses promo.SourceStatusWriter, adapters promo.SourceAdapters,
	rawItems promo.RawItemWriter, maxErrors int, fetchTimeout time.Duration) *usecase {
	if maxErrors <= 0 {
		maxErrors = 3
	}
	if fetchTimeout <= 0 {
		fetchTimeout = 2 * time.Minute
	}
	return &usecase{sources: sources, statuses: statuses, adapters: adapters, rawItems: rawItems,
		maxErrors: maxErrors, fetchTimeout: fetchTimeout}
}

// RunSources fetches every due source, stores what it read as raw items, and writes each source's
// status back. One source failing never stops the others; the run fails only when the sheet
// can't be read or written, or every due source failed.
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
	if len(due) == 0 {
		log.Println("[Promo][RunSources][Usecase] No source is due")
		return resp, nil
	}

	now := input.Now.In(models.WIB).Format(models.TimeLayout)
	var items []models.RawItem
	var statuses []models.SourceStatus
	for _, src := range due {
		got, ferr := u.fetch(ctx, src)
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
			return resp, errors.WithMessage(err, "promoUC.RunSources.UpsertRawItems")
		}
	}

	resp.Statuses, err = u.statuses.UpdateSources(ctx, statuses)
	if err != nil {
		log.Println("[Promo][RunSources][Usecase] Problem updating source statuses, err: ", err.Error())
		return resp, errors.WithMessage(err, "promoUC.RunSources.UpdateSources")
	}

	if resp.Failed() == len(due) {
		return resp, fmt.Errorf("promoUC.RunSources: all %d due sources failed", len(due))
	}
	return resp, nil
}

// fetch runs one source's adapter with its own timeout and checks what came back.
func (u *usecase) fetch(ctx context.Context, src models.Source) (resp models.FetchResult, err error) {
	adapter, ok := u.adapters.ForType(src.Type)
	if !ok {
		return resp, fmt.Errorf("unknown source type %q", src.Type)
	}

	ctx, cancel := context.WithTimeout(ctx, u.fetchTimeout)
	defer cancel()
	resp, err = adapter.FetchItems(ctx, src)
	if err != nil {
		return resp, err
	}

	seen := map[string]bool{}
	items := make([]models.RawItem, 0, len(resp.Items))
	for _, it := range resp.Items {
		if it.Ref == "" {
			return resp, fmt.Errorf("adapter %s returned an item without a ref (%s)", adapter.Type(), it.URL)
		}
		if seen[it.Ref] {
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

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
