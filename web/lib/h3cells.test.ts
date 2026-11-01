import { describe, expect, it } from "vitest";
import { parseFeedQuery } from "./feedQuery";
import { distanceM, formatDistance, ringCells, walkLabel } from "./h3cells";

describe("ringCells", () => {
  // Same point and cell as the example outlet row in the sheet (and the Go fetcher's H3 test).
  const cells = ringCells(-6.2255, 106.799, 2);

  it("puts the user's own cell in ring 0", () => {
    expect(cells.split(",")[0]).toBe("0:888c107883fffff");
  });

  it("returns 1 + 6 + 12 cells for two rings", () => {
    const rings = cells.split(",").map((p) => p.split(":")[0]);
    expect(rings.filter((r) => r === "0")).toHaveLength(1);
    expect(rings.filter((r) => r === "1")).toHaveLength(6);
    expect(rings.filter((r) => r === "2")).toHaveLength(12);
  });

  it("is accepted by the API's query check", () => {
    expect(parseFeedQuery(new URLSearchParams({ cells: ringCells(-6.2255, 106.799, 4) })).ok).toBe(true);
  });
});

describe("distance", () => {
  it("measures and formats", () => {
    const m = distanceM(-6.2255, 106.799, -6.2255, 106.805); // ~663 m east
    expect(m).toBeGreaterThan(640);
    expect(m).toBeLessThan(690);
    expect(formatDistance(m)).toMatch(/^\d+ m$/);
    expect(formatDistance(2345)).toBe("2,3 km");
  });

  it.each([
    [30, "1 mnt jalan kaki"], // never "0 mnt"
    [650, "8 mnt jalan kaki"],
    [2000, "25 mnt jalan kaki"],
    [2001, ""], // too far to suggest walking
  ])("walkLabel(%i m)", (m, want) => {
    expect(walkLabel(m)).toBe(want);
  });
});
