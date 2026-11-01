package promo

import "context"

type Controller interface {
	RunSources(ctx context.Context, sourceID string) error
}
