---
name: add-source-type
description: Teach the MakanDiMana promo fetcher to read a new kind of source - a new host rule for web_page (a site whose pages it can't read well) or a new source type under promo/source (e.g. instagram, a JSON API, headless rendering). Use when a promo page fails or gives messy items with the generic rule, or the user wants a source that isn't a web page.
---

# Add a source type or host rule

The promo job reads every `sources` row through the adapter registered for its `type`
(`promo/source/<type>/`, port `promo.SourceAdapter`). Everything after fetching works on
`models.RawItem`, so a new kind of source never touches the usecase, the parser or Code.gs.
Read `.claude/rules/adapters.md` (the contract) and `promo/source/webpage/` (the reference
adapter) first.

Pick the smallest change:

| Need | Change |
|---|---|
| A site with a listing of posts or deals, read badly by the generic rule (one item for the whole page, boilerplate, a ref that changes every run) | **A host rule** in `web_page` (section A) |
| Something that isn't an HTML page: Instagram, a JSON API, a feed, a page that needs a browser | **A new source type** (section B) |
| A page the existing rules already read well | No code: the `add-promo-source` skill |

## Rules for both

1. **Items have a stable `Ref`**: the site's own post, deal or promo id, or a slug. Never a
   position in a list or the fetch time. Raw item ids are `<source id>:<ref>`, and a ref that
   changes makes the same post a new item, and a new parser call, every run.
2. **Skip what is stored.** `req.KnownRefs` lists the refs already in `raw_items`: list them if
   cheap, but never open their pages. Open at most `WebPage.MaxNewItems` (or your type's limit)
   new items a run.
3. **`Text` is the promo**: the title first, then the body, without navigation, share buttons,
   related posts, banners or footers. The sheet keeps 6,000 characters. Set `PostedAt` (RFC 3339)
   when the site shows a date.
4. **Fail loudly.** A listing that no longer has the expected structure, a login wall, or every
   item page failing returns an error, so the source's `error_count` rises and the user sees
   `last_error`. One item page failing is logged and skipped (it is tried again next run). Never
   return an empty or partial result as success.
5. **Be a good client**: the shared HTTP client (`web_page`) or your own with `ctx`, a timeout,
   retries with backoff, `Promo.UserAgent`, 1 request a second per host, and robots.txt. A
   robots.txt refusal is final; don't work around it.
6. **Test pages are hand-written** (rule 12 in `.claude/rules/adapters.md`): the markup or JSON
   your code reads, around an invented brand ("Mie Contoh") and invented text, `example` URLs where
   the URL isn't what's checked. Never paste in a live page and edit it down, and never commit a
   summary of one. Live captures go in the gitignored `places-fetcher/testdata/live/`.
7. **Nothing new in the models without asking.** If the source has something `RawItem` can't hold
   (images, for example), propose a vendor-neutral field to the user first.

## A. A host rule for `web_page`

Files in `promo/source/webpage/`:

1. `<host>.go` with a `<host>Rule` implementing `rule`:
   - fetch `target` with `get`, parse it (`parseHTML`), find the listing by a stable id or class
     (`findFirst`, `findAll`, `byID`, `byClass`); missing → `fmt.Errorf("<host> %s: no <what>,
     the page layout changed", target)`;
   - build one `detail{ref, url, item}` per entry, then `openDetails(ctx, get, req.Source,
     req.KnownRefs, dedupe(list), maxNew)`; `item` turns one detail page into a `RawItem` with
     `joinText(title, body)` and `text(node, skip)` (see `katalogBoilerplate` for skipping blocks);
   - a site whose listing already holds the whole text (like BCA's cards) can build items
     without opening detail pages.
2. An entry in `rules` in `webpage.go`, keyed by the host without `www.`, and a line in the
   package comment.
3. Hand-written test pages `testdata/webpage/<host>_listing.html` and `<host>_<item>.html`, and
   tests in `webpage_test.go` with `fx(name)` and the `fileFetcher` (copy the giladiskon tests):
   the listing gives the right refs, URLs and texts; known refs aren't opened; `MaxNewItems` is
   honoured; a changed layout is an error; one failing item page is skipped; all failing is an error.
4. The host table in `places-fetcher/README.md`.

## B. A new source type

1. `promo/source/<type>/`, laid out like an adapter (`.claude/rules/adapters.md`, "Files"):
   - `<type>.go`: `const Name = "<type>"` (the `type` column value), `init()` calling
     `source.Register(Name, factory)`, `New(cfg…)` that fails without required config (an empty
     token), `Type()` and `FetchItems(ctx, models.FetchRequest) (models.FetchResult, error)`;
   - `client.go`: the HTTP client and a file client for replay; `transform.go`: response →
     `models.RawItem`, vendor types unexported;
   - for sources that page by "newer than", return `LastSeenRef` (the newest ref); the fetcher
     stores it in the `sources` row and sends it back as `req.Source.LastSeenRef`.
2. A `<Type>Config` in `config/config.go` and a `<Type>:` section in `config.yaml`; tokens empty
   there, from env (bind an unusual env name in `LoadConfig` like `ANTHROPIC_API_KEY`), and as a
   secret in `.github/workflows/promos_daily.yml`. Tell the user which secret to create.
3. Blank-import the package in `job/promos.go`.
4. Tests in the package: registered through `source.NewSet` (`ForType("<type>")`), the transform
   of a small hand-written response (every `RawItem` field), known refs skipped, and at least one
   error response (expired token, rate limit, changed shape).
5. A "The `<type>` source type" section in `places-fetcher/README.md`: config, secrets, limits,
   known gaps. Cost check first (rule 11) if the service bills per call.

Code.gs needs no change: `type` is a free-text column, and raw items look the same for every type.

## Verify

From `~/promo/places-fetcher` in WSL:

```bash
gofmt -l . && go vet ./... && go test ./...
# layer check from .claude/rules/clean-architecture.md: must print "layers ok"
go list -f '{{.ImportPath}}: {{join .Imports " "}}' $(go list ./... | grep -E '/(usecase|controller)$') \
  | grep -E 'repository|provider|/source|/parser|places-fetcher/config|places-fetcher/job' \
  && echo "layer violation" || echo "layers ok"
```

Then a live dry run of a real source, fetch only, as in the `add-promo-source` skill step 3
(`-dry-run -parse=false -sources-file … -source <id>`): item count, clean text, `posted_at`, refs
the same on a second run. Report it to the user, then hand over the `sources` row through that
skill. Nothing fetched is committed, and never run without `-dry-run` unless the user asks.
