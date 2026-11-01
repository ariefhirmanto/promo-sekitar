package repository

import (
	"context"
	"fmt"
	"log"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/sheet"

	"github.com/pkg/errors"
)

// sheetRepository talks to the Apps Script web app bound to the makandimana spreadsheet (Code.gs).
type sheetRepository struct {
	client *sheet.Client
}

func NewSheetRepository(cfg config.SheetConfig) *sheetRepository {
	return &sheetRepository{client: sheet.NewClient(cfg)}
}

func (r *sheetRepository) GetBrands(ctx context.Context) (resp []models.Brand, err error) {
	var body struct {
		Brands []models.Brand `json:"brands"`
	}
	if err = r.client.Get(ctx, "brands", nil, &body); err != nil {
		log.Println("[Outlet][GetBrands][Repository] Problem reading brands from sheet, err: ", err.Error())
		return resp, errors.Wrap(hint(err), "sheetRepository.GetBrands")
	}

	return body.Brands, nil
}

// UpsertOutlets replaces this source's rows in the outlets tab.
func (r *sheetRepository) UpsertOutlets(ctx context.Context, source string, outlets []models.Outlet) (resp models.UpsertResult, err error) {
	payload := map[string]interface{}{"source": source, "outlets": outlets}
	if err = r.client.Post(ctx, "upsert_outlets", payload, &resp); err != nil {
		log.Println("[Outlet][UpsertOutlets][Repository] Problem writing outlets to sheet, err: ", err.Error())
		return resp, errors.Wrap(hint(err), "sheetRepository.UpsertOutlets")
	}

	return resp, nil
}

// hint says how to run without the sheet when it isn't configured.
func hint(err error) error {
	if err == sheet.ErrNotConfigured {
		return fmt.Errorf("%w (or use -dry-run with -brands-file)", err)
	}
	return err
}
