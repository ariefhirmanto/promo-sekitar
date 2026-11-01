package promo

import (
	"context"
	"places-fetcher/models"
)

// SourceReader reads the sources tab (sheet, or a JSON file for testing).
type SourceReader interface {
	GetSources(ctx context.Context) (resp []models.Source, err error)
}

// SourceStatusWriter writes the fetcher's columns of the sources tab (sheet, or the log in dry-run mode).
type SourceStatusWriter interface {
	UpdateSources(ctx context.Context, statuses []models.SourceStatus) (resp models.WriteResult, err error)
}

// RawItemWriter stores what was read (sheet, or a CSV file in dry-run mode).
type RawItemWriter interface {
	UpsertRawItems(ctx context.Context, items []models.RawItem) (resp models.WriteResult, err error)
}

// RawItemReader reads raw items by status, e.g. "new" and "failed" for the parse stage.
type RawItemReader interface {
	GetRawItems(ctx context.Context, status string) (resp []models.RawItem, err error)
}

// PromoRepository stores promos; only rows with source = web are ever changed.
type PromoRepository interface {
	UpsertPromos(ctx context.Context, promos []models.Promo) (resp models.WriteResult, err error)
}

// SourceAdapter is the port every source type implements (promo/source/<type>). One adapter
// reads every source row of its type; FetchItems gets one row and returns its items.
type SourceAdapter interface {
	Type() string // matches the sources tab's type column, e.g. "web_page"
	FetchItems(ctx context.Context, src models.Source) (resp models.FetchResult, err error)
}

// SourceAdapters picks the adapter for a source's type (the registry, or a replay file).
type SourceAdapters interface {
	ForType(sourceType string) (SourceAdapter, bool)
}
