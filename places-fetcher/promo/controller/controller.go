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
	p := resp.Parse
	if p.Items == 0 && p.Failed == 0 && p.Left == 0 {
		return nil // parse stage off, or nothing to parse
	}
	log.Printf("[Promo][RunSources][Controller] parsed %d items: %d with promos, %d without, %d failed, %d left for the next run",
		p.Items, p.Parsed, p.Skipped, p.Failed, p.Left)
	log.Printf("[Promo][RunSources][Controller] promos: %d written (%d hidden), %d dropped; sheet %d added, %d updated, %d skipped (not web)",
		p.Promos, p.Hidden, p.Dropped, p.Written.Added, p.Written.Updated, p.Written.Skipped)
	log.Printf("[Promo][RunSources][Controller] AI spend $%.4f, cache read %d tokens", p.CostUSD, p.CacheReadTokens)
	return nil
}
