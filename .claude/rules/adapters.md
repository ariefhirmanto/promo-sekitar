---
paths:
  - "places-fetcher/**/provider/**"
  - "places-fetcher/**/source/**"
  - "places-fetcher/**/parser/**"
  - "places-fetcher/job/**"
  - "places-fetcher/config/**"
---

# Adding an adapter

An adapter puts one outside service behind one port, so the usecase never knows which service
runs. Today: places vendors under `outlet/provider/<vendor>/` (port `outlet.PlaceProvider`,
reference adapter `overpass/`). Planned in the promo fetcher RFC: source types under
`promo/source/<type>/` (port `SourceAdapter`) and models under `promo/parser/<model>/` (port
`Parser`). Every kind follows the same rules; the layer rules are in `clean-architecture.md`.

For a places vendor, follow the `add-places-provider` skill step by step; this file is the
contract it applies.

## Contract

1. **One package per adapter**, named after the service (`overpass`, `webpage`, `claude`), under
   its domain's registry directory. It implements exactly one port declared in the domain
   package, and its public surface is `Name`, `New(…)` and the port's methods.
2. **Registry, like `database/sql` drivers.** The registry package (`outlet/provider/`) holds
   `Register(name, Factory)` (panics on a duplicate), `New(name, cfg, opts)` and `Names()`. The
   adapter calls `Register` in `init()`; `job/` blank-imports it; config picks it by name
   (`Places.Provider`), with a flag to override for trying it (`-provider`). A new adapter kind
   gets its own registry with the same three functions, not a switch statement.
3. **The factory fails at startup** when required config is missing (empty API key, no
   endpoint), so `New` errors before any fetch.
4. **Vendor types stay unexported in the package.** `transform.go` maps the response to
   `models.*`. If the models can't hold something, add a vendor-neutral field (never
   `GoogleTypes`, `OSMTags`) and tell the user first.
5. **No reaching sideways.** An adapter never imports a usecase, repository, controller, `job`
   or another adapter, and never writes to the sheet. It returns data; the usecase decides.
6. **Fail loudly.** Timeouts, quota errors, truncated responses, pages that stop early, and
   pages that render without the expected list return an error, never a partial result as
   success. A partial places result closes real outlets after 25 days.
7. **Replay and record.** Every adapter honours its registry's `Options`: `File` replays a saved
   raw response instead of calling the service, `SaveRaw` writes the raw response after a
   successful fetch. Tests run on these files, never on the network.
8. **Be a good client.** Every call takes `ctx` and has a timeout; retries use backoff
   (`overpass/client.go`). Follow the service's rate limits and robots.txt, send the configured
   `UserAgent`, and space requests per host (1 per second by default, the `Crawl-delay` where
   robots.txt sets one).
9. **Config and secrets.** A `<Name>Config` struct in `config/config.go` and a `<Name>:` section
   in `config.yaml`; keys and tokens stay empty there and come from env (viper maps
   `Google.APIKey` to `GOOGLE_APIKEY`) and GitHub secrets. Tell the user which secret to create
   and add it to the workflow's `env`.
10. **Identity of the data.**
    - Places: `Source()` names the data owner, not the API (`osm` for any OpenStreetMap API), and
      `SourceRef` is the owner's stable id, because outlet ids are `out_{Source}_{SourceRef}`.
    - Promos: every promo an adapter produces has `source = web` (CLAUDE.md); which `sources`
      row it came from goes in `source_ids`, and an item's ref must be stable across runs.
11. **Cost check first** for any service that bills per call (a paid places API, an AI parser):
    estimate calls per run and per month and get the user's OK before building.

## Files

```
<registry>/<name>/
  <name>.go        Name, init() → Register, New(), the port's methods
  client.go        client interface: HTTP client (ctx, timeout, retries) + fileClient (replay)
  query.go         request building from the port's input, if the service needs it
  transform.go     raw response → models.*; vendor types unexported
  <name>_test.go   registered via the registry's New, transform, error cases
testdata/<name>_<yyyy-mm-dd>.json   saved raw response (trim past ~1 MB)
```

## Done when

- Tests cover: registration through the registry's `New` with `Options{File: fixture}`, the
  transform of a small inline response (every model field), and at least one error response.
- `gofmt -l .`, `go vet ./...` and `go test ./...` pass in WSL, and the layer check in
  `clean-architecture.md` prints `layers ok`.
- A `-dry-run` with the new adapter has been compared with the current one (counts, false
  positives) and reported to the user before config switches to it.
- `places-fetcher/README.md` lists the adapter, its config, its secrets and its known gaps.
