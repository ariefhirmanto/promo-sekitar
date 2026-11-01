import type { FeedItem } from "./types";

export type PromoStatus = {
  kind: "active" | "last" | "soon" | "unknown";
  label: string;
};

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"];

// todayJakarta is the date in Asia/Jakarta, the sheet's time zone, as yyyy-mm-dd.
export function todayJakarta(now: Date = new Date()): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Jakarta" }).format(now);
}

export function shortDate(iso: string): string {
  const month = MONTHS.at(Number(iso.slice(5, 7)) - 1) ?? "";
  return `${Number(iso.slice(8, 10))} ${month}`;
}

// promoStatus is the stamp on a card. Code.gs already drops expired promos.
export function promoStatus(p: Pick<FeedItem, "valid_from" | "valid_until">, today: string): PromoStatus {
  if (p.valid_from && p.valid_from > today) {
    return { kind: "soon", label: `Mulai ${shortDate(p.valid_from)}` };
  }
  if (!p.valid_until) return { kind: "unknown", label: "Cek tanggal" };
  if (p.valid_until === today) return { kind: "last", label: "Hari terakhir" };
  return { kind: "active", label: `s.d. ${shortDate(p.valid_until)}` };
}

const CHANNELS: Record<string, string> = {
  dine_in: "Dine-in",
  take_away: "Take away",
  delivery: "Delivery",
  all: "Semua channel",
};

export function channelLabel(channel: string): string {
  return CHANNELS[channel] ?? channel;
}
