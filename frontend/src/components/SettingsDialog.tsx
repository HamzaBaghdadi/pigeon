"use client";

import { useEffect, useRef, useState } from "react";
import { AtSign, Keyboard, Palette, Plus, X } from "lucide-react";
import type { ThemeChoice } from "@/lib/theme";
import { api, errorMessage, type Account, type GoogleSetup as Setup } from "@/lib/api";
import GoogleSetup, { GoogleMark } from "./GoogleSetup";

type Props = {
  theme: ThemeChoice;
  onThemeChange: (t: ThemeChoice) => void;
  onClose: () => void;
  initialSection?: Section;
  accounts: Account[];
  hasSamples: boolean;
  syncErrors: Record<string, string | undefined>;
  onAddAccount: () => void;
  onRemoveAccount: (id: string) => Promise<void>;
  onRemoveSamples: () => Promise<void>;
};

export type Section = "accounts" | "appearance" | "shortcuts";

const themeOptions: { value: ThemeChoice; label: string; hint: string }[] = [
  { value: "system", label: "System", hint: "Match Windows" },
  { value: "light", label: "Light", hint: "Always light" },
  { value: "dark", label: "Dark", hint: "Always dark" },
];

const shortcuts: [string, string][] = [
  ["J / ↓", "Next message"],
  ["K / ↑", "Previous message"],
  ["E", "Archive"],
  ["# / Delete", "Move to Trash"],
  ["S", "Star / unstar"],
  ["U", "Mark read / unread"],
  ["Z", "Undo last action"],
  ["/ or Ctrl K", "Search"],
  ["Ctrl ,", "Settings"],
  ["Esc", "Close / clear"],
];

export default function SettingsDialog(props: Props) {
  const { theme, onThemeChange, onClose } = props;
  const [section, setSection] = useState<Section>(props.initialSection ?? "appearance");
  const dialogRef = useRef<HTMLDivElement>(null);

  // Focus the dialog on open, restore focus on close, close on Escape.
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current;
    (dialog?.querySelector<HTMLElement>("[role='radio'][aria-checked='true']") ?? dialog?.querySelector<HTMLElement>("button"))?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
      }
    }
    window.addEventListener("keydown", onKey, true);
    return () => {
      window.removeEventListener("keydown", onKey, true);
      previous?.focus();
    };
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-6 [animation:fade-in_120ms_ease-out]"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="settings-title"
        className="flex h-[min(480px,100%)] w-[min(720px,100%)] overflow-hidden rounded-xl border border-border bg-surface shadow-2xl [animation:dialog-in_160ms_cubic-bezier(0.16,1,0.3,1)]"
      >
        <nav className="flex w-[188px] shrink-0 flex-col gap-0.5 border-r border-border bg-sidebar p-2" aria-label="Settings sections">
          <h2 id="settings-title" className="px-2.5 pt-2 pb-3 text-[14px] font-semibold">
            Settings
          </h2>
          <NavItem icon={AtSign} label="Accounts" active={section === "accounts"} onClick={() => setSection("accounts")} />
          <NavItem icon={Palette} label="Appearance" active={section === "appearance"} onClick={() => setSection("appearance")} />
          <NavItem icon={Keyboard} label="Shortcuts" active={section === "shortcuts"} onClick={() => setSection("shortcuts")} />
        </nav>

        <div className="relative min-w-0 flex-1 overflow-y-auto px-7 py-6">
          <button
            onClick={onClose}
            aria-label="Close settings"
            title="Close (Esc)"
            className="absolute top-3 right-3 rounded-md p-1.5 text-subtle transition-colors duration-150 hover:bg-hover hover:text-text"
          >
            <X size={16} />
          </button>

          {section === "accounts" ? (
            <AccountsSection {...props} />
          ) : section === "appearance" ? (
            <section aria-labelledby="theme-label">
              <h3 id="theme-label" className="text-[13.5px] font-semibold">
                Theme
              </h3>
              <p className="mt-0.5 text-[12.5px] text-subtle">
                Choose how Pigeon looks. System follows your Windows setting and switches automatically.
              </p>
              <ThemePicker value={theme} onChange={onThemeChange} />
            </section>
          ) : (
            <section aria-labelledby="shortcuts-label">
              <h3 id="shortcuts-label" className="text-[13.5px] font-semibold">
                Keyboard shortcuts
              </h3>
              <p className="mt-0.5 text-[12.5px] text-subtle">Work through your inbox without the mouse.</p>
              <dl className="mt-4 divide-y divide-border rounded-lg border border-border">
                {shortcuts.map(([keys, action]) => (
                  <div key={keys} className="flex items-center justify-between px-3.5 py-2 text-[13px]">
                    <dt className="text-muted">{action}</dt>
                    <dd>
                      <kbd className="rounded border border-border bg-bg px-1.5 py-0.5 font-sans text-[11.5px] text-muted">
                        {keys}
                      </kbd>
                    </dd>
                  </div>
                ))}
              </dl>
            </section>
          )}
        </div>
      </div>
    </div>
  );
}

