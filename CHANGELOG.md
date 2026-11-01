# Changelog

Changes to MakanDiMana, newest first. The promo fetcher is built milestone by milestone from the
[promo fetcher RFC](https://claude.ai/artifact/71WwNuUfmQF8fPcKYDrhLY) on the `promo-fetcher`
branch: one commit and one tag per milestone, so any milestone can be rolled back on its own.

## Rolling back

| Tag | State |
|---|---|
| `promo-base` | `main` before any promo fetcher work |
| `promo-m1` | M1: sheet tabs and Code.gs endpoints |
| `promo-m2` | M2: promo job skeleton in Go (no source types yet) |

A change can live in three places; roll back each one the change touched.

- **Code (git):** undo one milestone with `git revert <tag>` (a new commit that reverses it;
  history stays intact). To see the code as it was, `git switch --detach <tag>`. Avoid
  `git reset --hard` on a pushed branch.
- **Apps Script:** the live web app runs a numbered deployment version, not the repo. To go back,
  Deploy → Manage deployments → edit (pencil) → Version → pick the previous version → Deploy. The
  URL and tokens stay the same.
- **Sheet:** promo fetcher changes only add tabs, columns and rows; nothing is renamed or removed.
  An older Code.gs ignores the extras, so a code or Apps Script rollback needs no sheet change.
  Only delete them if you want them gone, and never rename existing tabs or headers.

## Unreleased

### Promo fetcher M2: promo job skeleton (tag `promo-m2`)

Added
- `go run ./app/promos`, a second job in the `places-fetcher` module: reads `sources`, fetches
  every due source through the adapter for its `type`, upserts what it read into `raw_items`, and
  writes each source's status back. Flags: `-dry-run`, `-source <id>`, `-items-file` (replay saved
  raw items), `-sources-file` (sources from JSON), `-csv`.
- `promo/` domain with the ports from the RFC, the source-type registry (`promo/source`,
  `Register` / `NewSet` / `Names`), a replay adapter, the sheet repository for all five M1
  endpoints, and dry-run repositories (sources JSON, raw items CSV, status log).
- `sheet/`: the Apps Script HTTP client, now shared by both jobs.
- `Promo:` section in `config.yaml`: `UserAgent`, `FetchTimeoutSeconds` (120), `MaxErrors` (3).
- Fixtures `testdata/sources.json` and `testdata/promo_items_2026-11-01.json`; 26 new Go tests.

Behaviour
- Due: `active` and `fetch_every_hours` passed since `last_fetched_at`, less 10% (at most 2 h).
  `-source` runs one source whatever its status or due time.
- A failure adds 1 to `error_count` and sets `last_error`; the 3rd in a row moves an active
  source to `error`. A success sets `last_fetched_at` and clears both. One failing source never
  stops the others; the run fails only on a sheet read/write error or when every due source failed.
- Raw items are written before statuses, so a failed write leaves `last_fetched_at` old and the
  next run fetches again. The fetch stage never sends a raw item `status`: Code.gs marks new rows
  `new` and leaves seen rows as they are.
- No source types are registered yet (M3), so a live run marks every source
  `unknown source type`. Don't schedule it before M3; use the dry runs in `places-fetcher/README.md`.

Changed
- `outlet/repository` uses the shared `sheet` client. Same modes, payloads and results; the
  not-configured message reads `SHEET_URL and SHEET_TOKEN must be set (or use -dry-run with
  -brands-file)`. The places dry run on the saved Overpass response still gives 68 outlets.

Deploy: nothing. No Code.gs change, no workflow change.

Roll back: `git revert promo-m2`. Nothing to undo in Apps Script or the sheet.

### Promo fetcher M1: sheet tabs and Code.gs endpoints (tag `promo-m1`)

Added
- `sources` tab (12 columns): one row per page the promo fetcher reads. You own `id`, `type`,
  `target`, `brand_id`, `is_aggregator`, `fetch_every_hours`, `status`, `notes`; the fetcher
  writes `last_fetched_at`, `last_seen_ref`, `error_count`, `last_error`, and may move `status`
  from `active` to `error` only.
- `raw_items` tab (11 columns): every post or page read, `text` capped at 2,000 characters, rows
  older than 90 days pruned on each write.
- Six `promos` columns at the end: `source_ids`, `source_refs`, `dedupe_key`, `confidence`,
  `first_seen_at`, `last_seen_at`. Existing columns keep their names and order.
- Code.gs endpoints, all on `WRITE_TOKEN`: `GET mode=sources`, `GET mode=raw_items&status=…`,
  `POST mode=update_sources`, `POST mode=upsert_raw_items`, `POST mode=upsert_promos`.
- `setupPromoTabs()`, run once from the Apps Script editor: creates the two tabs, adds the promos
  columns, and seeds the 9 launch sources into an empty `sources` tab. Safe to run again.
  Seeded as `paused`: `src_ramenya_qpon` (needs headless Chrome and a brand page URL),
  `src_chatime_giladiskon`, `src_richeese_katalog` (activate once their brand rows exist).
- `apps-script/test/`: Code.gs runs under `node --test` against an in-memory sheet (13 tests).

Behaviour
- `upsert_promos` writes `source = web` and only updates rows that already have `source = web`;
  any other row (`manual`) is skipped and never rewritten, cell by cell. A web promo you hid by
  hand gets the fetcher's status back when it is written again; set its `source` to `manual` to
  keep it out.
- `doPost` routes through a table of write modes; `upsert_outlets` behaves exactly as before.

Deploy
1. Paste `apps-script/Code.gs` into the Apps Script editor and save.
2. Run `setupPromoTabs` once from the editor (Run ▸ select function); check the log for
   `{"seeded_sources":9}`.
3. Deploy → Manage deployments → edit → Version: New version → Deploy.

Roll back: `git revert promo-m1`; in Apps Script, redeploy the previous version. The new tabs and
columns can stay.
