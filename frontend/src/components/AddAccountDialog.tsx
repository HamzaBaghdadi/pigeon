"use client";

import { useEffect, useRef, useState } from "react";
import {
  AlertTriangle,
  ChevronRight,
  Eye,
  EyeOff,
  KeyRound,
  Loader2,
  X,
} from "lucide-react";
import { api, errorMessage, type ProviderSettings } from "@/lib/api";
import GoogleSetup, { GoogleMark } from "./GoogleSetup";
import { MicrosoftMark } from "./MicrosoftMark";
import { Tip } from "./Tip";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

type Props = {
  onClose: () => void;
  onAdded: () => void;
};

type Server = {
  username: string;
  imapHost: string;
  imapPort: number;
  imapSecurity: string;
  smtpHost: string;
  smtpPort: number;
  smtpSecurity: string;
};

const emptyServer: Server = {
  username: "",
  imapHost: "",
  imapPort: 993,
  imapSecurity: "tls",
  smtpHost: "",
  smtpPort: 465,
  smtpSecurity: "tls",
};

export default function AddAccountDialog({ onClose, onAdded }: Props) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [advanced, setAdvanced] = useState(false);
  const [server, setServer] = useState<Server>(emptyServer);
  const [serverEdited, setServerEdited] = useState(false);
  const [detected, setDetected] = useState<ProviderSettings | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [mode, setMode] = useState<"form" | "google-setup" | "waiting">("form");
  const [waitingFor, setWaitingFor] = useState<"Google" | "Microsoft">(
    "Google",
  );
  const [microsoft, setMicrosoft] = useState(false);
  const emailRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api
      .getMicrosoftSetup()
      .then((s) => setMicrosoft(s.configured))
      .catch(() => setMicrosoft(false));
  }, []);

  const close = () => {
    if (mode === "waiting") api.cancelSignIn();
    onClose();
  };

  async function runOAuth(provider: "Google" | "Microsoft") {
    setError(null);
    setWaitingFor(provider);
    setMode("waiting");
    const hint = email.includes("@") ? email.trim() : "";
    try {
      if (provider === "Google") await api.signInWithGoogle(hint, name);
      else await api.signInWithMicrosoft(hint, name);
      onAdded();
    } catch (err) {
      setMode("form");
      const msg = errorMessage(err);
      if (!/canceled/i.test(msg)) setError(msg);
    }
  }

  const runGoogle = () => runOAuth("Google");

  async function startGoogle() {
    setError(null);
    try {
      const setup = await api.getGoogleSetup();
      if (setup.configured) await runGoogle();
      else setMode("google-setup");
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  useEffect(() => {
    emailRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !busy) {
        e.stopPropagation();
        if (mode === "waiting") api.cancelSignIn();
        onClose();
      }
    }
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onClose, busy, mode]);

  // Suggest server settings as the address is typed (debounced).
  useEffect(() => {
    if (!email.includes("@")) return;
    const t = setTimeout(() => {
      api.detectSettings(email).then((s) => {
        setDetected(s);
        if (!serverEdited) {
          setServer((cur) => ({
            ...cur,
            imapHost: s.imapHost,
            imapPort: s.imapPort,
            imapSecurity: s.imapSecurity,
            smtpHost: s.smtpHost,
            smtpPort: s.smtpPort,
            smtpSecurity: s.smtpSecurity,
          }));
        }
      });
    }, 250);
    return () => clearTimeout(t);
  }, [email, serverEdited]);

  const editServer = (patch: Partial<Server>) => {
    setServerEdited(true);
    setServer((cur) => ({ ...cur, ...patch }));
  };

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      // The suggestion is debounced, so it may not have arrived yet (e.g. the
      // address was pasted and Enter pressed right away). Resolve it now.
      let srv = server;
      if (!serverEdited) {
        const s = await api.detectSettings(email);
        srv = {
          ...server,
          imapHost: s.imapHost,
          imapPort: s.imapPort,
          imapSecurity: s.imapSecurity,
          smtpHost: s.smtpHost,
          smtpPort: s.smtpPort,
          smtpSecurity: s.smtpSecurity,
        };
        setServer(srv);
        setDetected(s);
      }
      await api.addAccount({ name, email, password, ...srv });
      onAdded();
    } catch (err) {
      setError(errorMessage(err));
      setAdvanced(true); // most sign-in problems are fixed in server settings
    } finally {
      setBusy(false);
    }
  }

  const blocked = Boolean(detected?.warning && !detected.imapHost);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-6 [animation:fade-in_120ms_ease-out]"
      onMouseDown={(e) => e.target === e.currentTarget && !busy && close()}
    >
      <form
        role="dialog"
        aria-modal="true"
        aria-labelledby="add-account-title"
        onSubmit={submit}
        className="relative flex max-h-full w-[min(460px,100%)] flex-col overflow-hidden rounded-xl border border-border bg-surface shadow-2xl [animation:dialog-in_160ms_cubic-bezier(0.16,1,0.3,1)]"
      >
        <header className="flex h-12 shrink-0 items-center justify-between border-b border-border bg-sidebar pr-2 pl-5">
          <h2 id="add-account-title" className="display-heading font-display text-[14px] font-semibold">
            Add an account
          </h2>
          <Tip label="Close" shortcut="Esc">
            <button
              type="button"
              onClick={close}
              disabled={busy}
              aria-label="Close"
              className="rounded-md p-1.5 text-subtle transition-colors duration-150 hover:bg-hover hover:text-text disabled:opacity-50"
            >
              <X size={16} />
            </button>
          </Tip>
        </header>
        <div className="overflow-y-auto px-6 pt-4 pb-5">
          <p className="text-[12.5px] text-subtle">
            Works with Gmail, Outlook, iCloud, Yahoo, Fastmail and any IMAP
            provider.
          </p>

          {mode === "waiting" ? (
            <div
              className="flex flex-col items-center py-10 text-center"
              role="status"
            >
              {waitingFor === "Google" ? (
                <GoogleMark size={36} />
              ) : (
                <MicrosoftMark size={34} />
              )}
              <p className="mt-4 text-[14px] font-medium">
                Continue in your browser
              </p>
              <p className="mt-1.5 max-w-[320px] text-[12.5px] leading-snug text-subtle">
                Sign in with {waitingFor} in the browser tab that just opened.
                Pigeon connects as soon as you approve.
              </p>
              <Loader2
                size={18}
                className="mt-5 animate-spin text-subtle"
                aria-hidden
              />
              <button
                type="button"
                onClick={() => api.cancelSignIn()}
                className="mt-5 h-8 rounded-md px-3 text-[13px] text-muted transition-colors hover:bg-hover hover:text-text"
              >
                Cancel
              </button>
            </div>
          ) : mode === "google-setup" ? (
            <div className="mt-5">
              <GoogleSetup
                onSaved={runGoogle}
                onCancel={() => setMode("form")}
                saveLabel="Save and sign in"
              />
            </div>
          ) : (
            <div className="mt-5 space-y-3.5">
              <button
                type="button"
                onClick={startGoogle}
                className="flex h-9 w-full items-center justify-center gap-2.5 rounded-md border border-border bg-surface text-[13px] font-medium transition-colors duration-150 hover:bg-hover"
              >
                <GoogleMark />
                Sign in with Google
              </button>
              {microsoft && (
                <button
                  type="button"
                  onClick={() => runOAuth("Microsoft")}
                  className="flex h-9 w-full items-center justify-center gap-2.5 rounded-md border border-border bg-surface text-[13px] font-medium transition-colors duration-150 hover:bg-hover"
                >
                  <MicrosoftMark />
                  Sign in with Microsoft
                </button>
              )}
              <div
                className="flex items-center gap-3 text-[11.5px] text-subtle"
                aria-hidden
              >
                <span className="h-px flex-1 bg-border" />
                or use a password
                <span className="h-px flex-1 bg-border" />
              </div>

              <Field label="Email address" htmlFor="acct-email">
                <Input
                  id="acct-email"
                  ref={emailRef}
                  type="email"
                  autoComplete="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="you@example.com"
                  className={inputClass}
                />
                {detected?.known && detected.label && (
                  <p className="mt-1.5 text-[12px] text-subtle">
                    {detected.label} detected. Server settings are filled in for
                    you.
                  </p>
                )}
              </Field>

              {detected?.warning && (
                <div
                  role="note"
                  className="flex gap-2.5 rounded-md border border-border bg-bg px-3 py-2.5 text-[12.5px] text-muted"
                >
                  <AlertTriangle
                    size={15}
                    className="mt-px shrink-0 text-star"
                    aria-hidden
                  />
                  <span>
                    {detected.warning}
                    {detected.provider === "outlook" &&
                      microsoft &&
                      " Use Sign in with Microsoft above instead."}
                  </span>
                </div>
              )}

              <Field label="Password" htmlFor="acct-password">
                <div className="relative">
                  <Input
                    id="acct-password"
                    type={showPassword ? "text" : "password"}
                    autoComplete="current-password"
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className={`${inputClass} pr-10`}
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword((v) => !v)}
                    aria-label={
                      showPassword ? "Hide password" : "Show password"
                    }
                    className="absolute top-1/2 right-1.5 -translate-y-1/2 rounded p-1.5 text-subtle transition-colors hover:bg-hover hover:text-text"
                  >
                    {showPassword ? <EyeOff size={15} /> : <Eye size={15} />}
                  </button>
                </div>
                {detected?.provider === "gmail" ? (
                  <p className="mt-1.5 text-[12px] leading-snug text-subtle">
                    Gmail only accepts an app password here, not your normal
                    password. Easier:{" "}
                    <button
                      type="button"
                      onClick={startGoogle}
                      className="font-medium text-primary hover:underline"
                    >
                      Sign in with Google
                    </button>{" "}
                    instead.
                  </p>
                ) : (
                  detected?.passwordHint && (
                    <p className="mt-1.5 text-[12px] leading-snug text-subtle">
                      {detected.passwordHint}{" "}
                      {detected.helpUrl && (
                        <button
                          type="button"
                          onClick={() => api.openExternal(detected.helpUrl)}
                          className="font-medium text-primary hover:underline"
                        >
                          Create one
                        </button>
                      )}
                    </p>
                  )
                )}
              </Field>

              <Field label="Account name" htmlFor="acct-name" optional>
                <Input
                  id="acct-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Personal or Work"
                  className={inputClass}
                />
              </Field>

              <div>
                <button
                  type="button"
                  onClick={() => setAdvanced((v) => !v)}
                  aria-expanded={advanced}
                  className="-ml-1 flex items-center gap-1 rounded px-1 py-0.5 text-[12.5px] font-medium text-muted transition-colors hover:text-text"
                >
                  <ChevronRight
                    size={14}
                    className={`transition-transform duration-150 ${advanced ? "rotate-90" : ""}`}
                    aria-hidden
                  />
                  Server settings
                </button>
                {advanced && (
                  <div className="mt-3 space-y-3 rounded-lg border border-border p-3.5">
                    <Field
                      label="Username"
                      htmlFor="acct-user"
                      optional
                      hint="Usually your email address."
                    >
                      <Input
                        id="acct-user"
                        value={server.username}
                        onChange={(e) =>
                          editServer({ username: e.target.value })
                        }
                        placeholder={email || "Same as email"}
                        className={inputClass}
                      />
                    </Field>
                    <ServerRow
                      label="Incoming mail (IMAP)"
                      idPrefix="imap"
                      host={server.imapHost}
                      port={server.imapPort}
                      security={server.imapSecurity}
                      onChange={(h, p, s) =>
                        editServer({
                          imapHost: h,
                          imapPort: p,
                          imapSecurity: s,
                        })
                      }
                    />
                    <ServerRow
                      label="Outgoing mail (SMTP)"
                      idPrefix="smtp"
                      host={server.smtpHost}
                      port={server.smtpPort}
                      security={server.smtpSecurity}
                      onChange={(h, p, s) =>
                        editServer({
                          smtpHost: h,
                          smtpPort: p,
                          smtpSecurity: s,
                        })
                      }
                    />
                  </div>
                )}
              </div>
            </div>
          )}

          {error && mode !== "waiting" && (
            <div
              role="alert"
              className="mt-3.5 rounded-md border border-danger/30 bg-danger/8 px-3 py-2.5 text-[12.5px] text-danger"
            >
              {error}
            </div>
          )}
        </div>

        {mode === "form" && (
          <div className="flex items-center gap-3 border-t border-border bg-bg px-6 py-3.5">
            <p className="flex flex-1 items-center gap-1.5 text-[11.5px] leading-snug text-subtle">
              <KeyRound size={13} className="shrink-0" aria-hidden />
              Your password is kept in Windows Credential Manager, never in
              Pigeon&apos;s files.
            </p>
            <button
              type="submit"
              disabled={busy || blocked}
              className="flex h-8 shrink-0 items-center gap-2 rounded-md bg-primary px-4 text-[13px] font-medium text-primary-foreground transition-colors duration-150 hover:bg-primary-hover disabled:opacity-60"
            >
              {busy && (
                <Loader2 size={14} className="animate-spin" aria-hidden />
              )}
              {busy ? "Signing in…" : "Sign in"}
            </button>
          </div>
        )}
      </form>
    </div>
  );
}

