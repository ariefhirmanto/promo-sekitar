package models

// Vendor-neutral place models. Every provider adapter (outlet/provider/...) transforms its own
// response into these, so the usecase never sees vendor fields like OSM tags.

type LatLng struct {
	Lat float64
	Lng float64
}

// request to a provider
type PlaceQuery struct {
	Brands []Brand
	BBox   string // south,west,north,east
}

// A food place found by a provider.
type Place struct {
	Source    string // vendor that found it, e.g. "osm"; also the sheet's source column
	SourceRef string // the vendor's stable id, e.g. "n123"; outlet id = out_{Source}_{SourceRef}
	Name      string
	AltNames  []string // brand label, other-language or official names, also used for matching
	Address   string
	MallHints []string // address parts that may name a mall (building name, place)
	Lat       float64
	Lng       float64
}

// A mall outline found by a provider; it can have several outer rings.
type Mall struct {
	Source    string
	SourceRef string
	Name      string
	Rings     [][]LatLng
}

// response of a provider
type PlaceResult struct {
	Places []Place
	Malls  []Mall
}