function NavItem(props: { icon: React.ComponentType<{ size?: number; strokeWidth?: number }>; label: string; active: boolean; onClick: () => void }) {
  const Icon = props.icon;
  return (
    <button
      onClick={props.onClick}
      aria-current={props.active ? "page" : undefined}
      className={`flex h-[30px] items-center gap-2.5 rounded-md px-2.5 text-[13px] transition-colors duration-150 ${
        props.active ? "bg-active font-medium text-text" : "text-muted hover:bg-hover hover:text-text"
      }`}
    >
      <Icon size={15} strokeWidth={1.75} />
      {props.label}
    </button>
  );
}

function ThemePicker({ value, onChange }: { value: ThemeChoice; onChange: (t: ThemeChoice) => void }) {
  // Radio group: arrow keys move the selection, as with native radios.
  function onKeyDown(e: React.KeyboardEvent, i: number) {
    const dir = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
    if (!dir) return;
    e.preventDefault();
    const next = themeOptions[(i + dir + themeOptions.length) % themeOptions.length];
    onChange(next.value);
    (e.currentTarget.parentElement?.children[themeOptions.indexOf(next)] as HTMLElement | undefined)?.focus();
  }

  return (
    <div role="radiogroup" aria-labelledby="theme-label" className="mt-4 grid grid-cols-3 gap-3">
      {themeOptions.map((o, i) => {
        const checked = value === o.value;
        return (
          <button
            key={o.value}
            role="radio"
            aria-checked={checked}
            tabIndex={checked ? 0 : -1}
            onClick={() => onChange(o.value)}
            onKeyDown={(e) => onKeyDown(e, i)}
            className={`group rounded-lg border p-1.5 text-left transition-[border-color,box-shadow] duration-150 ${
              checked
                ? "border-primary shadow-[0_0_0_3px_var(--primary-soft)]"
                : "border-border hover:border-border-strong"
            }`}
          >
            <ThemePreview kind={o.value} />
            <div className="flex items-start gap-2 px-1.5 pt-2 pb-1">
              <span
                className={`mt-[3px] flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-full border ${
                  checked ? "border-primary" : "border-border-strong"
                }`}
                aria-hidden
              >
                {checked && <span className="h-1.5 w-1.5 rounded-full bg-primary" />}
              </span>
              <span className="min-w-0 leading-tight">
                <span className="block text-[13px] font-medium">{o.label}</span>
                <span className="block text-[11.5px] text-subtle">{o.hint}</span>
              </span>
            </div>
          </button>
        );
      })}
    </div>
  );
}

