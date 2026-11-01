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
testdata/                real Overpass response (1 Nov 2026) + brands.json
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
Linux/macOS/GitHub Actions; on Windows use WSL (Go 1.22 from go.dev + `apt install gcc`;
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
