package models

import "time"

// WIB is the sheet's time zone (UTC+7 all year); every timestamp the promo job writes uses it.
var WIB = time.FixedZone("WIB", 7*3600)

// TimeLayout is how timestamps are written to the sheet, e.g. 2026-11-02T06:01:00+07:00.
const TimeLayout = time.RFC3339

// base model, one row of the sources tab
type Source struct {
	ID              string `json:"id"`
	Type            string `json:"type"`   // which source adapter reads it, e.g. "web_page"
	Target          string `json:"target"` // page URL (or handle, for later types)
	BrandID         string `json:"brand_id"`
	IsAggregator    bool   `json:"is_aggregator"` // items cover many brands; the parser names the brand
	FetchEveryHours int    `json:"fetch_every_hours"`
	Status          string `json:"status"` // active, paused (you) or error (the job, after repeated failures)
	LastFetchedAt   string `json:"last_fetched_at"`
	LastSeenRef     string `json:"last_seen_ref"`
	ErrorCount      int    `json:"error_count"`
	LastError       string `json:"last_error"`
	Notes           string `json:"notes"`
}

// The fetcher's columns of a sources row, sent to update_sources. ErrorCount and LastError are
// always sent so a success clears them; empty LastFetchedAt / LastSeenRef / Status keep the cell.
type SourceStatus struct {
	ID            string `json:"id"`
	LastFetchedAt string `json:"last_fetched_at,omitempty"`
	LastSeenRef   string `json:"last_seen_ref,omitempty"`
	ErrorCount    int    `json:"error_count"`
	LastError     string `json:"last_error"`
	Status        string `json:"status,omitempty"` // only "error"; the sheet ignores it unless the row is active
}

// One post or page read from a source, a row of the raw_items tab. The fetch stage sends only the
// read fields; status, attempts, error and promo_ids belong to the parse stage and are omitted when
// empty, so re-sending a seen item never resets its status.
type RawItem struct {
	ID        string   `json:"id"` // RawItemID(source_id, ref)
	SourceID  string   `json:"source_id"`
	Ref       string   `json:"ref"` // the source's own stable id for the item (post id, deal slug, text hash)
	URL       string   `json:"url"`
	PostedAt  string   `json:"posted_at,omitempty"`
	Text      string   `json:"text"`
	Status    string   `json:"status,omitempty"` // new, parsed, skipped or failed
	Attempts  int      `json:"attempts,omitempty"`
	Error     string   `json:"error,omitempty"`
	PromoIDs  []string `json:"promo_ids,omitempty"`
	FetchedAt string   `json:"fetched_at,omitempty"`
}

// RawItemID is the raw_items id: the source id and the item's ref, e.g. src_ramenya_katalog:691830.
func RawItemID(sourceID, ref string) string {
	return sourceID + ":" + ref
}

// What a source adapter gets for one source.
type FetchRequest struct {
	Source    Source
	KnownRefs map[string]bool // refs already in raw_items: list them if cheap, but don't open their pages
}

// What a source adapter returns for one source.
type FetchResult struct {
	Items       []RawItem // ID and SourceID are filled in by the usecase
	LastSeenRef string    // newest ref, for adapters that page by it; empty keeps the old value
}

// A promo the job writes to the promos tab (source = web is set by the sheet).
type Promo struct {
	ID         string   `json:"id"` // promo_ + hash of DedupeKey
	BrandID    string   `json:"brand_id"`
	BrandText  string   `json:"brand_text"`
	Title      string   `json:"title"`
	Deal       string   `json:"deal"`
	Channel    string   `json:"channel"` // dine_in, take_away, delivery or all
	ValidFrom  string   `json:"valid_from"`
	ValidUntil string   `json:"valid_until"`
	PostedAt   string   `json:"posted_at"`
	OutletIDs  []string `json:"outlet_ids"`
	Status     string   `json:"status"` // active or hidden
	ActionURL  string   `json:"action_url"`
	SourceIDs  []string `json:"source_ids"`
	SourceRefs []string `json:"source_refs"` // raw item ids it was read from
	DedupeKey  string   `json:"dedupe_key"`
	Confidence float64  `json:"confidence"`
}

// response of a sheet write that adds or updates rows by id
type WriteResult struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"` // update_sources: unknown ids; upsert_promos: rows that aren't source = web
	Pruned  int `json:"pruned"`  // upsert_raw_items: rows past 90 days removed
}

// what happened to one due source in a run
type SourceRun struct {
	SourceID string
	Type     string
	Items    int
	Err      string // empty when the fetch worked
	Disabled bool   // this failure moved the source to status error
}

// response of one promo run
type PromoRunResult struct {
	Sources  []SourceRun // due sources, in sheet order
	Fetched  int         // raw items read from all sources
	RawItems WriteResult
	Statuses WriteResult
}

// Failed counts the due sources whose fetch failed.
func (r PromoRunResult) Failed() int {
	n := 0
	for _, s := range r.Sources {
		if s.Err != "" {
			n++
		}
	}
	return n
}
