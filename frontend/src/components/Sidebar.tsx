"use client";

import {
  Archive,
  File,
  Inbox,
  Layers,
  type LucideIcon,
  Loader2,
  PenSquare,
  Plus,
  RotateCw,
  Send,
  Settings,
  ShieldAlert,
  Star,
  Trash2,
} from "lucide-react";
import type { Account, Folder, FolderRole, SyncStatus } from "@/lib/api";
import type { Section } from "./SettingsDialog";

const folderItems: { role: string; label: string; icon: LucideIcon; count?: boolean }[] = [
  { role: "inbox", label: "Inbox", icon: Inbox, count: true },
  { role: "starred", label: "Starred", icon: Star },
  { role: "sent", label: "Sent", icon: Send },
  { role: "drafts", label: "Drafts", icon: File },
  { role: "archive", label: "Archive", icon: Archive },
  { role: "spam", label: "Spam", icon: ShieldAlert, count: true },
  { role: "trash", label: "Trash", icon: Trash2 },
];

type Props = {
  accounts: Account[];
  folders: Folder[];
  accountId: string;
  role: FolderRole;
  onSelectAccount: (id: string) => void;
  onSelectRole: (role: FolderRole) => void;
  onOpenSettings: (section?: Section) => void;
  onAddAccount: () => void;
  onSyncNow: () => void;
  sync: SyncStatus | null;
};

export default function Sidebar(props: Props) {
  const { accounts, folders, accountId, role, onSelectAccount, onSelectRole, onOpenSettings } = props;
  const unread = (r: string, acct: string) =>
    folders
      .filter((f) => f.role === r && (acct === "" || f.accountId === acct))
      .reduce((n, f) => n + f.unread, 0);

  return (
    <aside className="flex min-w-0 flex-1 flex-col bg-sidebar">
      <div className="flex h-[52px] shrink-0 items-center gap-2 px-4">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src="/pigeon.png" alt="" className="h-[22px] w-[22px]" draggable={false} />
        <span className="text-[14px] font-semibold tracking-[-0.01em]">Pigeon</span>
      </div>

      <div className="px-3 pb-3">
        <button
          title="Compose (C)"
          className="flex h-8 w-full items-center gap-2 rounded-md bg-primary px-3 text-[13px] font-medium text-white transition-colors duration-150 hover:bg-primary-hover active:translate-y-px"
        >
          <PenSquare size={15} strokeWidth={2} aria-hidden />
          <span className="flex-1 text-left">Compose</span>
          <span className="text-[11px] font-normal text-white/60">C</span>
        </button>
      </div>

      <nav className="min-h-0 flex-1 overflow-y-auto px-2 pb-3" aria-label="Mailboxes">
        <ul>
          {folderItems.map(({ role: r, label, icon: Icon, count }) => {
            const active = role === r;
            const n = count ? unread(r, accountId) : 0;
            return (
              <li key={r}>
                <button
                  onClick={() => onSelectRole(r as FolderRole)}
                  aria-current={active ? "page" : undefined}
                  className={`flex h-[30px] w-full items-center gap-2.5 rounded-md px-2.5 text-[13px] transition-colors duration-150 ${
                    active ? "bg-active font-medium text-text" : "text-muted hover:bg-hover hover:text-text"
                  }`}
                >
                  <Icon size={16} strokeWidth={1.75} className={active ? "text-primary" : ""} aria-hidden />
                  <span className="flex-1 truncate text-left">{label}</span>
                  {n > 0 && (
                    <span className={`text-[12px] tabular-nums ${active ? "font-semibold text-primary" : "text-subtle"}`}>
                      {n}
                    </span>
                  )}
                </button>
              </li>
            );
          })}
        </ul>

        <h2 className="mt-5 mb-1 px-2.5 text-[12px] font-medium text-subtle">Accounts</h2>
        <ul>
          <AccountRow
            active={accountId === ""}
            label="All inboxes"
            count={unread("inbox", "")}
            onClick={() => onSelectAccount("")}
          />
          {accounts.map((a) => (
            <AccountRow
              key={a.id}
              active={accountId === a.id}
              label={a.name}
              sub={a.email}
              color={a.color}
              count={unread("inbox", a.id)}
              onClick={() => onSelectAccount(a.id)}
            />
          ))}
          <li>
            <button
              onClick={props.onAddAccount}
              className="flex h-[30px] w-full items-center gap-2.5 rounded-md px-2.5 text-[13px] text-subtle transition-colors duration-150 hover:bg-hover hover:text-text"
            >
              <Plus size={15} strokeWidth={1.75} aria-hidden />
              Add account
            </button>
          </li>
        </ul>
      </nav>

      <div className="flex h-11 shrink-0 items-center gap-1 border-t border-border pr-2.5 pl-4 text-[12px] text-subtle">
        <SyncLine sync={props.sync} accounts={accounts} onShowErrors={() => onOpenSettings("accounts")} />
        {accounts.some((a) => !a.demo) && (
          <button
            onClick={props.onSyncNow}
            disabled={props.sync?.syncing}
            className="rounded-md p-1.5 transition-colors duration-150 hover:bg-hover hover:text-text disabled:opacity-40"
            title="Check for new mail"
            aria-label="Check for new mail"
          >
            <RotateCw size={14} strokeWidth={1.75} />
          </button>
        )}
        <button
          onClick={() => onOpenSettings()}
          className="rounded-md p-1.5 transition-colors duration-150 hover:bg-hover hover:text-text"
          title="Settings (Ctrl+,)"
          aria-label="Settings"
        >
          <Settings size={15} strokeWidth={1.75} />
        </button>
      </div>
    </aside>
  );
}

