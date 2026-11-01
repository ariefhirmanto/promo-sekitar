# web

The Promo Sekitar app: Next.js 16 (App Router, Cache Components). The browser never talks to
Apps Script; it calls this app's own `/api/feed`, and the server adds the token.

```
Browser                               Next.js server                       Apps Script
  location → h3-js ring cells
  GET /api/feed?mode&cells… ────────▶ app/api/feed/route.ts
                                      parseFeedQuery (validate)
                                      getFeed ('use cache', 5 min)
                                      + READ_TOKEN ─────────────────────▶ doGet → JSON
  cards, distance from outlet ◀────── JSON only
```

| File | Role |
|---|---|
| `app/api/feed/route.ts` | The only feed endpoint the browser can reach |
| `lib/feed.ts` | `server-only`: calls Apps Script with `READ_TOKEN`, cached per query |
| `lib/feedQuery.ts` | Validates `mode`, `cells`, `offset`, `limit`; sorts cells for caching |
| `lib/h3cells.ts` | Browser side: position → ring cells (res 8), distance |
| `lib/promoStatus.ts` | Card stamp ("s.d. 19 Nov", "Hari terakhir") in Jakarta time |
| `lib/mockFeed.ts` | Sample promos for `FEED_MOCK=1` |
| `components/SwipeDeck.tsx` | "Mau makan apa?" deck: swipe, keys, paging with wrap-around |

## Run

Node comes from nvm in WSL (`. ~/.nvm/nvm.sh` in non-interactive shells).

```bash
cp .env.example .env.local   # fill APPS_SCRIPT_URL + READ_TOKEN, or set FEED_MOCK=1
npm run dev                  # http://localhost:3000
npm test && npx eslint . && npx tsc --noEmit && npx next build
```

## Install as an app (PWA)

| File | Role |
|---|---|
| `app/manifest.ts` | Name, colours, `standalone` display, icons → `/manifest.webmanifest` |
| `public/icon-*.png`, `app/icon.png`, `app/apple-icon.png` | Rendered by `node scripts/make-icons.mjs` from one SVG |
| `public/sw.js` | Offline start: pages network-first, `/_next/static` cache-first, `/api` never cached. Bump `VERSION` when changing it. |
| `components/PwaSetup.tsx` | Registers the service worker (production only) and shows the iPhone install hint |
| `next.config.ts` | Security headers; `sw.js` is never cached |

Installing needs HTTPS (any Vercel URL works; `localhost` only on the same computer):

- **Android (Chrome):** menu ⋮ → **Install app** (Chrome may also offer it on its own).
- **iPhone (Safari):** **Share** → **Add to Home Screen**. The app shows this hint itself on iOS
  until dismissed. Location permission is asked again inside the installed app.

## Deploy (Vercel)

Root directory `web`. Environment variables `APPS_SCRIPT_URL` and `READ_TOKEN`, **without**
`NEXT_PUBLIC_`: that prefix copies a value into the browser bundle.

## Not yet

- Login: the app is open to anyone with its URL (the sheet is not). Planned: Auth.js with Google
  sign-in and an email allowlist in `middleware.ts`.
- Nearby list and "Semua promo" screens from the mockup.
