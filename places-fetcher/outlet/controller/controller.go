package controller

import (
	"context"
	"log"

	"places-fetcher/outlet"
)

type controller struct {
	outletUC outlet.Usecase
	verbose  bool // print every outlet (dry-run)
}

func NewOutletController(outletUC outlet.Usecase, verbose bool) *controller {
	return &controller{outletUC: outletUC, verbose: verbose}
}

// RefreshOutlets maps the CLI input to the usecase and reports the result in the job log.
func (c *controller) RefreshOutlets(ctx context.Context, bbox string) error {
	resp, err := c.outletUC.RefreshOutlets(ctx, outlet.InputRefresh{BBox: bbox})
	if err != nil {
		log.Println("[Outlet][RefreshOutlets][Controller] Refresh failed, err: ", err.Error())
		return err
	}

	log.Printf("[Outlet][RefreshOutlets][Controller] %d places, %d malls -> %d outlets", resp.Places, resp.Malls, len(resp.Outlets))
	for brandID, n := range resp.PerBrand {
		log.Printf("[Outlet][RefreshOutlets][Controller]   %-20s %d", brandID, n)
	}
	inMall := 0
	for _, o := range resp.Outlets {
		if o.MallName != "" {
			inMall++
		}
		if c.verbose {
			log.Printf("%s\t%s\t%s\t%.6f,%.6f\t%s\t%s", o.ID, o.Name, o.Format, o.Lat, o.Lng, o.H3R8, o.MallName)
		}
	}
	log.Printf("[Outlet][RefreshOutlets][Controller] %d outlets have a mall_name", inMall)
	log.Printf("[Outlet][RefreshOutlets][Controller] upsert: %d added, %d updated, %d closed", resp.Upsert.Added, resp.Upsert.Updated, resp.Upsert.Closed)

	return nil
}
