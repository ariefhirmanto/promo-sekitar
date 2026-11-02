// Package prompt is the vendor-neutral part of every promo parser: the instructions, the reply
// schema, the three request blocks, the checks on a reply, and the one-retry loop. A parser
// adapter (promo/parser/<name>) only sends the blocks to its service and returns the reply text.
//
// Request layout, in this order, so a provider with prompt caching can cache blocks 1 and 2:
//
//  1. Instructions   the same bytes for every item
//  2. Context(req)   the brand and its outlets, or the tracked brands; same for a brand's items
//  3. Item(req)      the item itself, with today's date; never cached
package prompt

import (
	"strings"

	"places-fetcher/models"

	"golang.org/x/text/unicode/norm"
)

// Instructions is block 1 of every request: the same bytes for every item, so a provider can
// cache it. Never put the date or anything per-run in it.
const Instructions = `You read Indonesian food and drink promo posts and pages and return the promos they announce, as JSON matching the given schema.

The item is one post or page from a promo source: a brand's own page, a bank's promo page, or an aggregator site that writes about many brands. It may contain several promos, one, or none. Return every distinct promo once. Return an empty list when the item announces no promo.

What is a deal (is_deal = true):
- a price cut, discount, cashback, voucher, bundle price, buy-one-get-one, free item with a purchase, member or payment-method deal, or a special price for a limited time.
What is not a deal (is_deal = false, still return it so it can be counted, with your best reading of the other fields):
- a new menu item or a menu at its normal price, an opening announcement without a deal, a contest or giveaway, a job post, news, or a recipe.

Fields:
- brand_name: the restaurant or drink brand the promo is for, as the item writes it ("HokBen", "Marugame Udon"). If a brand line is given below, use that brand unless the item is clearly about another one.
- title: a short name for the promo in the item's language, at most about 80 characters ("Paket Hemat Ramadan", "Diskon 30% Kartu Kredit BCA"). No emoji.
- deal: what the customer gets and pays, with the numbers: "Diskon 30% maks. Rp50.000, min. transaksi Rp150.000", "Beli 1 Gratis 1 Teh Susu", "Paket ayam + nasi + minum Rp25.000". Keep it under about 200 characters.
- channel: dine_in, take_away, delivery, or all. Use delivery for GoFood, GrabFood, ShopeeFood or the brand's own delivery; dine_in when the post says "makan di tempat" or only at the outlet; take_away for "bawa pulang"; all when it isn't limited or isn't said.
- valid_from, valid_until: dates as YYYY-MM-DD, or an empty string when not stated.
  - Read Indonesian dates: "1 - 30 November 2026", "s.d. 31 Des 2026", "periode 01/11/2026 - 15/11/2026" (day first).
  - A year left out is the year of the post date, or the next year if that date would already be over by more than a month.
  - Relative dates count from the post date: "hari ini" is the post date, "H+7" or "7 hari" ends 7 days after it, "minggu ini" ends on the Sunday of that week, "bulan ini" ends on the last day of that month.
  - "selama persediaan masih ada", "while stocks last", "berlaku terbatas" or no end date at all: valid_until is empty.
  - Never invent dates. If only the end date is stated, valid_from is empty.
- recurrence_text: a repeating day or time as written ("setiap Jumat", "Senin - Kamis 14.00-17.00", "tanggal kembar"), or empty.
- outlet_limits: places the promo is limited to, each with a kind: mall (a mall or shopping centre, "Margo City", "Grand Indonesia"), outlet (a named outlet that isn't in a mall), city ("Bekasi", "Jabodetabek"), or format (a sub-brand or store type such as "HokBen+"). Empty when it runs at all outlets or the item doesn't say. When an outlet list is given below, write the names as they appear there.
- payment_methods: payment methods the deal needs ("Kartu Kredit BCA", "GoPay", "QRIS", "ShopeePay"), empty if any payment works.
- category: food, drink, or non_food (merchandise, vouchers for other shops, anything not eaten or drunk).
- confidence: 0 to 1, how sure you are that this is a real, current promo and that the fields above are right. Use under 0.6 when the dates or the deal are guesses, the text is cut off, or the item may be an old post.

Rules:
- Use only what the item says. Do not add terms from general knowledge of the brand.
- A bank or aggregator page often lists many merchants: return only the food and drink promos, one per merchant and deal.
- The same promo repeated in a page (a summary and then the details) is one promo; merge what each part says.
- Write title and deal in the item's language; keep brand and place names as written.

Example 1. Item: "PROMO MIE CONTOH! Beli 2 mangkok Mie Ayam Spesial cuma Rp35.000 khusus makan di tempat. Berlaku 3 - 9 November 2026 di Mie Contoh Mal Contoh Raya dan Mie Contoh Bekasi." Post date 2026-11-02.
Reply: {"promos":[{"is_deal":true,"brand_name":"Mie Contoh","title":"Beli 2 Mie Ayam Spesial Rp35.000","deal":"2 mangkok Mie Ayam Spesial Rp35.000","channel":"dine_in","valid_from":"2026-11-03","valid_until":"2026-11-09","recurrence_text":"","outlet_limits":[{"kind":"mall","name":"Mal Contoh Raya"},{"kind":"city","name":"Bekasi"}],"payment_methods":[],"category":"food","confidence":0.95}]}

Example 2. Item: "Menu baru! Mie Contoh Pedas Level 5 sekarang hadir di semua outlet, harga Rp28.000." Post date 2026-11-02.
Reply: {"promos":[{"is_deal":false,"brand_name":"Mie Contoh","title":"Menu baru Mie Contoh Pedas Level 5","deal":"Rp28.000","channel":"all","valid_from":"","valid_until":"","recurrence_text":"","outlet_limits":[],"payment_methods":[],"category":"food","confidence":0.9}]}

Example 3. Item: "Setiap Jumat, cashback 20% maks. Rp20.000 bayar pakai GoPay di Mie Contoh via GoFood. Selama persediaan masih ada." Post date 2026-11-02.
Reply: {"promos":[{"is_deal":true,"brand_name":"Mie Contoh","title":"Cashback 20% GoPay Setiap Jumat","deal":"Cashback 20% maks. Rp20.000","channel":"delivery","valid_from":"","valid_until":"","recurrence_text":"setiap Jumat","outlet_limits":[],"payment_methods":["GoPay"],"category":"food","confidence":0.85}]}`

