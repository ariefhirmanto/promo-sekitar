---
paths:
  - "places-fetcher/**/*.go"
---

# Clean architecture in the Go fetcher

The module follows github.com/ariefhirmanto/go-clean-rest. Each domain (`outlet/` today, `promo/`
per the promo fetcher RFC) has the same layers, and dependencies only point inward:
`app → job → controller → usecase → domain interfaces ← repository / adapters`, with `models`
at the centre.

## Layers and what each may import

| Layer | Package | Holds | May import (besides stdlib) | Never imports |
|---|---|---|---|---|
| Models | `models/` | Vendor-neutral data (`Brand`, `Place`, `Outlet`, results) | nothing from the project | anything else in the module |
| Domain | `<domain>/` (`outlet/`) | Ports: `Usecase`, `Controller`, `Repository`, `BrandRepository`, provider ports; input structs with `ValidateInput()` | `models` | usecase, repository, controller, adapters, `config` |
| Usecase | `<domain>/usecase/` | Business rules (matching, merging, dedupe, "refuse to write an empty result") | `models`, `<domain>`, `github.com/pkg/errors` | `config`, repository, controller, adapters, `net/http`, file I/O |
| Repository | `<domain>/repository/` | Storage: the sheet via Code.gs, CSV/JSON files for dry runs | `models`, `config`, `github.com/pkg/errors` | usecase, controller, adapters |
| Adapters | `<domain>/provider/<vendor>/` (later `promo/source/<type>/`, `promo/parser/<model>/`) | Outside services behind a port; see `adapters.md` | `models`, `config`, `<domain>`, their registry | usecase, repository, controller, `job`, other adapters |
| Controller | `<domain>/controller/` | Maps CLI input to usecase input, logs the result | `<domain>` | repository, adapters, `config` |
| Wiring | `job/` | The only place that builds concrete types and picks them (sheet vs CSV, provider by name) | everything above | — |
| Entry | `app/main.go` | Flags, `config.LoadConfig`, `job.Run` | `config`, `job`, registries (for flag help) | business logic |

## Rules

1. **Interfaces live in the domain package**, next to the types that use them, not in the
   implementing package. A new dependency of the usecase starts as a port in `<domain>/`.
2. **The usecase sees only ports and models.** It gets values like `MergeRadiusM` through its
   constructor, never by reading `config`. No HTTP, files, sheet or vendor names in it.
3. **Vendor and storage shapes stop at their layer.** Repositories and adapters convert to
   `models.*` before returning; JSON field names of Code.gs or a vendor never reach the usecase.
4. **Only `job/` wires.** Choosing an implementation (`-dry-run` → CSV, `-brands-file` → JSON,
   provider by name) happens there. No package below `job/` calls another package's `New…`
   except its own sub-packages.
5. **Constructors** are `NewXxx(deps…) *xxx` returning an unexported struct that satisfies the
   port; callers hold the interface type.
6. **Method shape:** `ctx context.Context` first; named results `(resp T, err error)`; validate
   input first (`input.ValidateInput()`) in the usecase.
7. **Errors** are wrapped once per layer step with `github.com/pkg/errors`:
   `errors.WithMessage(err, "outletUC.RefreshOutlets.GetBrands")` (usecase),
   `errors.Wrap(err, "sheetRepository.GetBrands")` (repository), `"<vendor>.Method.Step"`
   (adapter). `exception.PanicIfNeeded` is for startup only (config loading).
8. **Logs** use `[Domain][Method][Layer]`, e.g. `[Outlet][RefreshOutlets][Usecase]`; adapters use
   their name as the layer (`[Outlet][SearchPlaces][Overpass]`). Log at the point of failure,
   then return the wrapped error.
9. **New domain** (e.g. `promo/`) copies the `outlet/` layout: `<domain>/{repository.go,
   usecase.go, controller.go, input.go}` for ports and inputs, then `usecase/`, `repository/`,
   `controller/` and the adapter registries. Shared pieces (sheet client, brand matching) move to
   a package both domains import; don't import one domain's usecase from another.
10. **Tests** for usecases use fakes of the ports (no network, no sheet); adapter and repository
    tests use files in `testdata/`.

## Check before finishing

From `~/promo/places-fetcher` in WSL, after `gofmt -l .`, `go vet ./...`, `go test ./...`:

```bash
# usecase and controller packages must not import repositories, adapters, config or job
go list -f '{{.ImportPath}}: {{join .Imports " "}}' $(go list ./... | grep -E '/(usecase|controller)$') \
  | grep -E 'repository|provider|/source|/parser|places-fetcher/config|places-fetcher/job' \
  && echo "layer violation" || echo "layers ok"
```
