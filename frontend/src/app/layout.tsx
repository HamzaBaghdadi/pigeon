import type { Metadata } from "next";
import "@fontsource-variable/inter"; // bundled locally: no requests to font CDNs
import "./globals.css";
import { THEME_STORAGE_KEY } from "@/lib/theme";

export const metadata: Metadata = {
  title: "Pigeon",
  description: "A secure, open-source email client",
};

// Runs before first paint so a forced light/dark theme never flashes the
// other one. The Go settings file stays the source of truth; this is a cache.
const themeScript = `try{var t=localStorage.getItem(${JSON.stringify(THEME_STORAGE_KEY)});if(t==="light"||t==="dark")document.documentElement.dataset.theme=t}catch(e){}`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>{children}</body>
    </html>
  );
}
