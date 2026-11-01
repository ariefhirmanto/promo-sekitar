---
name: add-places-provider
description: Add or switch the places data vendor for the MakanDiMana places fetcher (a new adapter under outlet/provider, e.g. Geofabrik, Nominatim, Google Places, Foursquare). Use when the user wants another places API, a fallback vendor, or to replace Overpass.
---

# Add a places provider

The usecase only knows `outlet.PlaceProvider` and the vendor-neutral `models.Place` /
`models.Mall`. A vendor is one package under `places-fetcher/outlet/provider/<vendor>/` that
registers itself by name. Read `outlet/provider/overpass/` first: it's the reference adapter, and
a new one should look like it.

## Rules

1. **Don't touch the usecase, controller or repositories.** If a vendor gives something the
   models can't hold, add a vendor-neutral field to `models.Place`/`models.Mall` (never vendor
   names like `GoogleTypes`), and say so to the user before doing it.
2. **`Source()` names the data owner, not the API.** It's the sheet's `source` column and scopes
   upserts: a run only updates and closes rows of its own source.
   - Another API over OpenStreetMap data (Geofabrik extract, Nominatim, a self-hosted Overpass)
     must return `osm` **and** the same `SourceRef` format (`n123`, `w456`, `r789`), so outlet ids
     (`out_osm_n123`) stay stable and the switch doesn't duplicate or close rows.
   - A different data owner gets its own source (`google`, `foursquare`) and a stable vendor id as
     `SourceRef`. Its rows live beside the `osm` rows; switching vendors fully means old-source
     rows stop updating, so tell the user to hide or delete them in the sheet.
3. **Fail loudly, never return a partial result as success.** Timeouts, quota errors, truncated
   responses, and pagination that stops early must return an error; the usecase refuses to write
   an empty result, but a half result would still close real outlets after 25 days.
4. **Respect the vendor.** Follow its usage policy and rate limits (Nominatim: 1 req/s, no bulk
   use, real User-Agent). Every HTTP call takes `ctx`, has a timeout, and retries with backoff
   like `overpass/client.go`.
5. **Secrets come from env, never config.yaml.** Add the key to the adapter's config section with
   an empty value; viper maps `Google.APIKey` to `GOOGLE_APIKEY`. Add it to the workflow's `env`
   and tell the user which GitHub secret to create.
6. **Cost check before building.** If the vendor bills per request, estimate requests per run
   (brands × area tiles × pages) and per month (2 runs), and get the user's OK.

## Steps

### 1. Package layout

```
outlet/provider/<vendor>/
  <vendor>.go       adapter: init() registers, New(), Source(), SearchPlaces()
  client.go         client interface; HTTP client (retries, ctx, timeout) + fileClient (replay)
  query.go          request building from models.PlaceQuery (brands, bbox), if the vendor needs it
  transform.go      vendor response → models.PlaceResult; vendor types stay unexported here
  <vendor>_test.go  registration, transform, and error-case tests
```

### 2. Adapter skeleton

```go
const (
	Name   = "<vendor>" // registry name, used in config.yaml
	source = "<owner>"  // see rule 2
)

func init() {
	provider.Register(Name, func(cfg config.MainConfig, opts provider.Options) (outlet.PlaceProvider, error) {
		return New(cfg.<Vendor>, opts)
	})
}
```

`New` must honour both `provider.Options`:
- `File`: replay a saved raw response instead of calling the vendor (`fileClient`).
- `SaveRaw`: write the raw response to that path after a successful fetch.

Return an error from the factory when required config is missing (e.g. an empty API key), so
`provider.New` fails at startup instead of mid-run.

### 3. Transform

Map every place to `models.Place`:

| Field | Fill with |
|---|---|
| `Source`, `SourceRef` | Rule 2; `SourceRef` must be stable across runs |
| `Name` | The display name as shown to customers |
| `AltNames` | Brand label, other-language and official names (all used for brand matching) |
| `Address` | One readable line |
| `MallHints` | Building / complex names from the address (used when no mall outline matches) |
| `Lat`, `Lng` | The entrance or center point |

Return `models.Mall` outlines when the vendor has building polygons; skip unnamed ones. Without
malls, `mall_name` falls back to guessing from names and `MallHints`, which is fine.

Searching by brand: build the request from `query.Brands` (each `Brand.Names()`); the usecase
re-matches every place against the aliases, so over-fetching is safe and under-fetching isn't.

### 4. Config and wiring

- Add `<Vendor>Config` to `config/config.go` and a `<Vendor>:` section to `config.yaml`.
- Blank-import the package in `job/app.go` next to the overpass import.
- Switch with `Places.Provider: "<vendor>"` in `config.yaml`, or try it first with
  `-provider <vendor>`.

### 5. Tests and fixture

1. Fetch once with `-save-raw` and store it as `testdata/<vendor>_<yyyy-mm-dd>.json` (trim it if it's
   over ~1 MB; keep at least one outlet per test brand and, if supported, some malls).
2. Tests in the adapter package, mirroring `overpass/overpass_test.go`:
   - registered: `provider.New(Name, config.MainConfig{}, provider.Options{File: fixture})` works and
     `Source()` is right;
   - transform: a small inline response maps every `models.Place` field;
   - errors: an error/quota/truncated response returns an error.
3. A usecase fixture test like `TestRefreshOutletsFixture` with the new provider.

### 6. Verify

```bash
cd ~/promo/places-fetcher
gofmt -l . && go vet ./...
CGO_ENABLED=0 go test ./outlet/usecase/... ./outlet/provider/...
go test ./...
go run ./app -dry-run -provider overpass -brands-file testdata/brands.json -csv /tmp/a.csv
go run ./app -dry-run -provider <vendor> -brands-file testdata/brands.json -csv /tmp/b.csv
```

Compare the two dry runs per brand: outlet counts, false positives, `mall_name` coverage, and
for an `osm`-source adapter, that the ids overlap (`comm -12 <(cut -d, -f1 /tmp/a.csv | sort) <(cut -d, -f1 /tmp/b.csv | sort) | wc -l`).
Report the comparison to the user before switching `Places.Provider`.

### 7. Docs

Update `places-fetcher/README.md`: the structure tree, the vendor's config and secrets, and its
known gaps. Never run without `-dry-run` (live sheet write) unless the user asks.
