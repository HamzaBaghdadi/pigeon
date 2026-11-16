"use client";

import { useState } from "react";
import { ExternalLink, Loader2 } from "lucide-react";
import { api, errorMessage } from "@/lib/api";

type Props = {
  onSaved: () => void;
  onCancel?: () => void;
  saveLabel?: string;
};

const steps: { text: string; link?: [string, string] }[] = [
  { text: "Create a project in Google Cloud (any name, e.g. Pigeon).", link: ["Create project", "https://console.cloud.google.com/projectcreate"] },
  { text: "Enable the Gmail API for that project.", link: ["Gmail API", "https://console.cloud.google.com/apis/library/gmail.googleapis.com"] },
  {
    text: "Set up Google Auth Platform: choose External, add your own email as a test user, then under Audience click Publish app so your sign-in doesn't expire every 7 days.",
    link: ["Auth Platform", "https://console.cloud.google.com/auth/overview"],
  },
  { text: "Under Clients, create a client of type Desktop app and copy its Client ID and Client secret here.", link: ["Clients", "https://console.cloud.google.com/auth/clients"] },
];

/**
 * One-time setup of the user's own Google OAuth client. Official builds can
 * ship with a client built in; until then each person brings their own, which
 * also keeps credentials out of the open-source repository.
 */
export default function GoogleSetup({ onSaved, onCancel, saveLabel = "Save and continue" }: Props) {
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      await api.saveGoogleSetup(clientId, clientSecret);
      onSaved();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="rounded-lg border border-border p-4">
      <h3 className="text-[13.5px] font-semibold">One-time setup for Sign in with Google</h3>
      <p className="mt-1 text-[12.5px] leading-snug text-subtle">
        Google requires each app that reads Gmail to be registered. This takes about 10 minutes and is free. Your password
        is never shared with Pigeon.
      </p>
      <ol className="mt-3 space-y-2 text-[12.5px] leading-snug text-muted">
        {steps.map((s, i) => (
          <li key={i} className="flex gap-2.5">
            <span className="mt-px flex h-[18px] w-[18px] shrink-0 items-center justify-center rounded-full bg-bg text-[11px] font-semibold text-subtle">
              {i + 1}
            </span>
            <span>
              {s.text}{" "}
              {s.link && (
                <button
                  type="button"
                  onClick={() => api.openExternal(s.link![1])}
                  className="inline-flex items-center gap-0.5 font-medium text-primary hover:underline"
                >
                  {s.link[0]}
                  <ExternalLink size={11} aria-hidden />
                </button>
              )}
            </span>
          </li>
        ))}
      </ol>

      <div className="mt-4 space-y-2.5">
        <div>
          <label htmlFor="g-client-id" className="mb-1 block text-[12.5px] font-medium">
            Client ID
          </label>
          <input
            id="g-client-id"
            value={clientId}
            onChange={(e) => setClientId(e.target.value)}
            placeholder="1234-abc.apps.googleusercontent.com"
            spellCheck={false}
            className={input}
          />
        </div>
        <div>
          <label htmlFor="g-client-secret" className="mb-1 block text-[12.5px] font-medium">
            Client secret
          </label>
          <input
            id="g-client-secret"
            type="password"
            value={clientSecret}
            onChange={(e) => setClientSecret(e.target.value)}
            placeholder="GOCSPX-…"
            spellCheck={false}
            className={input}
          />
          <p className="mt-1 text-[11.5px] text-subtle">Saved in Windows Credential Manager.</p>
        </div>
      </div>

      {error && (
        <p role="alert" className="mt-3 text-[12.5px] text-danger">
          {error}
        </p>
      )}

      <div className="mt-4 flex justify-end gap-2">
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className="h-8 rounded-md px-3 text-[13px] text-muted transition-colors hover:bg-hover hover:text-text"
          >
            Back
          </button>
        )}
        <button
          type="button"
          onClick={save}
          disabled={busy || !clientId || !clientSecret}
          className="flex h-8 items-center gap-2 rounded-md bg-primary px-4 text-[13px] font-medium text-white transition-colors hover:bg-primary-hover disabled:opacity-50"
        >
          {busy && <Loader2 size={14} className="animate-spin" aria-hidden />}
          {saveLabel}
        </button>
      </div>
    </div>
  );
}

const input =
  "h-9 w-full rounded-md border border-border bg-surface px-3 font-mono text-[12.5px] text-text outline-none transition-[border-color,box-shadow] duration-150 placeholder:font-sans placeholder:text-subtle hover:border-border-strong focus:border-primary focus:shadow-[0_0_0_3px_var(--primary-soft)]";

/** Google's "G" mark, drawn inline (no external request). */
export function GoogleMark({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 48 48" aria-hidden>
      <path fill="#FFC107" d="M43.6 20.5H42V20H24v8h11.3C33.7 32.7 29.2 36 24 36c-6.6 0-12-5.4-12-12s5.4-12 12-12c3.1 0 5.9 1.2 8 3.1l5.7-5.7C34 6.1 29.3 4 24 4 12.9 4 4 12.9 4 24s8.9 20 20 20 20-8.9 20-20c0-1.2-.1-2.3-.4-3.5z" />
      <path fill="#FF3D00" d="M6.3 14.7l6.6 4.8C14.7 15.1 19 12 24 12c3.1 0 5.9 1.2 8 3.1l5.7-5.7C34 6.1 29.3 4 24 4 16.3 4 9.7 8.3 6.3 14.7z" />
      <path fill="#4CAF50" d="M24 44c5.2 0 9.9-2 13.4-5.2l-6.2-5.2C29.2 35.1 26.7 36 24 36c-5.2 0-9.6-3.3-11.3-7.9l-6.5 5C9.5 39.6 16.2 44 24 44z" />
      <path fill="#1976D2" d="M43.6 20.5H42V20H24v8h11.3c-.8 2.2-2.2 4.2-4.1 5.6l6.2 5.2C37 39.2 44 34 44 24c0-1.2-.1-2.3-.4-3.5z" />
    </svg>
  );
}
