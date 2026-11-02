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

// The parse stage's columns of a raw_items row, sent to upsert_raw_items. Every field is always
// sent, so a parse that works after a failed one clears the old error.
type RawItemStatus struct {
	ID       string   `json:"id"`
	SourceID string   `json:"source_id"`
	Status   string   `json:"status"` // parsed (promos found), skipped (none) or failed
	Attempts int      `json:"attempts"`
	Error    string   `json:"error"`
	PromoIDs []string `json:"promo_ids"`
}

// A promo the job writes to the promos tab (source = web is set by the sheet).
type Promo struct {
	ID             string   `json:"id"` // promo_ + hash of DedupeKey
	BrandID        string   `json:"brand_id"`
	BrandText      string   `json:"brand_text"`
	Title          string   `json:"title"`
	Deal           string   `json:"deal"`
	Channel        string   `json:"channel"` // dine_in, take_away, delivery or all
	ValidFrom      string   `json:"valid_from"`
	ValidUntil     string   `json:"valid_until"`
	PostedAt       string   `json:"posted_at"`
	OutletIDs      []string `json:"outlet_ids"` // empty: every outlet of the brand
	Status         string   `json:"status"`     // active or hidden
	ActionURL      string   `json:"action_url"`
	SourceIDs      []string `json:"source_ids"`
	SourceRefs     []string `json:"source_refs"` // raw item ids it was read from
	DedupeKey      string   `json:"dedupe_key"`
	Confidence     float64  `json:"confidence"`
	Recurrence     string   `json:"recurrence"`      // "setiap Jumat"; empty for every day
	PaymentMethods []string `json:"payment_methods"` // e.g. "BCA credit card", "GoPay"
}

// What a parser gets for one raw item. Brand and Outlets are empty for an aggregator's item;
// KnownBrands then lists the tracked brands the item may be about.
type ParseRequest struct {
	Item        RawItem
	Today       string   // the run's date (YYYY-MM-DD, WIB), for "H+7" and "berlaku minggu ini"
	Brand       string   // the source's brand name and aliases, "HokBen (Hoka Hoka Bento)"
	Outlets     []string // that brand's outlet and mall names, sorted
	KnownBrands []string // sorted; aggregators only
}

// One promo as the parser read it, before brand and outlet ids are resolved.
type ParsedPromo struct {
	IsDeal         bool // false for menu launches and news, which are dropped
	BrandName      string
	Title          string
	Deal           string
	Channel        string // dine_in, take_away, delivery or all
	ValidFrom      string // YYYY-MM-DD or empty
	ValidUntil     string // YYYY-MM-DD or empty (no end date)
	Recurrence     string
	OutletLimits   []OutletLimit // empty: every outlet
	PaymentMethods []string
	Category       string // food, drink or non_food
	Confidence     float64
}

// A place a promo is limited to, as written in the post: a mall ("Margo City"), an outlet name,
// a city ("Bekasi") or a format ("HokBen+").
type OutletLimit struct {
	Kind string // mall, outlet, city or format
	Name string
}

// What a parser returns for one raw item.
type ParseResult struct {
	Promos          []ParsedPromo
	CostUSD         float64 // every call made for the item, retries included
	CacheReadTokens int64
}

// ParseError is a parser failure that belongs to the item (a reply that stays invalid, a refusal,
// a cut-off reply): the item is marked failed and retried on later runs. Any other parser error
// (auth, network, quota) stops the parse stage and leaves the items as they are.
type ParseError struct {
	Reason string
}

func (e *ParseError) Error() string { return e.Reason }

// what the parse stage did in one run
type ParseRunResult struct {
	Items           int // raw items parsed (calls made)
	Parsed          int // items with at least one promo
	Skipped         int // items with no promo
	Failed          int
	Left            int // items not parsed because the budget ran out; next run continues
	Promos          int // promos written (after merging)
	Hidden          int // of those, saved as hidden
	Dropped         int // read but not written: not a deal, or a brand that isn't tracked
	CostUSD         float64
	CacheReadTokens int64
	Written         WriteResult
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
	Parse    ParseRunResult
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
