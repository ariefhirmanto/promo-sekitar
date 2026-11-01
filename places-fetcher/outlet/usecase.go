package outlet

import (
	"context"
	"places-fetcher/models"
)

type Usecase interface {
	RefreshOutlets(ctx context.Context, input InputRefresh) (resp models.RefreshResult, err error)
}
