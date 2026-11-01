import type { MetadataRoute } from "next";

// Served at /manifest.webmanifest and linked from every page; makes the app installable.
export default function manifest(): MetadataRoute.Manifest {
  return {
    id: "/",
    name: "Promo Sekitar",
    short_name: "Promo Sekitar",
    description: "Promo makanan di outlet dekatmu",
    lang: "id",
    start_url: "/",
    scope: "/",
    display: "standalone",
    orientation: "portrait",
    background_color: "#e4cfa8",
    theme_color: "#2e6b3f",
    categories: ["food", "lifestyle", "shopping"],
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
      { src: "/icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
}
