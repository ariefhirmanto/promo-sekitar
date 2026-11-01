// Package replay serves saved raw items instead of calling any source (-items-file). It stands in
// for every source type, so the rest of the pipeline runs on a file without network access.
//
// The file is a JSON array of models.RawItem; each item's source_id says which source returns it.
package replay

import (
	"context"
	"encoding/json"
	"os"

	"places-fetcher/models"
	"places-fetcher/promo"

	"github.com/pkg/errors"
)

const Name = "replay"

type adapter struct {
	bySource map[string][]models.RawItem
}

func New(path string) (*adapter, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "replay.New.ReadFile")
	}
	var items []models.RawItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, errors.Wrap(err, "replay.New.json.Unmarshal")
	}
	a := &adapter{bySource: map[string][]models.RawItem{}}
	for _, it := range items {
		if it.SourceID == "" || it.Ref == "" {
			return nil, errors.Errorf("replay.New: item %q needs source_id and ref", it.URL)
		}
		a.bySource[it.SourceID] = append(a.bySource[it.SourceID], it)
	}
	return a, nil
}

func (a *adapter) Type() string {
	return Name
}

// ForType returns the replay adapter for every type.
func (a *adapter) ForType(string) (promo.SourceAdapter, bool) {
	return a, true
}

// FetchItems returns the saved items of this source that aren't known yet; a source with none
// returns an empty result.
func (a *adapter) FetchItems(ctx context.Context, req models.FetchRequest) (resp models.FetchResult, err error) {
	for _, it := range a.bySource[req.Source.ID] {
		if !req.KnownRefs[it.Ref] {
			resp.Items = append(resp.Items, it)
		}
	}
	return resp, nil
}
