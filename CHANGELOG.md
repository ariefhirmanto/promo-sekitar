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
| `promo-m3` | M3: `web_page` source type; Code.gs `raw_item_refs` |
| `promo-m4` | M4 + M5: Claude Haiku parser, merge and promo upsert; `promos_daily` workflow (disabled); Go 1.24 |
| `promo-m6` | M6: `add-promo-source` and `add-source-type` skills; promo rules in CLAUDE.md |

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

### Promos without an end date are dropped

Changed
- The promo fetcher no longer writes a promo whose `valid_until` is empty: aggregators keep ended
  deals for years, and an undated one looked current (Gila Diskon pages have no post date, so it
  got the run date and the feed kept it 30 days). An item whose promos are all undated is
  `skipped`, so it isn't parsed again. Standing or weekly offers with no end date go too; add one
  by hand (`source = manual`) to show it.

Deploy: merge to `main`; the next `promos_daily` run uses it. Undated `web` rows already in
`promos` stay until the feed's 30-day rule drops them, or delete them by hand (filter `source` =
`web` and `valid_until` empty; leave `manual` rows). Roll back: `git revert` this commit.

### App: pick a brand

Added
- `Code.gs` feed: optional `brand=brand_x` keeps only that brand's promos (swipe and nearby,
  still nearest outlet first), and every feed answer lists `brands`: each brand with a current
  promo and its count. Without `brand` the feed answers as before, plus that list.
- Web: a brand dropdown above the deck ("Semua brand" or one brand, with counts). Picking one
  restarts the deck for the same location; `/api/feed` checks and passes `brand`, and caches per
  brand.

Deploy: paste `apps-script/Code.gs` into Apps Script and deploy a new version, then merge so
Vercel deploys the web app (in that order: the dropdown only appears once the feed sends
`brands`). Roll back: `git revert` this commit and deploy `Code.gs` again.
### promos_daily runs every day at 03:00 WIB

Changed
- `.github/workflows/promos_daily.yml`: the schedule is on, `0 20 * * *` (20:00 UTC = 03:00 WIB)
  instead of the commented-out 06:00 WIB. Scheduled runs write the sheet; manual runs stay dry
  runs by default.
- `promos_daily` and `places_refresh` share the `sheet-writes` concurrency group: on the 2nd and
  16th both start at 03:00 WIB, and GitHub now runs one after the other instead of both writing
  the sheet at once.

Deploy: merge to `main` (schedules only run from the default branch). Roll back: `git revert`
this commit, or disable the workflow in Actions → promos_daily → ⋯ → Disable workflow.

### Fix: one slow Claude reply stopped the whole parse run

Found in the first parse dry run against the sheet (`context deadline exceeded`).

Fixed
- `Claude.TimeoutSeconds` (60) now applies to each attempt, so the SDK retries a reply that hangs
  (2 retries, with backoff) instead of the first attempt using up the whole call.
- A call that still times out after its retries is that item's failure (`failed`, tried again
  next run), not the run's: the other items go on. Running out of the job's own 20 minutes still
  stops parsing after writing what was done.

Deploy: nothing; the next promo run uses it. Roll back: `git revert` this commit.

### Feed: undated promos leave after 30 days

Changed
- `Code.gs` feed: a promo without `valid_until` is shown for `UNDATED_KEEP_DAYS` (30) after its
  `posted_at`, instead of forever. Its row stays in the sheet. Promos without `posted_at` (manual
  rows, mostly) are kept as before; expired promos were already dropped.

Deploy: paste `apps-script/Code.gs` into Apps Script and deploy a new version (Deploy → Manage
deployments → edit → New version). Roll back: `git revert` this commit and deploy again.

### Fix: promo outlet limits matched outlets they don't name

Found in a local parse dry run of the HokBen katalogpromosi page with outlets.

Fixed
- A place the promo names ("HokBen Sulawesi Selatan") no longer matches every outlet whose name is
  just the brand ("HokBen"): an outlet name only matches when it holds the place.
- A mall named inside the place ("HokBen Grand Indonesia") still matches, but by whole words, so
  "Pakuwon Mall" no longer matches "ÆON Mall" (compacted, "onmall" sits inside "pakuwonmall").

Deploy: nothing; the next promo run uses it. Roll back: `git revert` this commit.

### Promo fetcher M6: rules and skills (tag `promo-m6`)

Added
- `.claude/skills/add-promo-source`: a `sources` row, step by step, with a fetch-only dry run
  (item count, clean text, `posted_at`, refs that stay the same on a second run) and a parse dry
  run (promos, hidden and dropped, with the cost), then the row to paste.
- `.claude/skills/add-source-type`: when a site needs code, either a host rule in `web_page` or a
  new source type under `promo/source/<type>`, with the rules every source follows (stable refs,
  skip stored items, clean text, fail loudly, hand-written test pages) and the checks.

Changed
- CLAUDE.md: the promo job, its tabs and workflow in the overview; the new skills and the parser
  steps under Workflows; rules for `sources` ownership, `src_*` ids, `raw_items`, and parse cost.
- `add-brand` skill: points to the shared `match/` matcher, and to `add-promo-source` for the
  brand's promo pages (instead of noting an Instagram handle).

Deploy: nothing. No code, Code.gs or workflow change.

Roll back: `git revert promo-m6`.

### Promo fetcher M4 + M5: parser, merge and upsert (tag `promo-m4`)

Built as one milestone (decided 2 Nov 2026).

Added
- Parse stage after fetching: this run's new items, stored `new` items, and `failed` items with
  retries left are read into promos and upserted to `promos` (`source = web`). Raw items get
  `parsed` / `skipped` / `failed` with `attempts`, `error` and `promo_ids`.