function AccountRow(props: {
  active: boolean;
  label: string;
  sub?: string;
  color?: string;
  count: number;
  onClick: () => void;
}) {
  return (
    <li>
      <button
        onClick={props.onClick}
        title={props.sub}
        aria-current={props.active ? "true" : undefined}
        className={`flex h-[30px] w-full items-center gap-2.5 rounded-md px-2.5 text-[13px] transition-colors duration-150 ${
          props.active ? "bg-active font-medium text-text" : "text-muted hover:bg-hover hover:text-text"
        }`}
      >
        <span className="flex w-4 justify-center" aria-hidden>
          {props.color ? (
            <span className="h-2 w-2 rounded-[3px]" style={{ background: props.color }} />
          ) : (
            <Layers size={14} strokeWidth={1.75} />
          )}
        </span>
        <span className="flex-1 truncate text-left">{props.label}</span>
        {props.count > 0 && <span className="text-[12px] text-subtle tabular-nums">{props.count}</span>}
      </button>
    </li>
  );
}

function SyncLine({ sync, accounts, onShowErrors }: { sync: SyncStatus | null; accounts: Account[]; onShowErrors: () => void }) {
  const real = accounts.filter((a) => !a.demo);
  const errors = Object.entries(sync?.errors ?? {}).filter(([, v]) => v);
  if (real.length === 0) {
    return <span className="flex-1 truncate">Sample mail</span>;
  }
  if (sync?.syncing) {
    return (
      <span className="flex flex-1 items-center gap-2 truncate" role="status">
        <Loader2 size={12} className="animate-spin" aria-hidden />
        Syncing…
      </span>
    );
  }
  if (errors.length > 0) {
    const name = real.find((a) => a.id === errors[0][0])?.name ?? "an account";
    return (
      <button onClick={onShowErrors} className="flex min-w-0 flex-1 items-center gap-2 text-left hover:text-text" title={errors[0][1]}>
        <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-star" aria-hidden />
        <span className="truncate">Couldn&apos;t sync {name}</span>
      </button>
    );
  }
  return (
    <span className="flex flex-1 items-center gap-2 truncate">
      <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-emerald-500" aria-hidden />
      {sync?.lastSync && !sync.lastSync.startsWith("0001") ? `Updated ${ago(sync.lastSync)}` : "Connecting…"}
    </span>
  );
}

function ago(iso: string): string {
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  return new Date(iso).toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
}