// Schema is the reply's JSON schema. Pass it to a provider that enforces one (Claude structured
// outputs need additionalProperties false and every property required); Read checks the ranges
// either way.
var Schema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"promos"},
	"properties": map[string]any{
		"promos": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required": []string{"is_deal", "brand_name", "title", "deal", "channel", "valid_from", "valid_until",
					"recurrence_text", "outlet_limits", "payment_methods", "category", "confidence"},
				"properties": map[string]any{
					"is_deal":         map[string]any{"type": "boolean"},
					"brand_name":      map[string]any{"type": "string"},
					"title":           map[string]any{"type": "string"},
					"deal":            map[string]any{"type": "string"},
					"channel":         map[string]any{"type": "string", "enum": Channels},
					"valid_from":      map[string]any{"type": "string", "description": "YYYY-MM-DD or empty"},
					"valid_until":     map[string]any{"type": "string", "description": "YYYY-MM-DD or empty"},
					"recurrence_text": map[string]any{"type": "string"},
					"outlet_limits": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"kind", "name"},
							"properties": map[string]any{
								"kind": map[string]any{"type": "string", "enum": LimitKinds},
								"name": map[string]any{"type": "string"},
							},
						},
					},
					"payment_methods": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"category":        map[string]any{"type": "string", "enum": Categories},
					"confidence":      map[string]any{"type": "number", "description": "0 to 1"},
				},
			},
		},
	},
}

// The values the schema allows.
var (
	Channels   = []string{"dine_in", "take_away", "delivery", "all"}
	LimitKinds = []string{"mall", "outlet", "city", "format"}
	Categories = []string{"food", "drink", "non_food"}
)

// Context is block 2: the brand and its outlets, or the tracked brands for an aggregator. It is
// the same for every item of a brand (outlets come sorted), so a provider's cache marker goes
// after it.
func Context(req models.ParseRequest) string {
	var b strings.Builder
	if req.Brand != "" {
		b.WriteString("Brand line: this source belongs to " + req.Brand + ".\n")
		if len(req.Outlets) > 0 {
			b.WriteString("Its outlets and malls, for outlet_limits:\n")
			for _, o := range req.Outlets {
				b.WriteString("- " + o + "\n")
			}
		}
		return b.String()
	}
	b.WriteString("This source is an aggregator that covers many brands; set brand_name per promo. Brands we track:\n")
	for _, k := range req.KnownBrands {
		b.WriteString("- " + k + "\n")
	}
	b.WriteString("Promos for other food and drink brands may be returned too, with their names as written.\n")
	return b.String()
}

// Item is block 3, the user message: different for every item, never cached.
func Item(req models.ParseRequest) string {
	it := req.Item
	posted := it.PostedAt
	if posted == "" {
		posted = req.Today + " (not stated; this is the date it was read)"
	}
	return "Today: " + req.Today + "\nPost date: " + posted + "\nURL: " + it.URL +
		"\n\n<item>\n" + norm.NFKC.String(it.Text) + "\n</item>"
}

// Retry is the follow-up message after a reply that failed Read: the problems, and a request
// for the whole corrected JSON.
func Retry(problem error) string {
	return "That reply has problems: " + problem.Error() + ". Reply again with the whole corrected JSON."
}
