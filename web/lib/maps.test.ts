import { describe, expect, it } from "vitest";
import { directionsUrl } from "./maps";

describe("directionsUrl", () => {
  const outlet = { lat: -6.2255, lng: 106.799 };

  it("points at the outlet's coordinates, walking", () => {
    const u = new URL(directionsUrl(outlet, true));
    expect(u.origin + u.pathname).toBe("https://www.google.com/maps/dir/");
    expect(u.searchParams.get("api")).toBe("1");
    expect(u.searchParams.get("destination")).toBe("-6.2255,106.799");
    expect(u.searchParams.get("travelmode")).toBe("walking");
    expect(u.searchParams.has("origin")).toBe(false); // Maps starts from the user's location
  });

  it("lets Maps pick the mode when it's too far to walk", () => {
    expect(new URL(directionsUrl(outlet, false)).searchParams.has("travelmode")).toBe(false);
  });
});