/** A tiny drawing of the three-pane layout in each theme. */
function ThemePreview({ kind }: { kind: ThemeChoice }) {
  if (kind === "system") {
    return (
      <div className="relative h-[78px] overflow-hidden rounded-md">
        <Mini palette={light} />
        <div className="absolute inset-0 [clip-path:polygon(100%_0,100%_100%,0_100%)]">
          <Mini palette={dark} />
        </div>
      </div>
    );
  }
  return (
    <div className="h-[78px] overflow-hidden rounded-md">
      <Mini palette={kind === "light" ? light : dark} />
    </div>
  );
}

// Fixed preview colors (independent of the active theme, by design).
const light = { side: "#f1f1f3", list: "#ffffff", read: "#f7f7f8", line: "#e6e6ea", text: "#c9c9d0", accent: "#2b4fd8" };
const dark = { side: "#0f1013", list: "#18191d", read: "#131417", line: "#26272d", text: "#3a3b43", accent: "#6b8aff" };

function Mini({ palette: p }: { palette: typeof light }) {
  return (
    <div className="flex h-full w-full" style={{ background: p.read }} aria-hidden>
      <div className="flex w-[26%] flex-col gap-1 p-1.5" style={{ background: p.side }}>
        <div className="h-2 w-full rounded-sm" style={{ background: p.accent }} />
        <div className="h-1 w-3/4 rounded-sm" style={{ background: p.text }} />
        <div className="h-1 w-2/3 rounded-sm" style={{ background: p.text }} />
        <div className="h-1 w-3/4 rounded-sm" style={{ background: p.text }} />
      </div>
      <div className="flex w-[34%] flex-col" style={{ background: p.list, borderLeft: `1px solid ${p.line}`, borderRight: `1px solid ${p.line}` }}>
        {[0, 1, 2].map((i) => (
          <div key={i} className="flex flex-col gap-0.5 px-1.5 py-1.5" style={{ borderBottom: `1px solid ${p.line}` }}>
            <div className="h-1 w-2/3 rounded-sm" style={{ background: i === 0 ? p.accent : p.text }} />
            <div className="h-1 w-full rounded-sm" style={{ background: p.text }} />
          </div>
        ))}
      </div>
      <div className="flex flex-1 flex-col gap-1 p-2">
        <div className="h-1.5 w-3/4 rounded-sm" style={{ background: p.text }} />
        <div className="mt-1 h-8 w-full rounded-sm bg-white" style={{ border: `1px solid ${p.line}` }} />
      </div>
    </div>
  );
}

function AccountsSection(props: Props) {
  const real = props.accounts.filter((a) => !a.demo);
  return (
    <section aria-labelledby="accounts-label">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h3 id="accounts-label" className="text-[13.5px] font-semibold">
            Accounts
          </h3>
          <p className="mt-0.5 text-[12.5px] text-subtle">Removing an account deletes Pigeon&apos;s local copy only. Nothing is deleted on the server.</p>
        </div>
        <button
          onClick={props.onAddAccount}
          className="flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-border px-3 text-[13px] font-medium transition-colors duration-150 hover:bg-hover"
        >
          <Plus size={14} aria-hidden />
          Add account
        </button>
      </div>

      {real.length === 0 ? (
        <p className="mt-4 rounded-lg border border-dashed border-border px-4 py-6 text-center text-[12.5px] text-subtle">No accounts yet.</p>
      ) : (
        <ul className="mt-4 divide-y divide-border rounded-lg border border-border">
          {real.map((a) => (
            <AccountItem key={a.id} account={a} error={props.syncErrors[a.id]} onRemove={() => props.onRemoveAccount(a.id)} />
          ))}
        </ul>
      )}

      <GoogleSection />

      {props.hasSamples && (
        <div className="mt-5 flex items-center justify-between gap-4 rounded-lg bg-bg px-3.5 py-3 text-[12.5px]">
          <span className="text-muted">Sample mail is loaded so you can explore Pigeon.</span>
          <button onClick={props.onRemoveSamples} className="shrink-0 rounded px-2 py-1 font-medium text-primary transition-colors hover:bg-primary-soft">
            Remove sample mail
          </button>
        </div>
      )}
    </section>
  );
}

