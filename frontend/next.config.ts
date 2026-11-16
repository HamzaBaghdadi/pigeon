import type { NextConfig } from "next";

// Pigeon's UI is a static export embedded in the Go binary by Wails.
// There is no Node server at runtime: no SSR, API routes or server actions.
const nextConfig: NextConfig = {
  output: "export",
  distDir: "dist",
  trailingSlash: true, // /settings -> settings/index.html, which the Wails asset server resolves
  images: { unoptimized: true },
  devIndicators: false,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
