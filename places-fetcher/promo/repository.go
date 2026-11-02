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

// RawItemRefReader lists the refs already stored per source id, so adapters skip opening them.
type RawItemRefReader interface {
	GetRawItemRefs(ctx context.Context) (resp map[string][]string, err error)
}

// RawItemReader reads raw items by status, e.g. "new" and "failed" for the parse stage.
type RawItemReader interface {
	GetRawItems(ctx context.Context, status string) (resp []models.RawItem, err error)
}

// RawItemStatusWriter writes the parse stage's columns of raw items (sheet, or the log in dry-run mode).
type RawItemStatusWriter interface {
	UpdateRawItems(ctx context.Context, statuses []models.RawItemStatus) (resp models.WriteResult, err error)
}

// BrandReader reads the brands tab (sheet, or a JSON file), to resolve the brand a promo names.
type BrandReader interface {
	GetBrands(ctx context.Context) (resp []models.Brand, err error)
}

// OutletReader reads open outlets, to turn "Margo City" into out_* ids.
type OutletReader interface {
	GetOutlets(ctx context.Context) (resp []models.Outlet, err error)
}

// Parser reads promos out of one raw item (promo/parser/<model>). An item-level failure is a
// *models.ParseError; any other error means the parser can't work this run.
type Parser interface {
	Parse(ctx context.Context, req models.ParseRequest) (resp models.ParseResult, err error)
}

// PromoRepository stores promos; only rows with source = web are ever changed.
type PromoRepository interface {
	UpsertPromos(ctx context.Context, promos []models.Promo) (resp models.WriteResult, err error)
}

// SourceAdapter is the port every source type implements (promo/source/<type>). One adapter
// reads every source row of its type; FetchItems gets one row and returns its new items.
type SourceAdapter interface {
	Type() string // matches the sources tab's type column, e.g. "web_page"
	FetchItems(ctx context.Context, req models.FetchRequest) (resp models.FetchResult, err error)
}

// SourceAdapters picks the adapter for a source's type (the registry, or a replay file).
type SourceAdapters interface {
	ForType(sourceType string) (SourceAdapter, bool)
}
