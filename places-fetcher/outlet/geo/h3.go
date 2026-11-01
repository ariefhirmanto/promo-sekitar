// Package geo wraps uber/h3-go, which needs cgo; keeping it here lets the usecase and its
// tests build without a C compiler.
package geo

import "github.com/uber/h3-go/v4"

type h3Indexer struct{}

func NewH3Indexer() *h3Indexer {
	return &h3Indexer{}
}

// Cells returns the resolution 8 cell (nearby search) and resolution 6 cell, as hex strings.
func (h *h3Indexer) Cells(lat, lng float64) (r8, r6 string) {
	ll := h3.NewLatLng(lat, lng)
	return h3.LatLngToCell(ll, 8).String(), h3.LatLngToCell(ll, 6).String()
}
