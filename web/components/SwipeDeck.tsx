"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { distanceM, formatDistance, ringCells, walkLabel } from "@/lib/h3cells";
import { directionsUrl } from "@/lib/maps";
import { channelLabel, promoStatus, todayJakarta } from "@/lib/promoStatus";
import type { FeedBrand, FeedError, FeedItem, FeedResponse } from "@/lib/types";
import styles from "./SwipeDeck.module.css";

type Position = { lat: number; lng: number };
type Phase = "locating" | "loading" | "ready" | "empty" | "error";

const PAGE = 10;
const SWIPE_PX = 90;
const TAP_PX = 8; // a pointer that moved further than this was a drag, not a tap

export default function SwipeDeck() {
  const [phase, setPhase] = useState<Phase>("locating");
  const [pos, setPos] = useState<Position | null>(null);
  const [cells, setCells] = useState("");
  const [rings, setRings] = useState(2);
  const [items, setItems] = useState<FeedItem[]>([]);
  const [idx, setIdx] = useState(0);
  const [nextOffset, setNextOffset] = useState(0);
  const [note, setNote] = useState("");
  const [today, setToday] = useState(""); // set when a page loads; render may not read the clock
  const [dx, setDx] = useState(0);
  const [brands, setBrands] = useState<FeedBrand[]>([]);
  const [brand, setBrand] = useState(""); // brand_* id; "" = every brand
  const brandRef = useRef(""); // loadPage reads the pick from here, so locate's callbacks see it
  const dragFrom = useRef<number | null>(null);
  const dragged = useRef(false);

  const loadPage = useCallback(async (forCells: string, offset: number) => {
    setPhase("loading");
    try {
      const qs = new URLSearchParams({ mode: "swipe", cells: forCells, offset: String(offset), limit: String(PAGE) });
      if (brandRef.current) qs.set("brand", brandRef.current);
      const res = await fetch(`/api/feed?${qs}`);
      const data = (await res.json()) as FeedResponse | FeedError;
      if (!data.ok) throw new Error(data.error);
      if (data.brands) setBrands(data.brands);
      setToday(todayJakarta());
      setItems(data.items);
      setIdx(0);
      setNextOffset(data.next_offset ?? 0);
      setPhase(data.items.length ? "ready" : "empty");
    } catch {
      setPhase("error");
    }
  }, []);

  // locate only sets state from callbacks, so calling it from the mount effect doesn't re-render
  // synchronously (react-hooks/set-state-in-effect).
  const locate = useCallback(
    (k: number) => {
      const withoutLocation = (message: string) => {
        setNote(message);
        void loadPage("", 0);
      };
      if (!("geolocation" in navigator)) {
        setTimeout(() => withoutLocation("Browser ini tidak mendukung lokasi; promo diurutkan dari yang terbaru."), 0);
        return;
      }
      navigator.geolocation.getCurrentPosition(
        (p) => {
          const here = { lat: p.coords.latitude, lng: p.coords.longitude };
          const c = ringCells(here.lat, here.lng, k);
          setRings(k);
          setPos(here);
          setCells(c);
          setNote("");
          void loadPage(c, 0);
        },
        () => withoutLocation("Lokasi tidak diizinkan; promo diurutkan dari yang terbaru."),
        { timeout: 10_000, maximumAge: 300_000 },
      );
    },
    [loadPage],
  );

  useEffect(() => {
    locate(2);
  }, [locate]);

  const card = items.at(idx);
  const brandName = brands.find((b) => b.id === brand)?.name ?? "brand ini";

  // A new pick starts the deck over for the same location, nearest outlets first as before.
  const pickBrand = (id: string) => {
    brandRef.current = id;
    setBrand(id);
    void loadPage(cells, 0);
  };

  const vote = (want: boolean) => {
    if (!card) return;
    if (want && card.action_url) window.open(card.action_url, "_blank", "noopener");
    setDx(0);
    if (idx + 1 < items.length) setIdx(idx + 1);
    else void loadPage(cells, nextOffset); // next_offset 0 = the deck starts over
  };

  return (
    <section className={styles.deck} aria-labelledby="deck-title">
      <h1 id="deck-title" className={styles.title}>Mau makan apa?</h1>
      <p className={styles.lede}>
        Geser ke kanan atau ketuk “Mau” untuk membuka promonya. Geser ke kiri untuk lewat.
      </p>
      {brands.length > 0 && (
        <label className={styles.picker}>
          <span>Brand</span>
          <select
            value={brand}
            onChange={(e) => pickBrand(e.target.value)}
            disabled={phase === "locating" || phase === "loading"}
          >
            <option value="">Semua brand</option>
            {brands.map((b) => (
              <option key={b.id} value={b.id}>
                {b.name} ({b.count})
              </option>
            ))}
          </select>
        </label>
      )}
      {note && (
        <p className={styles.note}>
          {note}{" "}
          {"geolocation" in globalThis.navigator ? (
            <button className={styles.link} onClick={() => locate(rings)}>Aktifkan lokasi</button>
          ) : null}
        </p>
      )}

      {(phase === "locating" || phase === "loading") && (
        <p className={styles.state}>{phase === "locating" ? "Mencari lokasimu…" : "Memuat promo…"}</p>
      )}
      {phase === "error" && (
        <div className={styles.state}>
          <p>Promo belum bisa dimuat.</p>
          <button className={styles.link} onClick={() => void loadPage(cells, 0)}>Coba lagi</button>
        </div>
      )}
      {phase === "empty" && (brand ? (
        <div className={styles.state}>
          <p>Belum ada promo {brandName} saat ini.</p>
          <button className={styles.link} onClick={() => pickBrand("")}>Lihat semua brand</button>
        </div>
      ) : (
        <div className={styles.state}>
          <p>Belum ada promo di sekitar sini.</p>
          {pos && rings < 4 && (
            <button className={styles.link} onClick={() => locate(4)}>Perluas area</button>
          )}
        </div>
      ))}

      {phase === "ready" && card && (
        <>
          <article
            className={styles.card}
            tabIndex={0}
            aria-label={`${card.brand}: ${card.title}`}
            style={{ transform: dx ? `translateX(${dx}px) rotate(${dx / 20}deg)` : undefined }}
            onPointerDown={(e) => {
              dragFrom.current = e.clientX;
              dragged.current = false;
              e.currentTarget.setPointerCapture(e.pointerId);
            }}
            onPointerMove={(e) => {
              if (dragFrom.current === null) return;
              const moved = e.clientX - dragFrom.current;
              if (Math.abs(moved) > TAP_PX) dragged.current = true;
              setDx(moved);
            }}
            onClick={() => {
              // A tap on the card works like "Mau"; the end of a drag also fires click, so skip it.
              if (!dragged.current) vote(true);
            }}
            onPointerUp={() => {
              dragFrom.current = null;
              if (Math.abs(dx) > SWIPE_PX) vote(dx > 0);
              else setDx(0);
            }}
            onPointerCancel={() => {
              dragFrom.current = null;
              setDx(0);
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowRight" || e.key === "Enter") vote(true);
              if (e.key === "ArrowLeft") vote(false);
            }}
          >
            <Stamp item={card} today={today} />
            <div className={styles.brand}>{card.brand}</div>
            <Distance outlet={card.outlet} pos={pos} />
            <p className={styles.deal}>
              <strong>{card.title}.</strong> {card.deal}
            </p>
            <ul className={styles.tags}>
              <li>{channelLabel(card.channel)}</li>
              {card.other_promos ? <li>+{card.other_promos} promo lain</li> : null}
            </ul>
          </article>
          <div className={styles.actions}>
            <button className={styles.no} onClick={() => vote(false)}>Lewat</button>
            <button className={styles.yes} onClick={() => vote(true)}>Mau</button>
          </div>
        </>
      )}
    </section>
  );
}

