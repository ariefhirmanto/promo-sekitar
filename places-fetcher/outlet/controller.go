package outlet

import "context"

type Controller interface {
	RefreshOutlets(ctx context.Context, bbox string) error
}
