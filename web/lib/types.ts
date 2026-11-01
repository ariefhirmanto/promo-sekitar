// Shapes of the Apps Script feed (apps-script/Code.gs, card_ / swipe_ / nearby_).
// Shared by the server proxy and the browser, so it must stay free of secrets.

export type FeedMode = "swipe" | "nearby";

export type FeedOutlet = {
  id: string;
  name: string;
  address: string;
  lat: number;
  lng: number;
};

export type FeedItem = {
  promo_id: string;
  brand: string;
  title: string;
  deal: string;
  channel: string;
  valid_from: string; // yyyy-mm-dd or ""
  valid_until: string; // yyyy-mm-dd or "" (end date unknown)
  action_url: string;
  outlet: FeedOutlet | null; // nearest outlet in range, null when none
  ring: number | null; // H3 ring of that outlet: 0 = the user's own cell
  other_promos?: number; // nearby mode: more promos at the same outlet
};

export type FeedResponse = {
  ok: true;
  mode: FeedMode;
  items: FeedItem[];
  total?: number; // swipe
  next_offset?: number; // swipe: 0 means the deck wrapped around
  needs_location?: boolean; // nearby without cells
};

export type FeedError = { ok: false; error: string };

export type FeedQuery = {
  mode: FeedMode;
  cells: string; // "ring:cell,…", canonical order; "" without location
  offset: number;
  limit: number;
};
