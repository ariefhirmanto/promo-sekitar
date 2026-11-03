import type { FeedBrand, FeedItem, FeedQuery, FeedResponse } from "./types";

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

// Feed items carry the brand's name only; the mock knows the ids of its two brands.
const MOCK_BRAND_IDS: Record<string, string> = { HokBen: "brand_hokben", "Marugame Udon": "brand_marugame" };

// The brand picker list, like Code.gs: brands with promos, by name.
function mockBrands(): FeedBrand[] {
  const count = new Map<string, number>();
  for (const i of MOCK_ITEMS) count.set(i.brand, (count.get(i.brand) ?? 0) + 1);
  return [...count]
    .map(([name, n]) => ({ id: MOCK_BRAND_IDS[name], name, count: n }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

// mockFeed pages through MOCK_ITEMS like Code.gs: the deck wraps to 0 after the last page.
export function mockFeed(q: FeedQuery): FeedResponse {
  const brands = mockBrands();
  const pool = q.brand ? MOCK_ITEMS.filter((i) => MOCK_BRAND_IDS[i.brand] === q.brand) : MOCK_ITEMS;
  if (q.mode === "nearby") {
    if (!q.cells) return { ok: true, mode: "nearby", items: [], needs_location: true, brands };
    return { ok: true, mode: "nearby", items: pool.filter((i) => i.outlet), brands };
  }
  const items = pool.slice(q.offset, q.offset + q.limit);
  const next = q.offset + q.limit < pool.length ? q.offset + q.limit : 0;
  return { ok: true, mode: "swipe", total: pool.length, next_offset: next, items, brands };
}
