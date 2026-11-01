package repository

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"log"
	"os"
	"strconv"

	"places-fetcher/models"

	"github.com/pkg/errors"
)

// brandFileRepository reads brands from a JSON file ([{id,name,aliases}]), for runs without the sheet.
type brandFileRepository struct {
	path string
}

func NewBrandFileRepository(path string) *brandFileRepository {
	return &brandFileRepository{path: path}
}

func (r *brandFileRepository) GetBrands(ctx context.Context) (resp []models.Brand, err error) {
	b, err := os.ReadFile(r.path)
	if err != nil {
		log.Println("[Outlet][GetBrands][Repository] Problem reading brands file, err: ", err.Error())
		return resp, err
	}
	err = json.Unmarshal(b, &resp)
	return resp, errors.Wrap(err, "brandFileRepository.GetBrands.json.Unmarshal")
}

// csvRepository writes outlets to a CSV with the outlets tab's columns (dry-run mode).
type csvRepository struct {
	path string
}

func NewCSVRepository(path string) *csvRepository {
	return &csvRepository{path: path}
}

func (r *csvRepository) UpsertOutlets(ctx context.Context, source string, outlets []models.Outlet) (resp models.UpsertResult, err error) {
	f, err := os.Create(r.path)
	if err != nil {
		log.Println("[Outlet][UpsertOutlets][Repository] Problem creating CSV, err: ", err.Error())
		return resp, err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.Write([]string{"id", "brand_id", "name", "address", "lat", "lng", "h3_r8", "h3_r6", "format", "mall_name", "source", "status"})
	for _, o := range outlets {
		w.Write([]string{o.ID, o.BrandID, o.Name, o.Address,
			strconv.FormatFloat(o.Lat, 'f', 6, 64), strconv.FormatFloat(o.Lng, 'f', 6, 64),
			o.H3R8, o.H3R6, o.Format, o.MallName, source, "open"})
	}
	w.Flush()
	if err = w.Error(); err != nil {
		return resp, err
	}

	log.Printf("[Outlet][UpsertOutlets][Repository] Wrote %d outlets to %s", len(outlets), r.path)
	return models.UpsertResult{Added: len(outlets)}, nil
}
