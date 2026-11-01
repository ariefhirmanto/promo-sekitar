// Renders the app icons from one SVG: a white map pin with a percent sign on banana-leaf green.
// Run after changing the design: node scripts/make-icons.mjs
import sharp from "sharp";

const GREEN = "#2e6b3f";
const CREAM = "#fffcf5";

// The glyph is drawn in a 512 box; `scale` shrinks it toward the centre, so maskable icons keep it
// inside the 80% safe zone that Android may crop to a circle or squircle.
function svg(scale, rounded) {
  const r = rounded ? 112 : 0;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
  <rect width="512" height="512" rx="${r}" fill="${GREEN}"/>
  <g transform="translate(256 256) scale(${scale}) translate(-256 -256)">
    <path d="M256 72c-79.5 0-144 62.6-144 141.5C112 316 256 440 256 440s144-124 144-226.5C400 134.6 335.5 72 256 72z" fill="${CREAM}"/>
    <circle cx="214" cy="178" r="26" fill="none" stroke="${GREEN}" stroke-width="18"/>
    <circle cx="298" cy="262" r="26" fill="none" stroke="${GREEN}" stroke-width="18"/>
    <line x1="306" y1="166" x2="206" y2="274" stroke="${GREEN}" stroke-width="20" stroke-linecap="round"/>
  </g>
</svg>`;
}

const outputs = [
  // Manifest icons (public/, referenced by app/manifest.ts)
  { file: "public/icon-192.png", size: 192, scale: 0.92, rounded: false },
  { file: "public/icon-512.png", size: 512, scale: 0.92, rounded: false },
  { file: "public/icon-maskable-512.png", size: 512, scale: 0.72, rounded: false },
  // Next.js file conventions: app/icon.png → <link rel="icon">, app/apple-icon.png → apple-touch-icon.
  // iOS rounds the corners itself and shows transparency as black, so no rounding there.
  { file: "app/icon.png", size: 64, scale: 1, rounded: true },
  { file: "app/apple-icon.png", size: 180, scale: 0.86, rounded: false },
];

for (const o of outputs) {
  await sharp(Buffer.from(svg(o.scale, o.rounded)), { density: 300 })
    .resize(o.size, o.size)
    .png()
    .toFile(o.file);
  console.log(`${o.file} ${o.size}×${o.size}`);
}
