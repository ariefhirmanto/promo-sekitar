import type { Metadata, Viewport } from "next";
import { Bricolage_Grotesque, Figtree } from "next/font/google";
import PwaSetup from "@/components/PwaSetup";
import "./globals.css";

const display = Bricolage_Grotesque({
  variable: "--font-display",
  subsets: ["latin"],
  weight: ["700", "800"],
});

const body = Figtree({
  variable: "--font-body",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Promo Sekitar",
  description: "Promo makanan di outlet dekatmu",
  applicationName: "Promo Sekitar",
  // iOS home-screen app: full screen, its own name under the icon (icon: app/apple-icon.png)
  appleWebApp: { capable: true, title: "Promo Sekitar", statusBarStyle: "default" },
  formatDetection: { telephone: false },
};

export const viewport: Viewport = {
  themeColor: "#2e6b3f",
  viewportFit: "cover",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="id" className={`${display.variable} ${body.variable}`}>
      <body>
        <header className="top">
          <div className="top-inner">
            <strong>Promo Sekitar</strong>
            <span>promo makan di dekatmu</span>
          </div>
        </header>
        <main>{children}</main>
        <PwaSetup />
      </body>
    </html>
  );
}
