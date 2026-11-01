import type { FeedItem, FeedQuery, FeedResponse } from "./types";

// Sample promos for FEED_MOCK=1, taken from the hand-entered rows of the sheet, so the UI can be
// built without Apps Script. The outlet is the sheet's example row (approximate location).
const outlet = {
  id: "out_example_1",
  name: "HokBen (example)",
  address: "Example only",
  lat: -6.2255,
  lng: 106.799,
};

export const MOCK_ITEMS: FeedItem[] = [
  {
    promo_id: "promo_hokben_bca28",
    brand: "HokBen",
    title: "Diskon 28% dengan BCA",
    deal: "Cek syarat di halaman promo BCA",
    channel: "dine_in",
    valid_from: "2026-10-27",
    valid_until: "2026-11-19",
    action_url: "https://promo.bca.co.id/id/all/c/fnb",
    outlet,
    ring: 0,
  },
  {
    promo_id: "promo_hokben_payday",
    brand: "HokBen",
    title: "It's Payday: Paket A + D atau B + C Rp50 ribuan/orang",
    deal: "Termasuk 2 Tehbotol Sosro, harga sebelum pajak",
    channel: "all",
    valid_from: "2026-10-27",
    valid_until: "2026-11-02",
    action_url: "https://www.instagram.com/p/De5qfVtkzzU/",
    outlet,
    ring: 0,
  },
  {
    promo_id: "promo_hokben_krom",
    brand: "HokBen",
    title: "Cashback 10% hingga Rp30.000 dengan Krom Bank",
    deal: "Min. transaksi Rp100.000",
    channel: "dine_in",
    valid_from: "2026-10-29",
    valid_until: "2026-11-02",
    action_url: "https://www.instagram.com/p/De_XSXaI1uJ/",
    outlet,
    ring: 0,
  },
  {
    promo_id: "promo_marugame_bca25",
    brand: "Marugame Udon",
    title: "Diskon 25% dengan BCA",
    deal: "Cek syarat di halaman promo BCA",
    channel: "dine_in",
    valid_from: "2026-11-01",
    valid_until: "2026-11-21",
    action_url: "https://promo.bca.co.id/id/all/c/fnb",
    outlet: null,
    ring: null,
  },
];

// mockFeed pages through MOCK_ITEMS like Code.gs: the deck wraps to 0 after the last page.
export function mockFeed(q: FeedQuery): FeedResponse {
  if (q.mode === "nearby") {
    if (!q.cells) return { ok: true, mode: "nearby", items: [], needs_location: true };
    return { ok: true, mode: "nearby", items: MOCK_ITEMS.filter((i) => i.outlet) };
  }
  const items = MOCK_ITEMS.slice(q.offset, q.offset + q.limit);
  const next = q.offset + q.limit < MOCK_ITEMS.length ? q.offset + q.limit : 0;
  return { ok: true, mode: "swipe", total: MOCK_ITEMS.length, next_offset: next, items };
}
