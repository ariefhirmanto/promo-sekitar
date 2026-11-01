package controller

import (
	"context"
	"log"
	"time"

	"places-fetcher/promo"
)

type controller struct {
	promoUC promo.Usecase
	now     func() time.Time
}

func NewPromoController(promoUC promo.Usecase) *controller {
	return &controller{promoUC: promoUC, now: time.Now}
}

// RunSources maps the CLI input to the usecase and reports the result in the job log.
func (c *controller) RunSources(ctx context.Context, sourceID string) error {
	resp, err := c.promoUC.RunSources(ctx, promo.InputRun{SourceID: sourceID, Now: c.now()})
	for _, s := range resp.Sources {
		switch {
		case s.Disabled:
			log.Printf("[Promo][RunSources][Controller]   %-26s FAILED, now status=error: %s", s.SourceID, s.Err)
		case s.Err != "":
			log.Printf("[Promo][RunSources][Controller]   %-26s FAILED: %s", s.SourceID, s.Err)
		default:
			log.Printf("[Promo][RunSources][Controller]   %-26s %d items", s.SourceID, s.Items)
		}
	}
	if err != nil {
		log.Println("[Promo][RunSources][Controller] Run failed, err: ", err.Error())
		return err
	}

	log.Printf("[Promo][RunSources][Controller] %d sources run, %d failed, %d items read",
		len(resp.Sources), resp.Failed(), resp.Fetched)
	log.Printf("[Promo][RunSources][Controller] raw items: %d added, %d updated, %d pruned; sources updated: %d",
		resp.RawItems.Added, resp.RawItems.Updated, resp.RawItems.Pruned, resp.Statuses.Updated)
	return nil
}
