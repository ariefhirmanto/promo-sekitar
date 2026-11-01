import type { NextRequest } from "next/server";
import { getFeed } from "@/lib/feed";
import { parseFeedQuery } from "@/lib/feedQuery";

// GET /api/feed?mode=swipe|nearby&cells=0:…,1:…&offset=0&limit=10
// The browser's only way to the feed: it never sees the Apps Script URL or READ_TOKEN.
export async function GET(request: NextRequest) {
  const parsed = parseFeedQuery(request.nextUrl.searchParams);
  if (!parsed.ok) {
    return Response.json({ ok: false, error: parsed.error }, { status: 400 });
  }

  try {
    return Response.json(await getFeed(parsed.query));
  } catch (err) {
    // Details stay in the server log; they can include the upstream response.
    console.error("[Feed][GET][Route]", err);
    return Response.json({ ok: false, error: "feed unavailable" }, { status: 502 });
  }
}
