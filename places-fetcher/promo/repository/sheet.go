package repository

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/sheet"

	"github.com/pkg/errors"
)

// sheetRepository reads and writes the promo job's tabs through Code.gs (sources, raw_items, promos).
type sheetRepository struct {
	client *sheet.Client
}

func NewSheetRepository(cfg config.SheetConfig) *sheetRepository {
	return &sheetRepository{client: sheet.NewClient(cfg)}
}

func (r *sheetRepository) GetSources(ctx context.Context) (resp []models.Source, err error) {
	var body struct {
		Sources []models.Source `json:"sources"`
	}
	if err = r.client.Get(ctx, "sources", nil, &body); err != nil {
		log.Println("[Promo][GetSources][Repository] Problem reading sources from sheet, err: ", err.Error())
		return resp, errors.Wrap(hint(err), "sheetRepository.GetSources")
	}
	return body.Sources, nil
}

func (r *sheetRepository) UpdateSources(ctx context.Context, statuses []models.SourceStatus) (resp models.WriteResult, err error) {
	if err = r.client.Post(ctx, "update_sources", map[string]interface{}{"sources": statuses}, &resp); err != nil {
		log.Println("[Promo][UpdateSources][Repository] Problem writing source statuses, err: ", err.Error())
		return resp, errors.Wrap(hint(err), "sheetRepository.UpdateSources")
	}
	return resp, nil
}

func (r *sheetRepository) UpsertRawItems(ctx context.Context, items []models.RawItem) (resp models.WriteResult, err error) {
	if err = r.client.Post(ctx, "upsert_raw_items", map[string]interface{}{"raw_items": items}, &resp); err != nil {
		log.Println("[Promo][UpsertRawItems][Repository] Problem writing raw items, err: ", err.Error())
		return resp, errors.Wrap(hint(err), "sheetRepository.UpsertRawItems")
	}
	return resp, nil
}

func (r *sheetRepository) GetRawItemRefs(ctx context.Context) (resp map[string][]string, err error) {
	var body struct {
		Refs map[string][]string `json:"refs"`
	}
	if err = r.client.Get(ctx, "raw_item_refs", nil, &body); err != nil {
		log.Println("[Promo][GetRawItemRefs][Repository] Problem reading raw item refs, err: ", err.Error())
		return resp, errors.Wrap(hint(err), "sheetRepository.GetRawItemRefs")
	}
	return body.Refs, nil
}

func (r *sheetRepository) GetRawItems(ctx context.Context, status string) (resp []models.RawItem, err error) {
	var body struct {
		RawItems []struct {
			models.RawItem
			PromoIDs string `json:"promo_ids"` // the sheet stores them comma-separated
		} `json:"raw_items"`
	}
	if err = r.client.Get(ctx, "raw_items", url.Values{"status": {status}}, &body); err != nil {
		log.Println("[Promo][GetRawItems][Repository] Problem reading raw items, err: ", err.Error())
		return resp, errors.Wrap(hint(err), "sheetRepository.GetRawItems")
	}
	for _, it := range body.RawItems {
		item := it.RawItem
		item.PromoIDs = splitList(it.PromoIDs)
		resp = append(resp, item)
	}
	return resp, nil
}

// UpdateRawItems writes the parse stage's columns through the same upsert; the read columns stay.
func (r *sheetRepository) UpdateRawItems(ctx context.Context, statuses []models.RawItemStatus) (resp models.WriteResult, err error) {
	if err = r.client.Post(ctx, "upsert_raw_items", map[string]interface{}{"raw_items": statuses}, &resp); err != nil {
		log.Println("[Promo][UpdateRawItems][Repository] Problem writing raw item statuses, err: ", err.Error())
		return resp, errors.Wrap(hint(err), "sheetRepository.UpdateRawItems")
	}
	return resp, nil
}

func (r *sheetRepository) GetOutlets(ctx context.Context) (resp []models.Outlet, err error) {
	var body struct {
		Outlets []models.Outlet `json:"outlets"`
	}
	if err = r.client.Get(ctx, "outlets", nil, &body); err != nil {
		log.Println("[Promo][GetOutlets][Repository] Problem reading outlets, err: ", err.Error())
		return resp, errors.Wrap(hint(err), "sheetRepository.GetOutlets")
	}
	return body.Outlets, nil
}

func (r *sheetRepository) UpsertPromos(ctx context.Context, promos []models.Promo) (resp models.WriteResult, err error) {
	if err = r.client.Post(ctx, "upsert_promos", map[string]interface{}{"promos": promos}, &resp); err != nil {
		log.Println("[Promo][UpsertPromos][Repository] Problem writing promos, err: ", err.Error())
		return resp, errors.Wrap(hint(err), "sheetRepository.UpsertPromos")
	}
	return resp, nil
}

// hint says how to run without the sheet when it isn't configured.
func hint(err error) error {
	if err == sheet.ErrNotConfigured {
		return fmt.Errorf("%w (or use -dry-run with -sources-file)", err)
	}
	return err
}
