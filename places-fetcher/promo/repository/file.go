package repository

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"

	"places-fetcher/models"

	"github.com/pkg/errors"
)

// sourceFileRepository reads sources from a JSON file (the GET mode=sources shape), for runs without the sheet.
type sourceFileRepository struct {
	path string
}

func NewSourceFileRepository(path string) *sourceFileRepository {
	return &sourceFileRepository{path: path}
}

func (r *sourceFileRepository) GetSources(ctx context.Context) (resp []models.Source, err error) {
	b, err := os.ReadFile(r.path)
	if err != nil {
		log.Println("[Promo][GetSources][Repository] Problem reading sources file, err: ", err.Error())
		return resp, err
	}
	err = json.Unmarshal(b, &resp)
	return resp, errors.Wrap(err, "sourceFileRepository.GetSources.json.Unmarshal")
}

// noRefs knows no stored raw items: every item counts as new (runs with -sources-file).
type noRefs struct{}

func NewNoRefs() noRefs {
	return noRefs{}
}

func (noRefs) GetRawItemRefs(ctx context.Context) (resp map[string][]string, err error) {
	return map[string][]string{}, nil
}

// GetRawItems: there are no stored items, so only this run's items are parsed.
func (noRefs) GetRawItems(ctx context.Context, status string) (resp []models.RawItem, err error) {
	return nil, nil
}

// outletFileRepository reads outlets from a JSON file (the GET mode=outlets shape); no file means
// no outlets, so every promo limited to a place is saved hidden.
type outletFileRepository struct {
	path string
}

func NewOutletFileRepository(path string) *outletFileRepository {
	return &outletFileRepository{path: path}
}

func (r *outletFileRepository) GetOutlets(ctx context.Context) (resp []models.Outlet, err error) {
	if r.path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(r.path)
	if err != nil {
		log.Println("[Promo][GetOutlets][Repository] Problem reading outlets file, err: ", err.Error())
		return resp, err
	}
	err = json.Unmarshal(b, &resp)
	return resp, errors.Wrap(err, "outletFileRepository.GetOutlets.json.Unmarshal")
}

// logRawStatusWriter prints the raw item statuses a live run would write (dry-run mode).
type logRawStatusWriter struct{}

func NewLogRawStatusWriter() *logRawStatusWriter {
	return &logRawStatusWriter{}
}

func (logRawStatusWriter) UpdateRawItems(ctx context.Context, statuses []models.RawItemStatus) (resp models.WriteResult, err error) {
	for _, s := range statuses {
		log.Printf("[Promo][UpdateRawItems][Repository] would set %s: status=%s attempts=%d promo_ids=%q error=%q",
			s.ID, s.Status, s.Attempts, strings.Join(s.PromoIDs, ","), s.Error)
	}
	return models.WriteResult{Updated: len(statuses)}, nil
}

// promoCSVRepository writes promos to a CSV with the promos tab's fetcher columns (dry-run mode).
type promoCSVRepository struct {
	path string
}

func NewPromoCSVRepository(path string) *promoCSVRepository {
	return &promoCSVRepository{path: path}
}

func (r *promoCSVRepository) UpsertPromos(ctx context.Context, promos []models.Promo) (resp models.WriteResult, err error) {
	f, err := os.Create(r.path)
	if err != nil {
		log.Println("[Promo][UpsertPromos][Repository] Problem creating CSV, err: ", err.Error())
		return resp, err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.Write([]string{"id", "brand_id", "brand_text", "title", "deal", "channel", "valid_from", "valid_until",
		"posted_at", "outlet_ids", "status", "action_url", "source_ids", "source_refs", "dedupe_key", "confidence",
		"recurrence", "payment_methods"})
	for _, p := range promos {
		w.Write([]string{p.ID, p.BrandID, p.BrandText, p.Title, p.Deal, p.Channel, p.ValidFrom, p.ValidUntil,
			p.PostedAt, strings.Join(p.OutletIDs, ","), p.Status, p.ActionURL, strings.Join(p.SourceIDs, ","),
			strings.Join(p.SourceRefs, ","), p.DedupeKey, strconv.FormatFloat(p.Confidence, 'f', 2, 64),
			p.Recurrence, strings.Join(p.PaymentMethods, ",")})
	}
	w.Flush()
	if err = w.Error(); err != nil {
		return resp, err
	}

	log.Printf("[Promo][UpsertPromos][Repository] Wrote %d promos to %s", len(promos), r.path)
	return models.WriteResult{Added: len(promos)}, nil
}

// logStatusWriter prints the source statuses a live run would write (dry-run mode).
type logStatusWriter struct{}

func NewLogStatusWriter() *logStatusWriter {
	return &logStatusWriter{}
}

func (logStatusWriter) UpdateSources(ctx context.Context, statuses []models.SourceStatus) (resp models.WriteResult, err error) {
	for _, s := range statuses {
		log.Printf("[Promo][UpdateSources][Repository] would set %s: last_fetched_at=%q error_count=%d last_error=%q status=%q",
			s.ID, s.LastFetchedAt, s.ErrorCount, s.LastError, s.Status)
	}
	return models.WriteResult{Updated: len(statuses)}, nil
}

// rawItemCSVRepository writes raw items to a CSV with the raw_items tab's read columns (dry-run mode).
type rawItemCSVRepository struct {
	path string
}

func NewRawItemCSVRepository(path string) *rawItemCSVRepository {
	return &rawItemCSVRepository{path: path}
}

func (r *rawItemCSVRepository) UpsertRawItems(ctx context.Context, items []models.RawItem) (resp models.WriteResult, err error) {
	f, err := os.Create(r.path)
	if err != nil {
		log.Println("[Promo][UpsertRawItems][Repository] Problem creating CSV, err: ", err.Error())
		return resp, err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.Write([]string{"id", "source_id", "ref", "url", "posted_at", "text_chars", "text"})
	for _, it := range items {
		w.Write([]string{it.ID, it.SourceID, it.Ref, it.URL, it.PostedAt, strconv.Itoa(len([]rune(it.Text))), it.Text})
	}
	w.Flush()
	if err = w.Error(); err != nil {
		return resp, err
	}

	log.Printf("[Promo][UpsertRawItems][Repository] Wrote %d raw items to %s", len(items), r.path)
	return models.WriteResult{Added: len(items)}, nil
}

func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
