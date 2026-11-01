// Package overpass is the places adapter for OpenStreetMap data through the Overpass API.
//
//	client.go     fetches a raw response (live servers with retries, or a saved file)
//	query.go      builds the Overpass QL query from the brands
//	transform.go  turns the OSM response into vendor-neutral models.Place / models.Mall
package overpass

import (
	"context"
	"log"
	"os"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/outlet"
	"places-fetcher/outlet/provider"

	"github.com/pkg/errors"
)

const (
	Name   = "overpass" // registry name, used in config.yaml
	source = "osm"      // the data is OpenStreetMap's, whichever OSM API fetched it
)

func init() {
	provider.Register(Name, func(cfg config.MainConfig, opts provider.Options) (outlet.PlaceProvider, error) {
		return New(cfg.Overpass, opts), nil
	})
}

type adapter struct {
	client  client
	saveRaw string
}

func New(cfg config.OverpassConfig, opts provider.Options) *adapter {
	var c client = newHTTPClient(cfg)
	if opts.File != "" {
		c = fileClient{path: opts.File}
	}
	return &adapter{client: c, saveRaw: opts.SaveRaw}
}

func (a *adapter) Source() string {
	return source
}

func (a *adapter) SearchPlaces(ctx context.Context, query models.PlaceQuery) (resp models.PlaceResult, err error) {
	raw, err := a.client.Fetch(ctx, BuildQuery(query.Brands, query.BBox))
	if err != nil {
		log.Println("[Outlet][SearchPlaces][Overpass] Problem fetching, err: ", err.Error())
		return resp, errors.WithMessage(err, "overpass.SearchPlaces.Fetch")
	}

	if a.saveRaw != "" {
		if err := os.WriteFile(a.saveRaw, raw, 0o644); err != nil {
			log.Println("[Outlet][SearchPlaces][Overpass] Problem saving raw response, err: ", err.Error())
		}
	}

	resp, err = Transform(raw)
	if err != nil {
		log.Println("[Outlet][SearchPlaces][Overpass] Problem transforming response, err: ", err.Error())
		return resp, errors.WithMessage(err, "overpass.SearchPlaces.Transform")
	}

	return resp, nil
}
