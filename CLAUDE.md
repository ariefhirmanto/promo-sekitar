# MakanDiMana

Food promo app MVP. Google Sheets is the database: tabs `brands` (edited by hand), `outlets`
(places fetcher), `promos` (promo fetcher). `apps-script/Code.gs` serves the feed and accepts
fetcher writes.

- `places-fetcher/`: Go, clean architecture after github.com/ariefhirmanto/go-clean-rest
  (models → domain interfaces → repository / usecase / controller, wired in `job/app.go`).
  Places vendors are adapters under `outlet/provider/<vendor>`.
- `.github/workflows/places_refresh.yml`: runs the fetcher on the 2nd and 16th, 03:00 WIB.

## Workflows

- Adding or fixing a brand: use the `add-brand` skill.
- Adding or switching a places API: use the `add-places-provider` skill.

## Rules

- Run Go commands in WSL from `~/promo/places-fetcher` (Go 1.22 + gcc live there, not on Windows).
  Before finishing a change: `gofmt -l .`, `go vet ./...`, `go test ./...`.
- `-dry-run` for every fetcher run; a run without it writes the live sheet, so only on request.
- Never change sheet tab names, header names, or existing ids (`brand_*`, `out_*`): Code.gs and
  promos depend on them. New columns are fine; Code.gs reads by header name.
- Rows with `source = manual` belong to the user; code must never update or close them.
- Secrets (`SHEET_URL`, `SHEET_TOKEN`, vendor keys) come from env / GitHub secrets, never files.
- After changing `apps-script/Code.gs`, remind the user to paste it into Apps Script and deploy a
  new version (Deploy → Manage deployments → edit → New version).
- Match the existing style: log lines like `[Outlet][Method][Layer]`, errors wrapped with
  `github.com/pkg/errors` (`errors.WithMessage(err, "outletUC.Method.Step")`).