// shadcn Input, sized to Pigeon's 13px UI.
const baseInput = "h-9 text-[13px] md:text-[13px]";
const inputClass = `${baseInput} w-full`;

function Field(props: {
  label: string;
  htmlFor: string;
  optional?: boolean;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label
        htmlFor={props.htmlFor}
        className="mb-1.5 flex items-baseline gap-1.5 text-[12.5px] font-medium"
      >
        {props.label}
        {props.optional && (
          <span className="font-normal text-subtle">Optional</span>
        )}
      </label>
      {props.children}
      {props.hint && (
        <p className="mt-1 text-[11.5px] text-subtle">{props.hint}</p>
      )}
    </div>
  );
}

function ServerRow(props: {
  label: string;
  idPrefix: string;
  host: string;
  port: number;
  security: string;
  onChange: (host: string, port: number, security: string) => void;
}) {
  const { host, port, security, onChange } = props;
  return (
    <fieldset>
      <legend className="mb-1.5 text-[12.5px] font-medium">
        {props.label}
      </legend>
      <div className="flex gap-1.5">
        <label className="sr-only" htmlFor={`${props.idPrefix}-host`}>
          Server
        </label>
        <Input
          id={`${props.idPrefix}-host`}
          value={host}
          onChange={(e) => onChange(e.target.value, port, security)}
          placeholder="imap.example.com"
          className={`${baseInput} min-w-0 flex-1`}
        />
        <label className="sr-only" htmlFor={`${props.idPrefix}-port`}>
          Port
        </label>
        <Input
          id={`${props.idPrefix}-port`}
          inputMode="numeric"
          value={port || ""}
          onChange={(e) =>
            onChange(
              host,
              Number(e.target.value.replace(/\D/g, "")) || 0,
              security,
            )
          }
          className={`${baseInput} w-[64px] shrink-0`}
        />
        <label className="sr-only" htmlFor={`${props.idPrefix}-sec`}>
          Encryption
        </label>
        <Select value={security} onValueChange={(v) => onChange(host, port, v)}>
          <SelectTrigger
            id={`${props.idPrefix}-sec`}
            className="h-9 w-[112px] shrink-0 text-[13px]"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="tls">SSL/TLS</SelectItem>
            <SelectItem value="starttls">STARTTLS</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </fieldset>
  );
}
