import { describe, expect, it } from "vitest";
import { MAX_LIMIT, parseFeedQuery } from "./feedQuery";

const parse = (qs: string) => parseFeedQuery(new URLSearchParams(qs));

describe("parseFeedQuery", () => {
  it("defaults to the first swipe page without location", () => {
    expect(parse("")).toEqual({ ok: true, query: { mode: "swipe", cells: "", offset: 0, limit: 10, brand: "" } });
  });

  it("passes a brand id through, and an empty brand means every brand", () => {
    expect(parse("brand=brand_hokben").ok && parse("brand=brand_hokben")).toMatchObject({ query: { brand: "brand_hokben" } });
    expect(parse("brand=").ok && parse("brand=")).toMatchObject({ query: { brand: "" } });
  });

  it("puts cells in a fixed order so one area is one cache entry", () => {
    const a = parse("cells=1:888c107883fffff,0:888c107881fffff");
    const b = parse("cells=0:888c107881fffff,1:888c107883fffff");
    expect(a).toEqual(b);
    expect(a.ok && a.query.cells).toBe("0:888c107881fffff,1:888c107883fffff");
  });

  it.each([
    ["mode=admin", "mode"],
    ["cells=0:nothex", "bad cell"],
    ["cells=0:888c107883fffff,1:1", "bad cell"],
    ["cells=0:888c107883fffff,999:888c107881fffff", "bad cell"],
    ["offset=-1", "offset"],
    ["offset=abc", "offset"],
    [`limit=${MAX_LIMIT + 1}`, "limit"],
    ["limit=0", "limit"],
    ["brand=hokben", "brand"],
    ["brand=brand_Hok%20Ben", "brand"],
    [`brand=brand_${"x".repeat(61)}`, "brand"],
  ])("rejects %s", (qs, word) => {
    const r = parse(qs);
    expect(r.ok).toBe(false);
    expect(!r.ok && r.error).toContain(word);
  });

  it("caps the number of cells", () => {
    const many = Array.from({ length: 128 }, () => "1:888c107883fffff").join(",");
    expect(parse(`cells=${many}`).ok).toBe(false);
  });
});
