"use client";

import dynamic from "next/dynamic";

// The mailbox talks to Go through the Wails runtime, which only exists in the
// app's webview. Rendering it client-side only keeps the static export build
// from touching it.
const Mailbox = dynamic(() => import("@/components/Mailbox"), { ssr: false });

export default function Home() {
  return <Mailbox />;
}
