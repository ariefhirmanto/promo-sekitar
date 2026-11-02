---
name: add-brand
description: Add a new food brand to MakanDiMana (brands tab row, aliases, outlet check). Use when the user wants to track, add, or watch a new restaurant brand, or fix a brand's aliases.
---

# Add a brand

Brands live in the `brands` tab of the makandimana Google Sheet, not in code. The places fetcher
reads that tab to find outlets, and promos link to brands by `id`. Adding a brand is a sheet row
plus a check that its aliases find the right outlets and nothing else.

You can't edit the sheet directly: produce the row for the user to paste, and verify it with a
local dry run first.

## 1. Collect the facts

Ask only for what's missing:

- Official brand name as customers see it (e.g. `Marugame Udon`).
- Other spellings and names: shortened forms, old names, sub-brands / formats
  (e.g. `HokBen+`, `HokBen 24 Jam`, old name `Hoka Hoka Bento`).
- Watchlist or not (`is_watchlist`: 1 shows a ★ in the app).
- Pages with its promos (its katalogpromosi.com or giladiskon.com page, its own promo page), so
  the promo fetcher can read them: add them after the brand with the `add-promo-source` skill.

## 2. Write the row

Columns: `id | name | aliases | is_watchlist`

| Field | Rule |
|---|---|
| `id` | `brand_<slug>`: lowercase, `a-z0-9_` only, e.g. `brand_ramenya`. **Never rename an existing id**: outlets and promos reference it. |
| `name` | Official display name. Its compact form must be at least 4 letters/digits. |
| `aliases` | Comma-separated. Include every spelling that appears in place names; punctuation and spacing don't matter (`Ramen-Ya` = `Ramen Ya` = `RamenYa`). |
| `is_watchlist` | `1` or `0`. |

How the matcher uses aliases (`match/match.go`, shared by the places job and the promo job,
which matches the brand an aggregator's promo names the same way):

- Names are compacted (lowercase, letters/digits only, `+` → `plus`) and matched as substrings,
  longest alias wins.
- An alias that **contains the brand name** and is longer becomes the outlet's `format`
  (`HokBen 24 Jam` → format `HokBen 24 Jam`). Old names that don't contain it collapse to the
  brand name (`Hoka Hoka Bento` → format `HokBen`). Add sub-brands as aliases on purpose.
- Aliases shorter than 4 compact characters are ignored.

## 3. Check for bad aliases before testing

Reject or rework an alias when:

- It's a generic food word or common dish (`Bakso`, `Kopi`, `Ramen`, `Sushi`, `Ayam Geprek`):
  it will match hundreds of unrelated places.
- Its compact form is contained in another brand's name or alias, or contains one: the longer
  alias wins, so a place could flip brands. Compare against every row in the brands tab.
- It's a person's or place name likely to appear in other businesses.

## 4. Dry run with the new brand

From `places-fetcher/` (in WSL; Claude on Windows runs these through `wsl.exe -e bash -c '…'`):

```bash
cd ~/promo/places-fetcher
# current brands + the new one, without touching the sheet
cat > /tmp/brands_new.json <<'EOF'
[
  { "id": "brand_example", "name": "Example Brand", "aliases": "Example, Example Express" }
]
EOF
go run ./app -dry-run -brands-file /tmp/brands_new.json -csv /tmp/outlets_new.csv -save-raw /tmp/places_new.json
```

Use only the new brand in the file so the result is easy to read. Then check:

- **Count**: does the outlet count look plausible for the brand in Jabodetabek? Zero usually
  means a missing alias or a brand that's thin in the data (see step 6).
- **False positives**: read every `name` in `/tmp/outlets_new.csv`; anything that isn't this
  brand means an alias is too broad. Fix the alias and rerun.
- **Format / mall**: `format` values are the sub-brands you expect; `mall_name` is filled for
  mall outlets where the data has outlines.

If Overpass is busy, the fetcher retries on its own; keep `/tmp/places_new.json` and rerun with
`-places-file /tmp/places_new.json` while iterating on aliases.

## 5. Lock it in with a test (when aliases are tricky)

If the brand needed non-obvious aliases or had a false positive, add its real place names to
`TestMatchBrand` in `outlet/usecase/usecase_test.go` and the brand to the test's `brands` list,
then run:

```bash
CGO_ENABLED=0 go test ./outlet/usecase/...
```

Add the brand to `testdata/brands.json` only if a test needs it.

## 6. Hand over

Give the user:

1. The row to paste into the `brands` tab, tab-separated, columns in order.
2. The dry-run result: outlet count, any names you excluded and why.
3. What happens next: outlets arrive on the next scheduled run (2nd and 16th, 03:00 WIB), or now
   via **Actions → places_refresh → Run workflow**.
4. Outlets the data misses (common for mall-only brands): add them as rows in the `outlets` tab
   with `source = manual`; the fetcher never touches manual rows. They need `h3_r8`/`h3_r6` to
   show in Nearby: compute them with `geo.NewH3Indexer().Cells(lat, lng)` in a throwaway Go test,
   and hand the values over with the row.

Don't run the fetcher without `-dry-run` unless the user asks: that writes the live sheet.
