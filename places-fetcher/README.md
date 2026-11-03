# places-fetcher

Fills the `outlets` tab of the makandimana sheet from OpenStreetMap (free, no API key).
Layout follows [go-clean-rest](https://github.com/ariefhirmanto/go-clean-rest).

## Structure

```
app/main.go              flags → job.Options, loads config, runs the job
job/app.go               wires repositories → usecase → controller (like server/app.go)
config/                  viper: config.yaml, overridable by env (Sheet.Token → SHEET_TOKEN)
models/                  Brand, Outlet, vendor-neutral Place / Mall, results
outlet/                  domain interfaces + input
  repository.go          BrandRepository, PlaceProvider (vendor port), Repository, GeoIndexer
  usecase.go             Usecase
  controller.go          Controller
  input.go               InputRefresh + validation
  provider/              registry of places vendor adapters (Register / New / Names)
    overpass/            OSM via Overpass: client (fetch) → query → transform (OSM → models.Place)
  repository/            sheet (Apps Script web app), CSV, brands JSON
  usecase/               brand matching, format, mall lookup, 50 m merge (vendor-agnostic)
  controller/            CLI controller: runs the usecase, logs the summary
  geo/                   H3 (uber/h3-go, cgo) behind GeoIndexer
app/promos/main.go       promo job: flags → job.PromoOptions, loads config, runs it
job/promos.go            wires the promo job (sheet or files, source registry or replay)
sheet/                   Apps Script client shared by both jobs (token + mode, {ok, error} envelope)
match/                   brand alias matching shared by both jobs (Compact, BestBrand)
promo/                   promo domain: ports (SourceReader, SourceStatusWriter, RawItemWriter/Reader,
                         RawItemStatusWriter, BrandReader, OutletReader, Parser, PromoRepository,
                         SourceAdapter, SourceAdapters) + InputRun
  source/                registry of source types (Register / NewSet / Names); replay/ for -items-file
    webpage/             web_page: client, robots.txt, HTML text, host rules (katalog, giladiskon, bca, generic)
  parser/                registry of parsers (Register / New / Names); golden-set test
    prompt/              shared by every parser: instructions, schema, request blocks, reply checks, retry loop
    claude/              Claude Haiku: Messages API call, cache marker, stop reasons, cost
  repository/            sheet (sources, raw_items, promos, outlets), JSON files, CSVs, dry-run logs
  usecase/               fetch stage (due sources, status write-back) and parse stage (brand and
                         outlet resolving, hidden rules, dedupe and merge, budget)
  controller/            CLI controller: runs the usecase, logs per source and the parse summary
testdata/                real Overpass response (1 Nov 2026) + brands.json; sources.json + promo items
```

```
brands tab ──GET mode=brands──▶ usecase ◀── PlaceProvider ◀── adapter (overpass, …)
                                   │  match aliases → mall polygon → H3 r8/r6 → merge 50 m
outlets tab ◀──POST mode=upsert_outlets──┘
```

## Switching or adding a places vendor

The usecase only sees `models.Place` / `models.Mall` through `outlet.PlaceProvider`, so a vendor
change never touches it.

1. Create `outlet/provider/<vendor>/` with a client (calls the vendor) and a transform (vendor
   response → `models.PlaceResult`). Set `Place.Source` (the sheet's `source` column, e.g. `osm`,
   `google`) and a stable `Place.SourceRef`; the outlet id becomes `out_{Source}_{SourceRef}`.
2. In its `init()`: `provider.Register("<vendor>", factory)`; add its settings section to
   `config.yaml`.
3. Blank-import it in `job/app.go`, then set `Places.Provider: "<vendor>"` (or `-provider <vendor>`).

Upserts only replace rows of the provider's `Source()`, so rows from another vendor or `manual`
stay untouched.

## One-time setup

1. Replace the Apps Script code with `../apps-script/Code.gs` (adds `mode=brands` and `doPost`).
2. Script properties: `READ_TOKEN` (the Next.js server's feed reads) and `WRITE_TOKEN` (the
   fetchers), two different random values; delete any old `TOKEN`. A call is refused when its
   property is missing.
3. Deploy → Manage deployments → edit → **New version** (the URL stays the same).
4. GitHub repo secrets: `SHEET_URL` (the `/exec` URL) and `SHEET_TOKEN` = the `WRITE_TOKEN` value.

## Run

```bash
go mod tidy
CGO_ENABLED=0 go test ./outlet/usecase/... ./outlet/provider/...   # no C compiler needed
go run ./app -dry-run -brands-file testdata/brands.json -places-file testdata/overpass_2026-11-01.json
SHEET_URL="https://script.google.com/macros/s/.../exec" SHEET_TOKEN="..." go run ./app
```

`-dry-run` writes `outlets.csv` instead of the sheet. `go run ./app -h` lists all flags.

`outlet/geo` uses cgo (uber/h3-go), so building the app needs a C compiler: fine on
Linux/macOS/GitHub Actions; on Windows use WSL (Go 1.24 from go.dev + `apt install gcc`;
Ubuntu 22.04's own golang package is too old).

## What it writes

| column | value |
|---|---|
| id | `out_osm_n123` / `out_osm_w456` (OSM node/way id, stable across runs) |
| format | sub-brand alias when the name contains one (`HokBen+`, `HokBen 24 Jam`), else the brand name |
| mall_name | name of the OSM mall outline (`shop=mall`) the outlet sits in; else guessed from its name |
| status, last_seen_at | `open` + today for every outlet seen; OSM rows unseen for 25 days become `closed` |

Rows with another `source` (e.g. `manual`) are never touched, so hand-added outlets are safe.

## Known MVP gaps

- OSM misses some outlets (Marugame and Ramen Ya especially). Add them by hand with
  `source = manual`.
- Public Overpass servers are often busy; the fetcher tries 3 servers × 3 rounds and aborts
  without writing if all fail, the query times out, or nothing matches, so outlets are never
  wrongly closed.

## Promo job

Built milestone by milestone from the promo fetcher RFC (see `../CHANGELOG.md`). It reads the
`sources` tab, fetches every due source through its type's adapter, stores the new items in
`raw_items`, writes each source's status back, then reads the new items into `promos` with the
parser (Claude Haiku). A dry run writes nothing to the sheet but still calls the parser, which
costs a little; `-parse=false` only fetches.

```bash
export ANTHROPIC_API_KEY=...   # in ~/.profile in WSL, never in a repo file
go run ./app/promos -dry-run -parse=false -sources-file testdata/sources.json -items-file testdata/promo_items_2026-11-01.json
go run ./app/promos -dry-run -sources-file testdata/sources.json -items-file testdata/promo_items_2026-11-01.json -brands-file testdata/brands.json
go run ./app/promos -dry-run -source src_ramenya_katalog     # one source from the sheet, due or not
go run ./app/promos -dry-run                                 # every due source, real pages and parser, nothing written
```

`-dry-run` logs the status writes and writes `raw_items.csv` and `promos.csv` (`-csv`,
`-promos-csv`). With `-sources-file`, brands come from `-brands-file` and outlets from
`-outlets-file` (none: every promo limited to a place is saved hidden); stored items aren't read.
The workflow `promos_daily` runs it at 03:00 WIB every day and writes the sheet. Actions →
promos_daily → Run workflow runs it by hand (dry run by default, or one source). It shares the
`sheet-writes` concurrency group with `places_refresh`, which starts at the same time on the 2nd
and 16th: one waits for the other.

### The `web_page` source type

`promo/source/webpage` reads a page with a rule for its host. A listing rule reads the source's
`target` and lists its items; only items whose ref isn't in `raw_items` yet (`GET
mode=raw_item_refs`) get their own page opened, at most `WebPage.MaxNewItems` (25) per run.

| Host | `target` | One item, `ref` | Text |
|---|---|---|---|
| katalogpromosi.com | `/restoran-dan-cafe/<slug>/` | a post in `category-<slug>`, post id | title + `entry-content`, without share buttons, related posts and banners; `posted_at` from `article:published_time` |
| giladiskon.com | `/merchant/<slug>` | a deal in `#deal-feed`, deal slug | title + the deal page's `.prose`; no `posted_at` (the page has none) |
| promo.bca.co.id | `/id/all/c/<category>` | a card in the Next.js payload, `idPromo` | `Brand:`, `Promo:`, `Periode:` from the card's dates + the card page's terms; `posted_at` = `publishDate` |
| qpon.id | — | — | fails: needs headless rendering (not built); keep the source paused |
| any other | the page | the page, hash of its text | title + `<main>` or `<article>` text |

Requests send `Promo.UserAgent`, wait `WebPage.HostDelayMs` (1 s) between calls to a host, follow
its robots.txt, and retry 429/5xx/network errors `WebPage.Retries` (2) times. One item page failing
is skipped and retried next run; every new item page failing, or a listing that no longer has the
expected structure, fails the source. Only the first listing page is read (newest first).

Adding a host: a `<host>.go` with a `rule`, an entry in `rules` in `webpage.go`, hand-written test
pages in `testdata/webpage/` (the markup the rule reads, invented brand and text; never a copy of
the live page), and a live `-dry-run -source` check. Live captures go in the gitignored
`testdata/live/`.

- A source is due when it's `active` and `fetch_every_hours` has passed since `last_fetched_at`,
  less 10% of the interval (at most 2 h) so a run that starts a little early still picks it up.
- `-source` runs that one source whatever its status; a paused source never moves to `error`.
- A failed fetch adds 1 to `error_count` and sets `last_error`; the 3rd failure in a row
  (`Promo.MaxErrors`) moves an active source to `error`. A success clears both.
- The run fails only when the sheet can't be read or written, or every due source failed.

### Parsing: raw items → promos

Each run parses this run's new items, then stored `new` items, then `failed` items with
`Parser.Retries` (3) runs left, with the parser `Parser.Name` picks (`-parser` overrides it for a
run). One call per item; with `claude` (`promo/parser/claude`):

- Model `claude-haiku-5-5`, effort `low`, structured outputs (the reply schema is in
  `promo/parser/claude/prompt.go`; `is_deal` false drops menu launches and news).
- Request layout for the prompt cache: block 1 the fixed instructions with worked examples
  (invented brand), block 2 the brand and its sorted outlet names (aggregators: the tracked
  brands) with the one cache marker, block 3 the item (NFKC text, post date, URL, today).
  The run log prints `cache read N tokens`; if it stays 0 for a week, drop the marker.
- A reply that fails the checks in `prompt.Read` (enums, date format and order, confidence 0–1,
  a deal with no title) is sent back once with the problems (`prompt.Run`, shared by every parser). A second bad reply, a refusal, a cut-off reply or a 400 marks
  the item `failed`. Any other API error (key, quota, network) stops parsing for the run, after
  writing what was done, and fails the run.
- `Parser.BudgetUSD` (0.50) per run at `Claude.InputUSDPerMTok` / `OutputUSDPerMTok`; past it,
  the remaining items stay `new` for the next run.

Resolving and writing:

- Brand: a brand source's `brand_id`; for an aggregator, the brand the parser names, matched with
  the places job's alias matching (`match.BestBrand`), and dropped if it isn't tracked. A brand
  source whose brand isn't in `brands` yet is marked `failed` without a call or an attempt.
- Outlets: `outlet_limits` (mall, outlet, city, format) are matched against the brand's open
  outlets (`GET mode=outlets`); empty means every outlet.
- Dropped, never written: a promo without `valid_until` ("while stocks last", standing or weekly
  offers with no end date). Aggregators keep ended deals for years, and nothing tells when an
  undated one stops. Add a standing offer by hand (`source = manual`) to show it.
- Saved `hidden`: confidence under `Parser.HideBelow` (0.6), `non_food`, or a place that matches
  no outlet. Flip them to `active` by hand (and set `source = manual` to keep that).
- `dedupe_key` = brand | the deal's numbers (else its compact text) | valid_from | valid_until |
  outlet ids; `id` = `promo_` + 16 hex of its SHA-256. The same promo from two items or sources
  is one row; Code.gs adds to its `source_ids` and `source_refs`.
- Promos are written before raw item statuses, so a failed promo write leaves the items `new`.
- Raw item status: `parsed` (with `promo_ids`), `skipped` (no promo kept), or `failed` (with
  `attempts` and `error`).

Golden set: `go test -tags golden -v ./promo/parser/` runs the cases in the gitignored
`testdata/live/golden/` against the parser `Parser.Name` picks, or `PARSER=<name>` (format in
`promo/parser/golden_test.go`); it passes at 15 of 17. A normal `go test` never calls the API.

### Switching or adding a parser

Switching between registered parsers is a config change: set `Parser.Name` (try it first with
`-dry-run -parser <name>` and `PARSER=<name> go test -tags golden ./promo/parser/`). Adding one
is a package under `promo/parser/<name>` that sends the shared blocks from `promo/parser/prompt`
to its service; the steps are in `promo/parser/parser.go`. Nothing in the usecase changes. For
services with an OpenAI-compatible API (hosted Qwen, routers), one generic adapter with
`BaseURL`, `Model` and prices in config would make moving between them a config change too;
check that the service returns token usage (the budget needs it) and whether it enforces the
JSON schema (if not, `prompt.Read` and the retry catch bad replies).

Secrets for `promos_daily`: `SHEET_URL`, `SHEET_TOKEN` (as for places) and `ANTHROPIC_API_KEY`
(from a `makandimana` workspace in console.anthropic.com with a monthly spend limit).
