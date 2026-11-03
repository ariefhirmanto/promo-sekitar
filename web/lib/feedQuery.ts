import type { FeedQuery } from "./types";

export const MAX_LIMIT = 50; // same cap as Code.gs
export const MAX_CELLS = 127; // gridDisk k = 6; the app asks for k = 2, or 4 when widened
const CELL = /^(\d{1,2}):([0-9a-f]{15})$/;
const BRAND = /^brand_[a-z0-9_]{1,60}$/; // the brands tab's ids

export type ParsedQuery =
  | { ok: true; query: FeedQuery }
  | { ok: false; error: string };

// parseFeedQuery checks everything the browser sends before it reaches Apps Script, and puts
// cells in a fixed order so the same area always hits the same cache entry.
export function parseFeedQuery(params: URLSearchParams): ParsedQuery {
  const mode = params.get("mode") ?? "swipe";
  if (mode !== "swipe" && mode !== "nearby") {
    return { ok: false, error: "mode must be swipe or nearby" };
  }

  const rawCells = (params.get("cells") ?? "").trim().toLowerCase();
  const pairs = rawCells === "" ? [] : rawCells.split(",");
  if (pairs.length > MAX_CELLS) {
    return { ok: false, error: `at most ${MAX_CELLS} cells` };
  }
  const cells: string[] = [];
  for (const pair of pairs) {
    const m = CELL.exec(pair.trim());
    if (!m) return { ok: false, error: `bad cell: ${pair.slice(0, 40)}` };
    cells.push(`${Number(m[1])}:${m[2]}`);
  }
  cells.sort();

  const offset = intParam(params.get("offset"), 0);
  const limit = intParam(params.get("limit"), 10);
  if (offset === null || offset < 0 || offset > 10_000) {
    return { ok: false, error: "offset must be 0 to 10000" };
  }
  if (limit === null || limit < 1 || limit > MAX_LIMIT) {
    return { ok: false, error: `limit must be 1 to ${MAX_LIMIT}` };
  }

  const brand = (params.get("brand") ?? "").trim();
  if (brand !== "" && !BRAND.test(brand)) {
    return { ok: false, error: `bad brand: ${brand.slice(0, 40)}` };
  }

  return { ok: true, query: { mode, cells: cells.join(","), offset, limit, brand } };
}

function intParam(raw: string | null, fallback: number): number | null {
  if (raw === null || raw === "") return fallback;
  return /^\d+$/.test(raw) ? Number(raw) : null;
}
