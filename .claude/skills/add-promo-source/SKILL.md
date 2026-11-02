---
name: add-promo-source
description: Add a page the MakanDiMana promo fetcher should read (a row in the sources tab), and check it with a dry run. Use when the user wants promos from a new page, brand page, bank promo page or aggregator, or wants to pause, fix or retarget an existing source.
---

# Add a promo source

Sources live in the `sources` tab of the makandimana Google Sheet, not in code. Each row is one
page the promo job (`go run ./app/promos`) reads on its schedule; its `type` picks the adapter
(`web_page` is the only one today). Adding a source is a sheet row plus a dry run showing that
the page gives clean items, and that the parser reads them into the right promos.

You can't edit the sheet directly: produce the row for the user to paste, and check it locally
first. If the page needs code (a host the `web_page` rules can't read), use the
`add-source-type` skill instead.

## 1. Collect the facts

Ask only for what's missing:

- The page URL. For an aggregator, the brand's own category or merchant page, not the home page
  (`katalogpromosi.com/restoran-dan-cafe/<brand>/`, `giladiskon.com/merchant/<brand>`).
- Whether the page is about one brand or many (a bank's F&B page, an aggregator's front page).
- For a one-brand page: the brand. Its `brand_*` id must already be in the `brands` tab; if it
  isn't, use the `add-brand` skill first. The parse stage marks every item of a source whose
  brand is missing as `failed` (without spending a call) until the brand is added.
- How often it changes: daily for aggregators and banks, weekly (168) for a brand page that
  rarely changes.

## 2. Write the row

Columns, in order: `id | type | target | brand_id | is_aggregator | fetch_every_hours | status |
last_fetched_at | last_seen_ref | error_count | last_error | notes`

| Field | Rule |
|---|---|
| `id` | `src_<brand or site>_<site>`, `a-z0-9_` only (`src_hokben_katalog`, `src_bca_fnb`). **Never rename an existing id**: raw item ids are `<source id>:<ref>`, and promos list it in `source_ids`. |
| `type` | `web_page` |
| `target` | The page URL, `https://…` |
| `brand_id` | The `brand_*` id for a one-brand page; empty for an aggregator |
| `is_aggregator` | `1` when the page covers many brands (the parser names the brand per promo, and promos for brands not in `brands` are dropped); else `0` |
| `fetch_every_hours` | `24` by default; `168` for weekly |
| `status` | `active`; `paused` to add it without running it yet |
| `last_fetched_at` … `last_error` | Leave empty (`error_count` 0): the fetcher writes them |
| `notes` | Anything the user should remember about the page |

How `web_page` reads a target (`places-fetcher/README.md`, "The `web_page` source type"):

- katalogpromosi.com, giladiskon.com and promo.bca.co.id have their own rules: one item per post,
  deal or promo card, keyed by the site's own id.
- qpon.id fails (needs headless rendering, not built); keep such a source `paused`.
- Any other host is read whole as one item, keyed by a hash of its text. That works for a page
  whose text only changes when the promos do. A page with a clock, a visitor counter, rotating
  banners or "posted 3 hours ago" makes a new item every run, and each one costs a parser call:
  check for that in step 3.

## 3. Dry run: fetch only

From `~/promo/places-fetcher` in WSL (Claude on Windows: write the commands to a script in the
scratchpad and run it with `wsl.exe -d ubuntu -- bash -lc 'bash <script>'`). Put the row in a
temporary sources file, so the sheet isn't needed:

```bash
cd ~/promo/places-fetcher
cat > /tmp/source_new.json <<'EOF'
[{ "id": "src_example_katalog", "type": "web_page", "target": "https://example.com/promo/",
   "brand_id": "brand_example", "is_aggregator": false, "fetch_every_hours": 24, "status": "active" }]
EOF
go run ./app/promos -dry-run -parse=false -sources-file /tmp/source_new.json -source src_example_katalog -csv /tmp/raw_new.csv
```

Check `/tmp/raw_new.csv` and the log:

- **The source didn't fail.** A `robots.txt` refusal is final: tell the user, and don't work around it.
- **Item count is plausible** for the page (at most `WebPage.MaxNewItems`, 25, new items a run;
  the rest come next run).
- **Text is the promo, not the page around it**: each item starts with its title, has the deal
  and the dates, and no menus, share buttons, related posts, cookie banners or footers.
  `text_chars` above 6,000 is cut to 6,000 in the sheet.
- **`posted_at`** is filled where the site shows a date. Without it, the parser uses the date
  the item was read.
- **Refs are stable**: run the command again with `-csv /tmp/raw_new2.csv` and compare refs
  (`diff <(cut -d, -f3 /tmp/raw_new.csv | sort) <(cut -d, -f3 /tmp/raw_new2.csv | sort)`).
  For a page read whole (any other host), a different ref on the second run means the text
  changes by itself, and the source would make a new item every run. Then the page needs a host
  rule (`add-source-type`), or the source shouldn't be added.

Sheet configured locally (`SHEET_URL`, `SHEET_TOKEN`)? After the user pastes the row, the same
check runs against the sheet with `go run ./app/promos -dry-run -parse=false -source <id>`. Any
source runs that way, due or not, whatever its status.

## 4. Dry run: parse

This calls the parser (needs `ANTHROPIC_API_KEY`) and costs roughly $0.0005 per item (a 25-item
first run is about 1 cent). `-sources-file` needs brands from a file too: use a file with the
source's brand (for an aggregator, the tracked brands), and outlets if you want outlet limits
checked (`-outlets-file`, the `GET mode=outlets` shape; without it, every promo limited to a
place comes out `hidden`).

```bash
go run ./app/promos -dry-run -sources-file /tmp/source_new.json -source src_example_katalog \
  -brands-file /tmp/brands_new.json -csv /tmp/raw_new.csv -promos-csv /tmp/promos_new.csv
```

Check `/tmp/promos_new.csv` and the summary lines:

- Each real deal on the page is one promo with the right `brand_id`, `deal`, `channel` and dates.
- `hidden` rows: is it low confidence, `non_food`, or a place that matched no outlet? A place
  that should have matched usually means a missing outlet or `mall_name` in `outlets`.
- `dropped`: menu launches and, for aggregators, untracked brands. A tracked brand dropped here
  means its aliases don't match how the page writes it: fix them with `add-brand`.
- Items `without` promos are fine for news or menu posts. Many of them on a promo page means the
  text is wrong (step 3).

## 5. Hand over

Give the user:

1. The row to paste into the `sources` tab, tab-separated, columns in order.
2. What the dry runs showed: items, promos, hidden and dropped, with reasons, and any page quirk
   worth a `notes` entry.
3. What happens next: the source is read on the next `promos_daily` run once it's `active` (or
   now via **Actions → promos_daily → Run workflow** with its id).
4. Pausing, fixing and retargeting: set `status` to `paused` to stop it; after 3 failed runs in a
   row the fetcher sets `error`, so fix the cause (`last_error`) and set it back to `active`.
   Changing `target` keeps the id; items already stored keep their refs.

Never commit what you fetched: raw CSVs, promo CSVs and saved pages stay in `/tmp` or the
gitignored `places-fetcher/testdata/live/` (rule 12 in `.claude/rules/adapters.md`). Never run
without `-dry-run` unless the user asks: that writes the live sheet.
