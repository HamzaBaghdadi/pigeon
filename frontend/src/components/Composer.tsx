"use client";

import { useEffect, useRef, useState } from "react";
import { Paperclip, Trash2, X } from "lucide-react";
import { api, errorMessage, type Account, type Address, type AttachmentInfo, type Draft } from "@/lib/api";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tip } from "./Tip";

export type ComposerState = {
  draft: Draft;
  title: string;
};

/** A draft with every list present (Go may send null for empty lists). */
type EditDraft = Omit<Draft, "to" | "cc" | "bcc" | "attachments"> & {
  to: Address[];
  cc: Address[];
  bcc: Address[];
  attachments: AttachmentInfo[];
};

function normalize(d: Draft): EditDraft {
  return { ...d, to: d.to ?? [], cc: d.cc ?? [], bcc: d.bcc ?? [], attachments: d.attachments ?? [] };
}

type Props = {
  state: ComposerState;
  accounts: Account[];
  onSend: (d: Draft) => void;
  onClose: () => void;
};

export default function Composer({ state, accounts, onSend, onClose }: Props) {
  const [start] = useState(() => normalize(state.draft));
  const [initial] = useState(() => JSON.stringify(start));
  const [draft, setDraft] = useState<EditDraft>(start);
  const [showCc, setShowCc] = useState(start.cc.length > 0);
  const [showBcc, setShowBcc] = useState(start.bcc.length > 0);
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [noSubjectOk, setNoSubjectOk] = useState(false);
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  const toRef = useRef<HTMLInputElement>(null);
  const dirty = JSON.stringify(draft) !== initial;

  const set = (patch: Partial<EditDraft>) => setDraft((d) => ({ ...d, ...patch }));

  // Replies start typing above the quote; new messages start at "To".
  useEffect(() => {
    if (start.to.length > 0) {
      bodyRef.current?.focus();
      bodyRef.current?.setSelectionRange(0, 0);
    } else {
      toRef.current?.focus();
    }
  }, [start.to.length]);

  function close() {
    if (dirty && !confirmDiscard) {
      setConfirmDiscard(true);
      return;
    }
    onClose();
  }

  function send() {
    setError(null);
    if (draft.to.length + draft.cc.length + draft.bcc.length === 0) {
      setError("Add at least one recipient.");
      toRef.current?.focus();
      return;
    }
    if (!draft.subject.trim() && !noSubjectOk) {
      setNoSubjectOk(true);
      setError("There's no subject. Press Send again to send it anyway.");
      return;
    }
    onSend(draft);
  }

  async function attach() {
    try {
      const picked = await api.pickAttachments();
      if (picked.length) set({ attachments: [...draft.attachments, ...picked] });
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  const totalSize = draft.attachments.reduce((n, a) => n + a.size, 0);
  // Real accounts, plus the draft's own account (e.g. replying to sample mail).
  const sendable = accounts.filter((a) => !a.demo || a.id === draft.accountId);

  return (
    <div
      role="dialog"
      aria-label={state.title}
      onKeyDown={(e) => {
        if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
          e.preventDefault();
          send();
        } else if (e.key === "Escape") {
          e.preventDefault();
          e.stopPropagation();
          close();
        }
      }}
      className="absolute right-5 bottom-5 z-40 flex h-[min(620px,calc(100vh-40px))] w-[min(560px,calc(100vw-40px))] flex-col overflow-hidden rounded-xl border border-border bg-surface shadow-2xl [animation:dialog-in_160ms_cubic-bezier(0.16,1,0.3,1)]"
    >
      <div className="flex h-10 shrink-0 items-center border-b border-border bg-sidebar pr-1.5 pl-4">
        <span className="flex-1 truncate text-[13px] font-medium">{draft.subject.trim() || state.title}</span>
        <Tip label="Close" shortcut="Esc">
          <button onClick={close} aria-label="Close" className="rounded-md p-1.5 text-subtle transition-colors hover:bg-hover hover:text-text">
            <X size={15} />
          </button>
        </Tip>
      </div>

      <div className="shrink-0 divide-y divide-border border-b border-border px-4 text-[13px]">
        {sendable.length > 1 && (
          <Row label="From">
            <Select value={draft.accountId} onValueChange={(v) => set({ accountId: v })}>
              <SelectTrigger
                aria-label="From"
                className="-ml-2 h-8 w-auto max-w-full border-transparent bg-transparent px-2 text-[13px] shadow-none hover:bg-hover focus-visible:ring-0 dark:bg-transparent"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {sendable.map((a) => (
                  <SelectItem key={a.id} value={a.id} className="text-[13px]">
                    <span className="h-2 w-2 shrink-0 rounded-[3px]" style={{ background: a.color }} aria-hidden />
                    <span className="truncate">{a.name}</span>
                    <span className="truncate text-muted-foreground">{a.email}</span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Row>
        )}
        <Row label="To">
          <RecipientInput inputRef={toRef} value={draft.to} onChange={(to) => set({ to })} label="To" />
          {!showCc && (
            <button tabIndex={-1} onClick={() => setShowCc(true)} className="shrink-0 rounded px-1.5 py-0.5 text-[12px] text-subtle hover:text-text">
              Cc
            </button>
          )}
          {!showBcc && (
            <button tabIndex={-1} onClick={() => setShowBcc(true)} className="shrink-0 rounded px-1.5 py-0.5 text-[12px] text-subtle hover:text-text">
              Bcc
            </button>
          )}
        </Row>
        {showCc && (
          <Row label="Cc">
            <RecipientInput value={draft.cc} onChange={(cc) => set({ cc })} label="Cc" autoFocus={start.cc.length === 0} />
          </Row>
        )}
        {showBcc && (
          <Row label="Bcc">
            <RecipientInput value={draft.bcc} onChange={(bcc) => set({ bcc })} label="Bcc" autoFocus={start.bcc.length === 0} />
          </Row>
        )}
        <Row label="Subject">
          <Input
            value={draft.subject}
            onChange={(e) => set({ subject: e.target.value })}
            aria-label="Subject"
            className="h-9 border-0 bg-transparent px-0 shadow-none focus-visible:ring-0 dark:bg-transparent rounded-none text-[13px] md:text-[13px]"
          />
        </Row>
      </div>

      <Textarea
        ref={bodyRef}
        value={draft.body}
        onChange={(e) => set({ body: e.target.value })}
        aria-label="Message"
        spellCheck
        className="selectable field-sizing-fixed min-h-0 flex-1 resize-none border-0 bg-transparent shadow-none focus-visible:ring-0 dark:bg-transparent rounded-none px-4 py-3 text-[14px] leading-relaxed md:text-[14px]"
      />

      {draft.attachments.length > 0 && (
        <div className="flex shrink-0 flex-wrap gap-1.5 border-t border-border px-4 py-2.5">
          {draft.attachments.map((a) => (
            <AttachmentChip key={a.id} a={a} onRemove={() => set({ attachments: draft.attachments.filter((x) => x.id !== a.id) })} />
          ))}
          {totalSize > 25 * 1024 * 1024 && <p className="w-full text-[12px] text-danger">Attachments are over 25 MB; most providers will reject this.</p>}
        </div>
      )}

      {error && (
        <p role="alert" className="shrink-0 px-4 pb-1 text-[12.5px] text-danger">
          {error}
        </p>
      )}

      <div className="flex h-14 shrink-0 items-center gap-1 border-t border-border px-3">
        {confirmDiscard ? (
          <>
            <span className="flex-1 pl-1 text-[13px] text-muted">Discard this draft?</span>
            <button onClick={() => setConfirmDiscard(false)} className="h-8 rounded-md px-3 text-[13px] text-muted hover:bg-hover hover:text-text">
              Keep editing
            </button>
            <button onClick={onClose} className="h-8 rounded-md bg-danger px-3 text-[13px] font-medium text-white hover:opacity-90">
              Discard
            </button>
          </>
        ) : (
          <>
            <Tip label="Send" shortcut="Ctrl ↵" side="top">
              <button
                onClick={send}
                className="flex h-8 items-center gap-2 rounded-md bg-primary px-4 text-[13px] font-medium text-primary-foreground transition-colors hover:bg-primary-hover"
              >
                Send
                <span className="text-[11px] font-normal text-primary-foreground/60">Ctrl ↵</span>
              </button>
            </Tip>
            <Tip label="Attach files" side="top">
              <button onClick={attach} aria-label="Attach files" className="rounded-md p-2 text-muted transition-colors hover:bg-hover hover:text-text">
                <Paperclip size={16} strokeWidth={1.75} />
              </button>
            </Tip>
            <div className="flex-1" />
            <Tip label="Discard draft" side="top">
              <button onClick={close} aria-label="Discard draft" className="rounded-md p-2 text-muted transition-colors hover:bg-hover hover:text-danger">
                <Trash2 size={16} strokeWidth={1.75} />
              </button>
            </Tip>
          </>
        )}
      </div>
    </div>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex min-h-9 items-center gap-2">
      <span className="w-14 shrink-0 text-[12.5px] text-subtle">{label}</span>
      {children}
    </div>
  );
}

function formatSize(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function AttachmentChip({ a, onRemove }: { a: AttachmentInfo; onRemove: () => void }) {
  return (
    <span className="flex max-w-[240px] items-center gap-1.5 rounded-md border border-border bg-bg py-1 pr-1 pl-2 text-[12px]">
      <Paperclip size={12} className="shrink-0 text-subtle" aria-hidden />
      <span className="truncate">{a.name}</span>
      <span className="shrink-0 text-subtle">{formatSize(a.size)}</span>
      <button onClick={onRemove} aria-label={`Remove ${a.name}`} className="rounded p-0.5 text-subtle hover:bg-hover hover:text-text">
        <X size={12} />
      </button>
    </span>
  );
}

const EMAIL_RE = /^[^\s@<>,;]+@[^\s@<>,;]+\.[^\s@<>,;]+$/;

/** Parses "Name <a@b.c>" or "a@b.c". */
function parseAddress(raw: string): Address | null {
  const s = raw.trim().replace(/[,;]+$/, "");
  const m = s.match(/^(.*)<([^>]+)>$/);
  const email = (m ? m[2] : s).trim();
  if (!EMAIL_RE.test(email)) return null;
  return { name: m ? m[1].trim().replace(/^"|"$/g, "") : "", email };
}

function RecipientInput(props: {
  value: Address[];
  onChange: (v: Address[]) => void;
  label: string;
  inputRef?: React.RefObject<HTMLInputElement | null>;
  autoFocus?: boolean;
}) {
  const [text, setText] = useState("");
  const [suggestions, setSuggestions] = useState<Address[]>([]);
  const [active, setActive] = useState(0);
  const [invalid, setInvalid] = useState(false);
  const localRef = useRef<HTMLInputElement>(null);
  const ref = props.inputRef ?? localRef;

  useEffect(() => {
    if (props.autoFocus) ref.current?.focus();
  }, [props.autoFocus, ref]);

  useEffect(() => {
    const q = text.trim();
    if (q.length < 1) return;
    let cancelled = false;
    const t = setTimeout(() => {
      api.suggestContacts(q).then((list) => {
        if (cancelled) return;
        const taken = new Set(props.value.map((a) => a.email.toLowerCase()));
        setSuggestions(list.filter((a) => !taken.has(a.email.toLowerCase())));
        setActive(0);
      });
    }, 120);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [text, props.value]);

  const shown = text.trim() ? suggestions : [];

  function add(a: Address) {
    if (!props.value.some((x) => x.email.toLowerCase() === a.email.toLowerCase())) props.onChange([...props.value, a]);
    setText("");
    setSuggestions([]);
    setInvalid(false);
  }

  /** Turns typed or pasted text into chips; returns false if something isn't an address. */
  function commit(raw: string): boolean {
    const parts = raw.split(/[,;\n]+/).map((p) => p.trim()).filter(Boolean);
    if (!parts.length) return true;
    const good: Address[] = [];
    const bad: string[] = [];
    for (const p of parts) {
      const a = parseAddress(p);
      if (a) good.push(a);
      else bad.push(p);
    }
    const next = [...props.value];
    for (const a of good) if (!next.some((x) => x.email.toLowerCase() === a.email.toLowerCase())) next.push(a);
    props.onChange(next);
    setText(bad.join(", "));
    setInvalid(bad.length > 0);
    setSuggestions([]);
    return bad.length === 0;
  }

  return (
    <div className="relative flex min-w-0 flex-1 flex-wrap items-center gap-1 py-1.5">
      {props.value.map((a) => (
        <Tip key={a.email} label={a.email} side="top">
        <span
          className="flex max-w-[220px] items-center gap-1 rounded-md bg-active py-0.5 pr-0.5 pl-2 text-[12.5px]"
        >
          <span className="truncate">{a.name || a.email}</span>
          <button
            onClick={() => props.onChange(props.value.filter((x) => x.email !== a.email))}
            aria-label={`Remove ${a.email}`}
            className="rounded p-0.5 text-subtle hover:bg-hover hover:text-text"
          >
            <X size={11} />
          </button>
        </span>
        </Tip>
      ))}
      <Input
        ref={ref}
        value={text}
        aria-label={props.label}
        aria-invalid={invalid}
        aria-autocomplete="list"
        onChange={(e) => {
          setInvalid(false);
          const v = e.target.value;
          if (/[,;]$/.test(v)) commit(v);
          else setText(v);
        }}
        onPaste={(e) => {
          const pasted = e.clipboardData.getData("text");
          if (/[,;\n]/.test(pasted) || pasted.includes("@")) {
            e.preventDefault();
            commit(text + pasted);
          }
        }}
        onKeyDown={(e) => {
          if (shown.length && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
            e.preventDefault();
            setActive((i) => (i + (e.key === "ArrowDown" ? 1 : shown.length - 1)) % shown.length);
          } else if ((e.key === "Enter" || e.key === "Tab") && shown.length && text.trim()) {
            e.preventDefault();
            add(shown[active]);
          } else if (e.key === "Enter" && !(e.ctrlKey || e.metaKey) && text.trim()) {
            e.preventDefault();
            commit(text);
          } else if (e.key === "Backspace" && !text && props.value.length) {
            props.onChange(props.value.slice(0, -1));
          }
        }}
        onBlur={() => setTimeout(() => text.trim() && commit(text), 120)}
        className={`h-7 w-auto min-w-[120px] flex-1 border-0 bg-transparent px-0 shadow-none focus-visible:ring-0 dark:bg-transparent rounded-none text-[13px] md:text-[13px] ${invalid ? "text-danger" : "text-text"}`}
      />
      {shown.length > 0 && (
        <ul role="listbox" className="absolute top-full left-0 z-10 mt-1 w-[min(360px,100%)] overflow-hidden rounded-lg border border-border bg-surface py-1 shadow-lg">
          {shown.map((a, i) => (
            <li
              key={a.email}
              role="option"
              aria-selected={i === active}
              onMouseDown={(e) => {
                e.preventDefault();
                add(a);
              }}
              onMouseEnter={() => setActive(i)}
              className={`px-3 py-1.5 text-[13px] ${i === active ? "bg-selected" : ""}`}
            >
              <div className="truncate font-medium">{a.name || a.email}</div>
              {a.name && <div className="truncate text-[12px] text-subtle">{a.email}</div>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
