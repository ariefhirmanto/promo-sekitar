# promo-sekitar

*Promo sekitar* means "promos around you": find food deals at outlets near where you are, so
eating out costs less.

The MVP collects promos from brand accounts, maps each brand's outlets, and serves a feed sorted
by distance: a swipe deck ("Mau makan apa?") and a nearby list.

## Pieces

| Folder | What it is |
|---|---|
| [`places-fetcher/`](places-fetcher/) | Go job that fills the `outlets` tab from OpenStreetMap: brand matching, mall lookup, H3 cells. Vendor adapters make the data source swappable. |
| [`apps-script/`](apps-script/) | Google Apps Script on the spreadsheet: serves the feed as JSON and accepts fetcher writes. |
| [`.github/workflows/`](.github/workflows/) | Scheduled places refresh (2nd and 16th, 03:00 WIB). |

Google Sheets is the MVP database: `brands` (edited by hand), `outlets` (places fetcher),
`promos` (promo fetcher). See [`places-fetcher/README.md`](places-fetcher/README.md) for setup.

## Data

Outlet and mall data © [OpenStreetMap contributors](https://www.openstreetmap.org/copyright),
available under the Open Database License (ODbL). This includes the sample response in
`places-fetcher/testdata/`.
