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