- `promo.Parser` port and the parser registry (`promo/parser`, `Register` / `New` / `Names`);
  `Parser.Name` picks the parser and `-parser` overrides it for a run. `promo/parser/prompt`
  holds what every parser shares (instructions, schema, request blocks, reply checks, the
  one-retry loop), so a new provider is a thin package and switching is a config change.
  `promo/parser/claude` on the official Go SDK (`anthropic-sdk-go` v1.84.0): `claude-haiku-5-5`,
  effort low, structured outputs, one retry with the problems, refusal / `max_tokens` / 400 as
  item failures, prompt cache marker after the brand block, cost per call.
- Hidden rules: confidence under 0.6, `non_food`, or an outlet limit that matches no outlet.
  Menu launches (`is_deal` false) and aggregator promos for untracked brands are dropped.
- Dedupe key (brand, the deal's numbers, dates, outlets) → `promo_` id; the same promo from
  several items or sources is merged.
- `Parser:` and `Claude:` sections in `config.yaml` (`BudgetUSD` 0.50 per run, `Retries` 3,
  `HideBelow` 0.6, prices for the budget). `Claude.APIKey` reads `ANTHROPIC_API_KEY`.
- Flags: `-parse` (default true), `-parser`, `-promos-csv`, `-brands-file`, `-outlets-file`.
- `match/`: brand alias matching moved out of `outlet/usecase`, now shared by both jobs.
- Code.gs `GET mode=outlets` (open outlets for location matching); `promos` columns
  `recurrence` and `payment_methods`; `upsert_promos` adds to `source_ids` and `source_refs`
  instead of replacing them. `raw_items` text cap raised from 2,000 to 6,000 characters.
- `.github/workflows/promos_daily.yml`, committed disabled: the 23:00 UTC schedule is commented
  out; Run workflow starts it by hand, as a dry run by default.
- Golden-set test behind `-tags golden`, reading the gitignored `testdata/live/golden/`; it runs
  any registered parser (`PARSER=<name>`), to compare providers.
- 20 Go tests (parse stage 10, shared prompt 4, Claude adapter 6 with the registry) and 3 Code.gs tests.

Changed
- Go 1.24 (the SDK needs it): `go.mod`, both workflows, CLAUDE.md. Install it in WSL from go.dev.
- `NewPromoUsecase` takes `*usecase.ParseDeps` (nil = fetch only). With no source due, a run
  still parses stored items.

Checked (dry run, nothing written): the replay fixture through a local stand-in for the Messages
API: 5 items, 5 promos, 1 hidden for low confidence, cache marker sent on every call. Not yet run
against the live API: no `ANTHROPIC_API_KEY` in WSL, and the golden set isn't written yet.

Deploy
1. Paste `apps-script/Code.gs` into the Apps Script editor and save; run `setupPromoTabs` once to
   add the two new `promos` columns.
2. Deploy → Manage deployments → edit → Version: New version → Deploy.
3. Add the `ANTHROPIC_API_KEY` repo secret. Run `promos_daily` by hand (dry run) and check the
   log, then uncomment its schedule.

Roll back: `git revert promo-m4`; in Apps Script, redeploy the previous version. Promos written by
M4 stay in the sheet (`source = web`); hide or delete them by hand if wanted. The two new columns
can stay.

### Promo fetcher M3: `web_page` source type (tag `promo-m3`)

Added
- `promo/source/webpage`, registered as `web_page`, with a rule per host: katalogpromosi.com
  brand category pages (one item per post), giladiskon.com merchant pages (one per deal),
  promo.bca.co.id category pages (one per promo card, with exact start/end dates), and any other
  page as one item keyed by a hash of its text. qpon.id fails with "needs headless rendering"
  until that is built; its source stays `paused`.
- HTTP client: `Promo.UserAgent`, 30 s timeout, 2 retries with backoff on 429/5xx/network, 1 s
  between requests to the same host, robots.txt honoured (read once per host per run).
- Code.gs `GET mode=raw_item_refs`: stored refs per source in one call. Adapters open only items
  not stored yet, at most `WebPage.MaxNewItems` (25) per source per run.
- `WebPage:` section in `config.yaml`; `golang.org/x/net` for HTML parsing.
- 16 adapter tests and 1 Code.gs test, on hand-written HTML test pages in `testdata/webpage/`: the
  markup each rule reads, around an invented brand and text. No copies of live pages are committed
  (rule 12 in `.claude/rules/adapters.md`); live captures go in the gitignored
  `places-fetcher/testdata/live/`, and dry-run CSVs (`raw_items*.csv`) are gitignored too.

Changed (fixtures)
- `testdata/promo_items_2026-11-01.json` (from M2) now holds invented items and `example` URLs
  instead of summaries of real posts.

Changed
- `promo.SourceAdapter.FetchItems` takes a `models.FetchRequest` (the source plus its known refs).
  The usecase also drops any returned item whose ref is already stored.

Checked live (dry run, 1 Nov 2026, nothing written): 67 new items from 5 sources in 73 s:
BCA 12, katalogpromosi RamenYa 11 and HokBen 20, giladiskon RamenYa 11 and Chatime 13; 0 failures.
katalogpromosi posts run 3,200–5,900 characters, so `raw_items`' 2,000-character cap keeps the
summary and details but cuts the FAQ at the end.

Deploy
1. Paste `apps-script/Code.gs` into the Apps Script editor and save.
2. Deploy → Manage deployments → edit → Version: New version → Deploy.
No sheet change and no workflow yet; a scheduled run comes with the workflow milestone.

Roll back: `git revert promo-m3`; in Apps Script, redeploy the previous version. An M2 build
calling an M3 Code.gs is fine (the new mode is only read when asked for); an M3 build needs the
M3 Code.gs for live runs (`-sources-file` dry runs don't call it).

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
