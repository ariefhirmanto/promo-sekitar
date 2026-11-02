package promo

import (
	"context"
	"places-fetcher/models"
)

type Usecase interface {
	RunSources(ctx context.Context, input InputRun) (resp models.PromoRunResult, err error)
}
