import "server-only";
import { cacheLife } from "next/cache";
import { mockFeed } from "./mockFeed";
import type { FeedQuery, FeedResponse } from "./types";

// getFeed calls the Apps Script web app with READ_TOKEN. It runs only on the server: the build
// fails if a client component imports this file, so the token can't reach the browser.
//
// Cached per query (mode, cells, offset, limit) for up to 5 minutes, the same window as Code.gs,
// so a busy area costs one Apps Script call per 5 minutes, not one per swipe.
export async function getFeed(query: FeedQuery): Promise<FeedResponse> {
  "use cache";
  cacheLife({ stale: 60, revalidate: 300, expire: 3600 });

  if (process.env.FEED_MOCK === "1") return mockFeed(query);

  const base = process.env.APPS_SCRIPT_URL;
  const token = process.env.READ_TOKEN;
  if (!base || !token) {
    throw new Error("APPS_SCRIPT_URL and READ_TOKEN must be set (or FEED_MOCK=1)");
  }

  const url = new URL(base);
  url.searchParams.set("token", token);
  url.searchParams.set("mode", query.mode);
  url.searchParams.set("cells", query.cells);
  url.searchParams.set("offset", String(query.offset));
  url.searchParams.set("limit", String(query.limit));

  // Apps Script answers with a redirect to the result; fetch follows it.
  const res = await fetch(url, { signal: AbortSignal.timeout(15_000) });
  const text = await res.text();
  let body: unknown;
  try {
    body = JSON.parse(text);
  } catch {
    // Usually the Google sign-in page: the web app isn't deployed with "Who has access: Anyone".
    throw new Error(`Apps Script HTTP ${res.status}, not JSON: ${text.slice(0, 120)}`);
  }

  const data = body as FeedResponse | { ok: false; error: string };
  if (!data.ok) throw new Error(`Apps Script: ${data.error}`);
  return data;
}
