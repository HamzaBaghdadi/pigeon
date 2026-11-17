"use client";

import { useEffect, useRef } from "react";
import { Archive, Loader2, Paperclip, Search, Star, Trash2, X } from "lucide-react";
import type { Account, FolderRole, MessageSummary } from "@/lib/api";
import { shortDate } from "@/lib/format";
import { Input } from "@/components/ui/input";
import { Tip } from "./Tip";

const roleTitle: Record<string, string> = {
  inbox: "Inbox",
  starred: "Starred",
  sent: "Sent",
  drafts: "Drafts",
  archive: "Archive",
  spam: "Spam",
  trash: "Trash",
};

type Props = {
  messages: MessageSummary[];
  accountsById: Map<string, Account>;
  showAccount: boolean;
  role: FolderRole;
  accountName: string;
  selectedId: string | null;
  query: string;
  onQuery: (q: string) => void;
  searchRef: React.RefObject<HTMLInputElement | null>;
  onSelect: (m: MessageSummary) => void;
  onToggleStar: (m: MessageSummary) => void;
  onArchive: (m: MessageSummary) => void;
  onTrash: (m: MessageSummary) => void;
  folderName?: string;
  canLoadOlder: boolean;
  olderLoading: boolean;
  olderDone: boolean;
  onLoadOlder: () => void;
  serverSearchBusy: boolean;
  serverSearchMatched: number | null;
  onSearchServer: () => void;
};

export default function MessageList({ searchRef, ...p }: Props) {
  const unreadCount = p.messages.filter((m) => m.unread).length;
  const listRef = useRef<HTMLUListElement>(null);

  // Keep the keyboard selection in view.
  useEffect(() => {
    if (!p.selectedId) return;
    listRef.current?.querySelector(`[data-id="${p.selectedId}"]`)?.scrollIntoView({ block: "nearest" });
  }, [p.selectedId]);

  return (
    <section className="flex min-w-0 flex-1 flex-col bg-pane-list" aria-label="Message list">
      <div className="flex h-[52px] shrink-0 items-center px-3">
        <label className="relative flex-1">
          <span className="sr-only">Search mail</span>
          <Search
            size={15}
            strokeWidth={1.75}
            className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-subtle"
            aria-hidden
          />
          <Input
            ref={searchRef}
            value={p.query}
            onChange={(e) => p.onQuery(e.target.value)}
            placeholder="Search"
            className="h-8 bg-bg pr-16 pl-8 text-[13px] md:text-[13px] dark:bg-bg"
          />
          {p.query ? (
            <button
              onClick={() => p.onQuery("")}
              className="absolute top-1/2 right-1.5 -translate-y-1/2 rounded p-1 text-subtle transition-colors hover:bg-hover hover:text-text"
              aria-label="Clear search"
            >
              <X size={14} />
            </button>
          ) : (
            <span className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-[11px] text-subtle">
              Ctrl K
            </span>
          )}
        </label>
      </div>

      <div className="flex h-9 shrink-0 items-baseline gap-2 border-b border-border px-4">
        <h1 className="display-heading truncate font-display text-[15px] font-semibold tracking-[-0.01em]">{p.folderName ?? roleTitle[p.role] ?? p.role}</h1>
        <span className="truncate text-[12px] text-subtle">
          {p.accountName}
          {unreadCount > 0 && ` · ${unreadCount} unread`}
        </span>
      </div>

      <div
        className="min-h-0 flex-1 overflow-y-auto"
        onScroll={(e) => {
          // Keep scrolling to load older mail (not while searching).
          const el = e.currentTarget;
          if (!p.query && p.canLoadOlder && !p.olderDone && el.scrollHeight - el.scrollTop - el.clientHeight < 400) p.onLoadOlder();
        }}
      >
      {p.messages.length === 0 ? (
        <EmptyState searching={p.query !== ""} />
      ) : (
        <ul ref={listRef} role="listbox" aria-label="Messages">
          {p.messages.map((m) => (
            <Row
              key={m.id}
              m={m}
              selected={m.id === p.selectedId}
              account={p.showAccount ? p.accountsById.get(m.accountId) : undefined}
              outgoing={p.role === "sent" || p.role === "drafts"}
              onSelect={() => p.onSelect(m)}
              onToggleStar={() => p.onToggleStar(m)}
              onArchive={() => p.onArchive(m)}
              onTrash={() => p.onTrash(m)}
            />
          ))}
        </ul>
      )}
      {p.canLoadOlder && <ListFooter {...p} empty={p.messages.length === 0} />}
      </div>
    </section>
  );
}

