"use client";

import { useCallback, useEffect, useRef } from "react";

type Props = {
  /** HTML already sanitized by the Go backend (internal/sanitize). */
  html: string;
  /** Whether the user opted in to remote images for this message. */
  allowRemote: boolean;
  /** Called with the URL of any clicked link; Go opens it in the system browser. */
  onLink: (url: string) => void;
};

/**
 * Renders an email in a sandboxed iframe — the second security layer after
 * server-side sanitizing:
 *  - `sandbox` without `allow-scripts`: nothing in the email can execute.
 *  - Its own CSP: no network requests at all, except images once allowed.
 *  - Links never navigate the app; they open in the user's browser.
 */
export default function MessageBody({ html, allowRemote, onLink }: Props) {
  const frameRef = useRef<HTMLIFrameElement>(null);

  const imgSrc = allowRemote ? "data: cid: https: http:" : "data: cid:";
  const srcDoc = `<!doctype html><html><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src ${imgSrc}; style-src 'unsafe-inline'; font-src data:">
<meta name="referrer" content="no-referrer">
<style>
  html,body{margin:0;background:#fff;overflow:hidden}
  body{padding:24px 28px;color:#17171c;font:14px/1.6 "Inter Variable","Segoe UI",system-ui,sans-serif;overflow-wrap:anywhere}
  img{max-width:100%;height:auto}
  a{color:#2b4fd8}
  table{max-width:100%}
  blockquote{margin:0 0 0 .8em;padding-left:1em;border-left:3px solid #dfe4f7;color:#565d8c}
</style></head><body>${html}</body></html>`;

  const fit = useCallback(() => {
    // Measure the body, not <html>: <html> is always at least as tall as the
    // iframe itself, so it could never shrink to fit short emails.
    const body = frameRef.current?.contentDocument?.body;
    if (frameRef.current && body) {
      frameRef.current.style.height = `${Math.ceil(body.getBoundingClientRect().height)}px`;
    }
  }, []);

  const onLoad = useCallback(() => {
    const doc = frameRef.current?.contentDocument;
    if (!doc) return;
    fit();
    doc.addEventListener("click", (e) => {
      const a = (e.target as Element | null)?.closest?.("a[href]") as HTMLAnchorElement | null;
      if (!a) return;
      e.preventDefault();
      if (/^(https?|mailto):/i.test(a.href)) onLink(a.href);
    });
    new ResizeObserver(fit).observe(doc.body);
  }, [fit, onLink]);

  useEffect(fit, [fit, html]);

  return (
    <iframe
      ref={frameRef}
      title="Email content"
      sandbox="allow-same-origin"
      srcDoc={srcDoc}
      onLoad={onLoad}
      className="message-card block w-full rounded-lg border border-border bg-white"
      style={{ height: 200 }}
    />
  );
}
