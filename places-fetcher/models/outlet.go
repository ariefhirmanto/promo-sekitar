package models

// base model, one row of the outlets tab
type Outlet struct {
	ID       string  `json:"id"`
	BrandID  string  `json:"brand_id"`
	Name     string  `json:"name"`
	Address  string  `json:"address"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	H3R8     string  `json:"h3_r8"`
	H3R6     string  `json:"h3_r6"`
	Format   string  `json:"format"`
	MallName string  `json:"mall_name"`
	Source   string  `json:"source"`
}

// response of the sheet upsert
type UpsertResult struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Closed  int `json:"closed"`
}

// response of one refresh run
type RefreshResult struct {
	Places   int            // OSM food places returned
	Malls    int            // OSM mall outlines returned
	Outlets  []Outlet       // after brand matching and merging
	PerBrand map[string]int // brand id → outlet count
	Upsert   UpsertResult
}