// Distance is measured on the phone, from its own position to the outlet's coordinates, so it's
// exact (straight line) rather than the ring band the server sorted by.
function Distance({ outlet, pos }: { outlet: FeedItem["outlet"]; pos: Position | null }) {
  if (!pos) {
    return (
      <p className={`${styles.distance} ${styles.unknown}`}>
        <PinIcon /> Jarak muncul setelah lokasi aktif
      </p>
    );
  }
  if (!outlet) {
    return (
      <p className={`${styles.distance} ${styles.unknown}`}>
        <PinIcon /> Belum ada outlet di dekatmu
      </p>
    );
  }
  const m = distanceM(pos.lat, pos.lng, outlet.lat, outlet.lng);
  const walk = walkLabel(m);
  return (
    <p className={styles.distance}>
      <PinIcon />
      <strong>{formatDistance(m)}</strong>
      {walk && <span>· {walk}</span>}
      <a
        className={styles.route}
        href={directionsUrl(outlet, walk !== "")}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={`Rute ke ${outlet.name} di Google Maps`}
        // The card is tappable ("Mau") and draggable: keep this link's taps, drags and Enter to itself.
        onPointerDown={(e) => e.stopPropagation()}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => e.stopPropagation()}
      >
        Rute ↗
      </a>
      <span className={styles.outletName}>{outlet.name}</span>
    </p>
  );
}

function PinIcon() {
  return (
    <svg className={styles.pin} viewBox="0 0 24 24" aria-hidden="true">
      <path d="M12 2C8.1 2 5 5 5 8.9 5 14.1 12 22 12 22s7-7.9 7-13.1C19 5 15.9 2 12 2zm0 9.6a2.6 2.6 0 1 1 0-5.2 2.6 2.6 0 0 1 0 5.2z" fill="currentColor" />
    </svg>
  );
}

function Stamp({ item, today }: { item: FeedItem; today: string }) {
  const s = promoStatus(item, today);
  return <span className={`${styles.stamp} ${styles[s.kind]}`}>{s.label}</span>;
}
