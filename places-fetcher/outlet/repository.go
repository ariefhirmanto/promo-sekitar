package outlet

import (
	"context"
	"places-fetcher/models"
)

// BrandRepository reads the brands to look for (sheet, or a JSON file for testing).
type BrandRepository interface {
	GetBrands(ctx context.Context) (resp []models.Brand, err error)
}

// PlaceProvider is the port every places vendor adapter implements (outlet/provider/<vendor>).
// Switching vendor means adding an adapter and changing Places.Provider in config.yaml.
type PlaceProvider interface {
	// Source names the data the adapter returns (e.g. "osm"); upserts only replace that source's rows.
	Source() string
	SearchPlaces(ctx context.Context, query models.PlaceQuery) (resp models.PlaceResult, err error)
}

// Repository stores outlets (sheet, or a CSV file in dry-run mode).
type Repository interface {
	UpsertOutlets(ctx context.Context, source string, outlets []models.Outlet) (resp models.UpsertResult, err error)
}

// GeoIndexer computes the H3 cells stored with each outlet.
type GeoIndexer interface {
	Cells(lat, lng float64) (r8, r6 string)
}
