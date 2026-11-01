# MakanDiMana

Food promo app MVP. Google Sheets is the database: tabs `brands` (edited by hand), `outlets`
(places fetcher), `promos` (promo fetcher). `apps-script/Code.gs` serves the feed and accepts
fetcher writes.

- `places-fetcher/`: Go, clean architecture after github.com/ariefhirmanto/go-clean-rest
  (models → domain interfaces → repository / usecase / controller, wired in `job/app.go`).
  Places vendors are adapters under `outlet/provider/<vendor>`.
- `.github/workflows/places_refresh.yml`: runs the fetcher on the 2nd and 16th, 03:00 WIB.
- `web/`: Next.js 16 app (App Router, Cache Components). Read `web/AGENTS.md` and the docs in
  `web/node_modules/next/dist/docs/` before changing it: APIs differ from older Next.js.

## Workflows

- Adding or fixing a brand: use the `add-brand` skill.
- Adding or switching a places API: use the `add-places-provider` skill.
- Go layering and adapter contracts: `.claude/rules/clean-architecture.md` and
  `.claude/rules/adapters.md` (they load automatically for files under `places-fetcher/`).

## Rules

- Run Go commands in WSL from `~/promo/places-fetcher` (Go 1.22 + gcc live there, not on Windows).
  Before finishing a change: `gofmt -l .`, `go vet ./...`, `go test ./...`.
- `web/`: Node 22 from nvm in WSL (`. ~/.nvm/nvm.sh` first in non-interactive shells). Before
  finishing: `npm test`, `npx eslint .`, `npx tsc --noEmit`, `npx next build`. The browser only
  calls `/api/feed`; anything that touches `READ_TOKEN` or `APPS_SCRIPT_URL` lives in a file that
  imports `server-only`. Use `FEED_MOCK=1` for UI work without the sheet.
- `-dry-run` for every fetcher run; a run without it writes the live sheet, so only on request.
- Never change sheet tab names, header names, or existing ids (`brand_*`, `out_*`): Code.gs and
  promos depend on them. New columns are fine; Code.gs reads by header name.
- Rows with `source = manual` belong to the user; code must never update or close them.
- `promos.source` is `web` for anything taken from a web page (bank promo pages, aggregators like
  katalogpromosi.com), whether pasted by hand or written by a promo fetcher; `action_url` keeps
  the page. A Go promo feeder writes `web` and, like the outlets upsert, only touches `web` rows.
  `outlets.source` stays the data owner (`osm`): it is part of the outlet ids.
- Secrets (`SHEET_URL`, `SHEET_TOKEN`, vendor keys) come from env / GitHub secrets, never files.
- Two Apps Script tokens: `READ_TOKEN` for feed reads (Next.js server only, never in browser code
  or a `NEXT_PUBLIC_` var) and `WRITE_TOKEN` for fetchers (`SHEET_TOKEN`). A new endpoint picks
  one with `authorized_(p, '<PROPERTY>')`; anything that changes the sheet takes `WRITE_TOKEN`.
- The spreadsheet stays shared as Restricted; the web app runs as its owner.
- After changing `apps-script/Code.gs`, remind the user to paste it into Apps Script and deploy a
  new version (Deploy → Manage deployments → edit → New version).
- Match the existing style: log lines like `[Outlet][Method][Layer]`, errors wrapped with
  `github.com/pkg/errors` (`errors.WithMessage(err, "outletUC.Method.Step")`).
