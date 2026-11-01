import { describe, expect, it } from "vitest";
import { mockFeed, MOCK_ITEMS } from "./mockFeed";
import { promoStatus, todayJakarta } from "./promoStatus";

describe("promoStatus", () => {
  const today = "2026-11-01";
  it.each([
    [{ valid_from: "2026-10-27", valid_until: "2026-11-19" }, "active", "s.d. 19 Nov"],
    [{ valid_from: "2026-10-27", valid_until: "2026-11-01" }, "last", "Hari terakhir"],
    [{ valid_from: "2026-11-06", valid_until: "2026-11-21" }, "soon", "Mulai 6 Nov"],
    [{ valid_from: "", valid_until: "" }, "unknown", "Cek tanggal"],
  ])("%j", (p, kind, label) => {
    expect(promoStatus(p, today)).toEqual({ kind, label });
  });

  it("uses Jakarta's date, not UTC", () => {
    // 20:00 UTC on 1 Nov is 03:00 on 2 Nov in Jakarta.
    expect(todayJakarta(new Date("2026-11-01T20:00:00Z"))).toBe("2026-11-02");
  });
});

describe("mockFeed", () => {
  it("pages and wraps around like Code.gs", () => {
    const first = mockFeed({ mode: "swipe", cells: "", offset: 0, limit: 3 });
    expect(first.items).toHaveLength(3);
    expect(first.next_offset).toBe(3);
    const last = mockFeed({ mode: "swipe", cells: "", offset: 3, limit: 3 });
    expect(last.items).toHaveLength(MOCK_ITEMS.length - 3);
    expect(last.next_offset).toBe(0);
  });

  it("asks for location in nearby mode without cells", () => {
    expect(mockFeed({ mode: "nearby", cells: "", offset: 0, limit: 10 }).needs_location).toBe(true);
  });
});
