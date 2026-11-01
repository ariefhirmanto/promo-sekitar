"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { distanceM, formatDistance, ringCells } from "@/lib/h3cells";
import { channelLabel, promoStatus, todayJakarta } from "@/lib/promoStatus";
import type { FeedError, FeedItem, FeedResponse } from "@/lib/types";
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
  const dragFrom = useRef<number | null>(null);
  const dragged = useRef(false);

  const loadPage = useCallback(async (forCells: string, offset: number) => {
    setPhase("loading");
    try {
      const qs = new URLSearchParams({ mode: "swipe", cells: forCells, offset: String(offset), limit: String(PAGE) });
      const res = await fetch(`/api/feed?${qs}`);
      const data = (await res.json()) as FeedResponse | FeedError;
      if (!data.ok) throw new Error(data.error);
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
      {note && <p className={styles.note}>{note}</p>}

      {(phase === "locating" || phase === "loading") && (
        <p className={styles.state}>{phase === "locating" ? "Mencari lokasimu…" : "Memuat promo…"}</p>
      )}
      {phase === "error" && (
        <div className={styles.state}>
          <p>Promo belum bisa dimuat.</p>
          <button className={styles.link} onClick={() => void loadPage(cells, 0)}>Coba lagi</button>
        </div>
      )}
      {phase === "empty" && (
        <div className={styles.state}>
          <p>Belum ada promo di sekitar sini.</p>
          {pos && rings < 4 && (
            <button className={styles.link} onClick={() => locate(4)}>Perluas area</button>
          )}
        </div>
      )}

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
            <p className={styles.deal}>
              <strong>{card.title}.</strong> {card.deal}
            </p>
            <ul className={styles.tags}>
              <li>{channelLabel(card.channel)}</li>
              {card.outlet && (
                <li>
                  {card.outlet.name}
                  {pos && ` · ${formatDistance(distanceM(pos.lat, pos.lng, card.outlet.lat, card.outlet.lng))}`}
                </li>
              )}
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

function Stamp({ item, today }: { item: FeedItem; today: string }) {
  const s = promoStatus(item, today);
  return <span className={`${styles.stamp} ${styles[s.kind]}`}>{s.label}</span>;
}