function AccountItem({ account, error, onRemove }: { account: Account; error?: string; onRemove: () => Promise<void> }) {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [reconnectError, setReconnectError] = useState<string | null>(null);
  const isGoogle = account.auth === "google";
  const needsReconnect = isGoogle && !!error && /sign.?in|expired|revoked|invalid_grant/i.test(error);

  async function reconnect() {
    setReconnecting(true);
    setReconnectError(null);
    try {
      await api.reconnectGoogle(account.id);
    } catch (err) {
      const msg = errorMessage(err);
      if (!/canceled/i.test(msg)) setReconnectError(msg);
    } finally {
      setReconnecting(false);
    }
  }
  return (
    <li className="px-3.5 py-3">
      <div className="flex items-center gap-3">
        <span className="h-2.5 w-2.5 shrink-0 rounded-[3px]" style={{ background: account.color }} aria-hidden />
        <div className="min-w-0 flex-1">
          <div className="truncate text-[13px] font-medium">{account.name}</div>
          <div className="flex items-center gap-1.5 truncate text-[12px] text-subtle">
            {isGoogle && <GoogleMark size={11} />}
            {account.email}
          </div>
        </div>
        {needsReconnect && !confirming && (
          <button
            onClick={reconnect}
            disabled={reconnecting}
            className="shrink-0 rounded bg-primary px-2.5 py-1 text-[12.5px] font-medium text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            {reconnecting ? "Waiting for Google…" : "Reconnect"}
          </button>
        )}
        {confirming ? (
          <div className="flex shrink-0 items-center gap-1">
            <button
              onClick={() => setConfirming(false)}
              className="rounded px-2 py-1 text-[12.5px] text-muted transition-colors hover:bg-hover hover:text-text"
            >
              Cancel
            </button>
            <button
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                try {
                  await onRemove();
                } finally {
                  setBusy(false);
                }
              }}
              className="rounded bg-danger px-2.5 py-1 text-[12.5px] font-medium text-white transition-opacity hover:opacity-90"
            >
              Remove
            </button>
          </div>
        ) : (
          <button
            onClick={() => setConfirming(true)}
            className="shrink-0 rounded px-2 py-1 text-[12.5px] text-muted transition-colors hover:bg-hover hover:text-danger"
          >
            Remove
          </button>
        )}
      </div>
      {error && <p className="mt-2 text-[12px] text-danger">Sync problem: {error}</p>}
      {reconnectError && <p className="mt-2 text-[12px] text-danger">{reconnectError}</p>}
    </li>
  );
}

function GoogleSection() {
  const [setup, setSetup] = useState<Setup | null>(null);
  const [editing, setEditing] = useState(false);
  useEffect(() => {
    api.getGoogleSetup().then(setSetup);
  }, [editing]);
  if (!setup) return null;
  return (
    <div className="mt-6">
      <h4 className="text-[13px] font-semibold">Sign in with Google</h4>
      {editing ? (
        <div className="mt-2">
          <GoogleSetup onSaved={() => setEditing(false)} onCancel={() => setEditing(false)} saveLabel="Save" />
        </div>
      ) : (
        <div className="mt-2 flex items-center justify-between gap-4 rounded-lg border border-border px-3.5 py-3 text-[12.5px]">
          <span className="min-w-0 text-muted">
            {setup.configured ? (
              <>
                {setup.builtIn ? "Using Pigeon's built-in Google client." : "Using your Google client "}
                {!setup.builtIn && <span className="font-mono text-[11.5px] text-subtle">{setup.clientId.split("-")[0]}…</span>}
              </>
            ) : (
              "Not set up yet. You'll be guided through it the first time you sign in with Google."
            )}
          </span>
          <button
            onClick={() => setEditing(true)}
            className="shrink-0 rounded px-2 py-1 font-medium text-primary transition-colors hover:bg-primary-soft"
          >
            {setup.configured && !setup.builtIn ? "Change" : "Set up"}
          </button>
        </div>
      )}
    </div>
  );
}