function Row(props: {
  m: MessageSummary;
  selected: boolean;
  account?: Account;
  outgoing?: boolean;
  onSelect: () => void;
  onToggleStar: () => void;
  onArchive: () => void;
  onTrash: () => void;
}) {
  const { m, selected, account } = props;
  return (
    <li
      data-id={m.id}
      role="option"
      aria-selected={selected}
      onClick={props.onSelect}
      className={`group relative border-b border-border py-2.5 pr-4 pl-6 transition-colors duration-100 ${
        selected ? "bg-selected" : "hover:bg-hover"
      }`}
    >
      {/* Account stripe (unified view only); the dot is reserved for "unread". */}
      {account && (
        <span
          className="absolute inset-y-0 left-0 w-[3px]"
          style={{ background: account.color }}
          aria-label={`Account: ${account.name}`}
        />
      )}
      {m.unread && <span className="absolute top-[17px] left-2.5 h-1.5 w-1.5 rounded-full bg-primary" aria-label="Unread" />}

      <div className="flex items-center gap-2">
        <span className={`min-w-0 flex-1 truncate text-[13.5px] ${m.unread ? "font-semibold text-text" : "text-text"}`}>
          {props.outgoing ? `To: ${(m.to ?? []).map((t) => t.name || t.email).join(", ") || "(no recipients)"}` : m.from.name || m.from.email}
        </span>
        <span className="flex shrink-0 items-center gap-1.5 group-hover:invisible">
          {m.hasAttachments && <Paperclip size={12} className="text-subtle" aria-label="Has attachments" />}
          {m.starred && <Star size={12} className="text-star" fill="currentColor" aria-label="Starred" />}
          <span className={`text-[12px] tabular-nums ${m.unread ? "font-medium text-text" : "text-subtle"}`}>
            {shortDate(m.date)}
          </span>
        </span>
      </div>
      <div className={`mt-px truncate text-[13px] ${m.unread ? "font-medium text-text" : "text-muted"}`}>
        {m.subject || "(no subject)"}
      </div>
      <div className="mt-px truncate text-[12.5px] text-subtle">{m.snippet}</div>

      {/* Quick actions replace the date on hover */}
      <div className="invisible absolute top-1.5 right-2 flex items-center rounded-md border border-border bg-surface shadow-sm group-hover:visible">
        <QuickAction label="Archive (E)" onClick={props.onArchive} icon={<Archive size={14} strokeWidth={1.75} />} />
        <QuickAction label="Delete (#)" onClick={props.onTrash} icon={<Trash2 size={14} strokeWidth={1.75} />} />
        <QuickAction
          label={m.starred ? "Unstar (S)" : "Star (S)"}
          onClick={props.onToggleStar}
          active={m.starred}
          icon={<Star size={14} strokeWidth={1.75} fill={m.starred ? "currentColor" : "none"} />}
        />
      </div>
    </li>
  );
}

function QuickAction(props: { label: string; icon: React.ReactNode; active?: boolean; onClick: () => void }) {
  return (
    <Tip label={props.label} side="top">
    <button
      aria-label={props.label}
      onClick={(e) => {
        e.stopPropagation();
        props.onClick();
      }}
      className={`p-1.5 transition-colors duration-100 first:rounded-l-md last:rounded-r-md hover:bg-hover ${
        props.active ? "text-star" : "text-muted hover:text-text"
      }`}
    >
      {props.icon}
    </button>
    </Tip>
  );
}

function EmptyState({ searching }: { searching: boolean }) {
  return (
    <div className="flex flex-col items-center justify-center px-8 pt-16 pb-6 text-center">
      <p className="text-[13.5px] font-medium">{searching ? "No results" : "Nothing here"}</p>
      <p className="mt-1 max-w-[240px] text-[12.5px] text-subtle">
        {searching ? "Try a sender name, subject or keyword." : "New mail will show up here."}
      </p>
    </div>
  );
}

function ListFooter(p: Omit<Props, "searchRef"> & { empty: boolean }) {
  const base = "flex items-center justify-center gap-2 px-4 py-4 text-[12.5px] text-subtle";
  const button = "rounded-md px-3 py-1.5 font-medium text-primary transition-colors hover:bg-primary-soft";
  if (p.query.trim()) {
    if (p.serverSearchBusy)
      return (
        <div className={base} role="status">
          <Loader2 size={13} className="animate-spin" aria-hidden />
          Searching all mail on the server…
        </div>
      );
    if (p.serverSearchMatched !== null)
      return (
        <div className={base} role="status">
          {p.serverSearchMatched === 0
            ? "No other matches on the server."
            : `${p.serverSearchMatched} ${p.serverSearchMatched === 1 ? "match" : "matches"} on the server. The newest are shown above.`}
        </div>
      );
    return (
      <div className={base}>
        {p.empty ? "Not in downloaded mail." : "Looking for something older?"}
        <button onClick={p.onSearchServer} className={button}>
          Search all mail on the server
        </button>
      </div>
    );
  }
  if (p.empty) return null;
  if (p.olderLoading)
    return (
      <div className={base} role="status">
        <Loader2 size={13} className="animate-spin" aria-hidden />
        Loading older messages…
      </div>
    );
  if (p.olderDone) return <div className={base}>That&apos;s everything in this folder.</div>;
  return (
    <div className={base}>
      <button onClick={p.onLoadOlder} className={button}>
        Load older messages
      </button>
    </div>
  );
}
