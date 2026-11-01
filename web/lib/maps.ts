import type { FeedOutlet } from "./types";

// Google Maps URLs (https://developers.google.com/maps/documentation/urls/get-started): plain
// links, no API key or billing. On phones they open the Google Maps app when it's installed.
//
// Directions use the outlet's coordinates as the destination, so the pin is exact even when the
// outlet's name isn't in Google's place data; the origin is left out, which Maps reads as
// "from my current location".
export function directionsUrl(outlet: Pick<FeedOutlet, "lat" | "lng">, walking: boolean): string {
  const params = new URLSearchParams({
    api: "1",
    destination: `${outlet.lat},${outlet.lng}`,
  });
  if (walking) params.set("travelmode", "walking");
  return `https://www.google.com/maps/dir/?${params}`;
}
