import { gridDiskDistances, latLngToCell } from "h3-js";

export const H3_RES = 8; // outlets.h3_r8; a cell is about 0.7 km² (≈ 460 m edge)

// ringCells turns the phone's position into Code.gs's `cells` parameter: every cell within k
// rings, tagged with its ring. Coordinates never leave the browser; only cell ids do.
export function ringCells(lat: number, lng: number, k: number): string {
  const origin = latLngToCell(lat, lng, H3_RES);
  return gridDiskDistances(origin, k)
    .flatMap((ring, i) => ring.map((cell) => `${i}:${cell}`))
    .join(",");
}

// distanceM is the straight-line distance in metres, for "650 m" on a card.
export function distanceM(lat1: number, lng1: number, lat2: number, lng2: number): number {
  const r = 6371000;
  const rad = Math.PI / 180;
  const dLat = (lat2 - lat1) * rad;
  const dLng = (lng2 - lng1) * rad;
  const a =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(lat1 * rad) * Math.cos(lat2 * rad) * Math.sin(dLng / 2) ** 2;
  return 2 * r * Math.asin(Math.sqrt(a));
}

export function formatDistance(m: number): string {
  if (m < 1000) return `${Math.round(m / 10) * 10} m`;
  return `${(m / 1000).toFixed(1).replace(".", ",")} km`;
}
