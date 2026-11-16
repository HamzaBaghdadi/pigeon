"use client";

import { useEffect, useRef } from "react";
import { Archive, Paperclip, Search, Star, Trash2, X } from "lucide-react";
import type { Account, FolderRole, MessageSummary } from "@/lib/api";
import { shortDate } from "@/lib/format";

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
    <section className="flex min-w-0 flex-1 flex-col bg-surface" aria-label="Message list">
      <div className="flex h-[52px] shrink-0 items-center px-3">
        <label className="relative flex-1">
          <span className="sr-only">Search mail</span>
          <Search
            size={15}
            strokeWidth={1.75}
            className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-subtle"
            aria-hidden
          />
          <input
            ref={searchRef}
            value={p.query}
            onChange={(e) => p.onQuery(e.target.value)}
            placeholder="Search"
            className="h-8 w-full rounded-md border border-border bg-bg pr-16 pl-8 text-[13px] text-text outline-none transition-[border-color,box-shadow] duration-150 placeholder:text-subtle hover:border-border-strong focus:border-primary focus:shadow-[0_0_0_3px_var(--primary-soft)]"
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
        <h1 className="text-[15px] font-semibold tracking-[-0.01em]">{roleTitle[p.role] ?? p.role}</h1>
        <span className="truncate text-[12px] text-subtle">
          {p.accountName}
          {unreadCount > 0 && ` · ${unreadCount} unread`}
        </span>
      </div>

      {p.messages.length === 0 ? (
        <EmptyState searching={p.query !== ""} />
      ) : (
        <ul ref={listRef} className="min-h-0 flex-1 overflow-y-auto" role="listbox" aria-label="Messages">
          {p.messages.map((m) => (
            <Row
              key={m.id}
              m={m}
              selected={m.id === p.selectedId}
              account={p.showAccount ? p.accountsById.get(m.accountId) : undefined}
              onSelect={() => p.onSelect(m)}
              onToggleStar={() => p.onToggleStar(m)}
              onArchive={() => p.onArchive(m)}
              onTrash={() => p.onTrash(m)}
            />
          ))}
        </ul>
      )}
    </section>
  );
}

function Row(props: {
  m: MessageSummary;
  selected: boolean;
  account?: Account;
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
          title={account.name}
          aria-label={`Account: ${account.name}`}
        />
      )}
      {m.unread && <span className="absolute top-[17px] left-2.5 h-1.5 w-1.5 rounded-full bg-primary" aria-label="Unread" />}

      <div className="flex items-center gap-2">
        <span className={`min-w-0 flex-1 truncate text-[13.5px] ${m.unread ? "font-semibold text-text" : "text-text"}`}>
          {m.from.name || m.from.email}
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
    <button
      title={props.label}
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
  );
}

function EmptyState({ searching }: { searching: boolean }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center px-8 text-center">
      <p className="text-[13.5px] font-medium">{searching ? "No results" : "Nothing here"}</p>
      <p className="mt-1 max-w-[240px] text-[12.5px] text-subtle">
        {searching ? "Try a sender name, subject or keyword." : "New mail will show up here."}
      </p>
    </div>
  );
}
