package usecase

import (
	"context"
	"log"

	"places-fetcher/models"
	"places-fetcher/outlet"

	"github.com/pkg/errors"
)

type usecase struct {
	brandRepo    outlet.BrandRepository
	places       outlet.PlaceProvider
	outletRepo   outlet.Repository
	geo          outlet.GeoIndexer
	mergeRadiusM float64
}

// Outlet Usecase constructor; places is whichever vendor adapter config picked.
func NewOutletUsecase(brandRepo outlet.BrandRepository, places outlet.PlaceProvider, outletRepo outlet.Repository, geo outlet.GeoIndexer, mergeRadiusM float64) *usecase {
	return &usecase{brandRepo: brandRepo, places: places, outletRepo: outletRepo, geo: geo, mergeRadiusM: mergeRadiusM}
}

func (u *usecase) RefreshOutlets(ctx context.Context, input outlet.InputRefresh) (resp models.RefreshResult, err error) {
	err = input.ValidateInput()
	if err != nil {
		log.Println("[Outlet][RefreshOutlets][Usecase] Invalid input, err: ", err.Error())
		return resp, errors.WithMessage(err, "outletUC.RefreshOutlets.ValidateInput")
	}

	brands, err := u.brandRepo.GetBrands(ctx)
	if err != nil {
		log.Println("[Outlet][RefreshOutlets][Usecase] Problem getting brands, err: ", err.Error())
		return resp, errors.WithMessage(err, "outletUC.RefreshOutlets.GetBrands")
	}
	if len(brands) == 0 {
		return resp, errors.New("outletUC.RefreshOutlets: no brands found; fill the brands tab first")
	}

	places, err := u.places.SearchPlaces(ctx, models.PlaceQuery{Brands: brands, BBox: input.BBox})
	if err != nil {
		log.Println("[Outlet][RefreshOutlets][Usecase] Problem searching places, err: ", err.Error())
		return resp, errors.WithMessage(err, "outletUC.RefreshOutlets.SearchPlaces")
	}

	resp.Places, resp.Malls = len(places.Places), len(places.Malls)
	resp.Outlets = BuildOutlets(places, brands, u.geo, u.mergeRadiusM)
	resp.PerBrand = map[string]int{}
	for _, o := range resp.Outlets {
		resp.PerBrand[o.BrandID]++
	}

	// An empty result is almost always an upstream problem; writing it would start closing real outlets.
	if len(resp.Outlets) == 0 {
		return resp, errors.New("outletUC.RefreshOutlets: no outlets matched; refusing to write")
	}

	resp.Upsert, err = u.outletRepo.UpsertOutlets(ctx, u.places.Source(), resp.Outlets)
	if err != nil {
		log.Println("[Outlet][RefreshOutlets][Usecase] Problem upserting outlets, err: ", err.Error())
		return resp, errors.WithMessage(err, "outletUC.RefreshOutlets.UpsertOutlets")
	}

	return resp, nil
}
