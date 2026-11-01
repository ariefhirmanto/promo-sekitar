"use client";

import { useEffect, useState, useSyncExternalStore } from "react";
import styles from "./PwaSetup.module.css";

const DISMISS_KEY = "ps-install-hint-dismissed";

// Registers the service worker in production builds (in `next dev` it would cache stale code),
// and on iPhone/iPad shows how to install, since iOS has no install prompt of its own.
// Android Chrome offers "Install app" by itself once the manifest is valid.
export default function PwaSetup() {
  useEffect(() => {
    if (process.env.NODE_ENV !== "production" || !("serviceWorker" in navigator)) return;
    navigator.serviceWorker.register("/sw.js", { scope: "/", updateViaCache: "none" }).catch(() => {});
  }, []);

  const needsHint = useSyncExternalStore(noSubscribe, iosHintNeeded, () => false);
  const [dismissed, setDismissed] = useState(false);

  if (!needsHint || dismissed) return null;
  return (
    <aside className={styles.hint} role="note">
      <p>
        Pasang di iPhone: ketuk <strong>Bagikan</strong>, lalu <strong>Tambah ke Layar Utama</strong>.
      </p>
      <button
        className={styles.close}
        aria-label="Tutup petunjuk"
        onClick={() => {
          try {
            localStorage.setItem(DISMISS_KEY, "1");
          } catch {}
          setDismissed(true);
        }}
      >
        ×
      </button>
    </aside>
  );
}

const noSubscribe = () => () => {};

function iosHintNeeded(): boolean {
  const ios =
    /iPad|iPhone|iPod/.test(navigator.userAgent) ||
    (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1); // iPadOS reports as a Mac
  const installed =
    window.matchMedia("(display-mode: standalone)").matches ||
    (navigator as Navigator & { standalone?: boolean }).standalone === true;
  let dismissed = false;
  try {
    dismissed = localStorage.getItem(DISMISS_KEY) === "1";
  } catch {}
  return ios && !installed && !dismissed;
}
