"use client";

import { useState } from "react";
import {
  Archive,
  ChevronRight,
  Folder as FolderIcon,
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
import { api, type Account, type Folder, type FolderRole, type SyncStatus, type UpdateStatus } from "@/lib/api";
import { Tip } from "./Tip";
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
  folderId: string;
  onSelectFolder: (f: Folder) => void;
  onOpenSettings: (section?: Section) => void;
  onAddAccount: () => void;
  onSyncNow: () => void;
  onCompose: () => void;
  sync: SyncStatus | null;
  update: UpdateStatus | null;
  onInstallUpdate: () => void;
};

export default function Sidebar(props: Props) {
  const { accounts, folders, accountId, role, onSelectAccount, onSelectRole, onOpenSettings } = props;
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  // The user's own folders and labels (Gmail's system views are hidden).
  const ownFolders = (acct: string) =>
    folders
      .filter((f) => f.accountId === acct && f.role === "custom" && !f.path.startsWith("[Gmail]"))
      .sort((a, b) => a.path.localeCompare(b.path, undefined, { sensitivity: "base" }));
  const unread = (r: string, acct: string) =>
    folders
      .filter((f) => f.role === r && (acct === "" || f.accountId === acct))
      .reduce((n, f) => n + f.unread, 0);

  return (
    <aside className="flex min-w-0 flex-1 flex-col bg-pane-side">
      {/* Brand lives in the title bar; Compose takes the top row (aligned with Search). */}
      <div className="flex h-[52px] shrink-0 items-center px-3">
        <Tip label="New message" shortcut="C" side="right">
        <button
          onClick={props.onCompose}
          className="flex h-8 w-full items-center gap-2 rounded-md bg-primary px-3 text-[13px] font-medium text-primary-foreground transition-colors duration-150 hover:bg-primary-hover active:translate-y-px"
        >
          <PenSquare size={15} strokeWidth={2} aria-hidden />
          <span className="flex-1 text-left">Compose</span>
          <span className="text-[11px] font-normal text-primary-foreground/60">C</span>
        </button>
        </Tip>
      </div>

      <nav className="min-h-0 flex-1 overflow-y-auto px-2 pb-3" aria-label="Mailboxes">
        <ul>
          {folderItems.map(({ role: r, label, icon: Icon, count }) => {
            const active = role === r && !props.folderId;
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
          {accounts.map((a) => {
            const own = ownFolders(a.id);
            const open = expanded[a.id] ?? (accountId === a.id && own.length > 0);
            return (
              <AccountRow
                key={a.id}
                active={accountId === a.id && !props.folderId}
                label={a.name}
                sub={a.email}
                color={a.color}
                count={unread("inbox", a.id)}
                onClick={() => onSelectAccount(a.id)}
                expandable={own.length > 0}
                expanded={open}
                onToggle={() => setExpanded((e) => ({ ...e, [a.id]: !open }))}
              >
                {open && own.length > 0 && (
                  <ul aria-label={`${a.name} folders`}>
                    {own.map((f) => (
                      <FolderRow key={f.id} folder={f} active={props.folderId === f.id} onClick={() => props.onSelectFolder(f)} />
                    ))}
                  </ul>
                )}
              </AccountRow>
            );
          })}
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

      <UpdateBanner update={props.update} onInstall={props.onInstallUpdate} onDetails={() => onOpenSettings("about")} />
      <div className="flex h-11 shrink-0 items-center gap-1 border-t border-border pr-2.5 pl-4 text-[12px] text-subtle">
        <SyncLine sync={props.sync} accounts={accounts} onShowErrors={() => onOpenSettings("accounts")} />
        {accounts.some((a) => !a.demo) && (
          <Tip label="Check for new mail" side="top">
            <button
              onClick={props.onSyncNow}
              disabled={props.sync?.syncing}
              className="rounded-md p-1.5 transition-colors duration-150 hover:bg-hover hover:text-text disabled:opacity-40"
              aria-label="Check for new mail"
            >
              <RotateCw size={14} strokeWidth={1.75} />
            </button>
          </Tip>
        )}
        <Tip label="Settings" shortcut="Ctrl ," side="top">
          <button
            onClick={() => onOpenSettings()}
            className="rounded-md p-1.5 transition-colors duration-150 hover:bg-hover hover:text-text"
            aria-label="Settings"
          >
            <Settings size={15} strokeWidth={1.75} />
          </button>
        </Tip>
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
  expandable?: boolean;
  expanded?: boolean;
  onToggle?: () => void;
  children?: React.ReactNode;
}) {
  return (
    <li className="relative">
      {props.expandable && (
        <button
          onClick={props.onToggle}
          aria-label={`${props.expanded ? "Hide" : "Show"} ${props.label} folders`}
          aria-expanded={props.expanded}
          className="absolute top-[7px] left-0 z-10 rounded p-0.5 text-subtle transition-colors hover:text-text"
        >
          <ChevronRight size={12} strokeWidth={2} className={`transition-transform duration-150 ${props.expanded ? "rotate-90" : ""}`} />
        </button>
      )}
      <MaybeTip label={props.sub}>
      <button
        onClick={props.onClick}
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
      </MaybeTip>
      {props.children}
    </li>
  );
}

function FolderRow({ folder, active, onClick }: { folder: Folder; active: boolean; onClick: () => void }) {
  const depth = folder.path.split("/").length - 1;
  return (
    <li>
      <MaybeTip label={folder.path !== folder.name ? folder.path : undefined}>
      <button
        onClick={onClick}
        aria-current={active ? "page" : undefined}
        style={{ paddingLeft: 30 + depth * 14 }}
        className={`flex h-[28px] w-full items-center gap-2 rounded-md pr-2.5 text-[12.5px] transition-colors duration-150 ${
          active ? "bg-active font-medium text-text" : "text-muted hover:bg-hover hover:text-text"
        }`}
      >
        <FolderIcon size={13} strokeWidth={1.75} className={active ? "text-primary" : ""} aria-hidden />
        <span className="flex-1 truncate text-left">{folder.name}</span>
        {folder.unread > 0 && <span className="text-[11.5px] text-subtle tabular-nums">{folder.unread}</span>}
      </button>
      </MaybeTip>
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
      <Tip label={errors[0][1] ?? "Sync problem"} side="top">
      <button onClick={onShowErrors} className="flex min-w-0 flex-1 items-center gap-2 text-left hover:text-text">
        <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-star" aria-hidden />
        <span className="truncate">Couldn&apos;t sync {name}</span>
      </button>
      </Tip>
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

function UpdateBanner({ update, onInstall, onDetails }: { update: UpdateStatus | null; onInstall: () => void; onDetails: () => void }) {
  if (!update || (update.state !== "ready" && update.state !== "available")) return null;
  const ready = update.state === "ready";
  return (
    <div className="mx-2 mb-2 rounded-lg border border-border bg-surface px-3 py-2.5 text-[12.5px]" role="status">
      <div className="font-medium">{ready ? "Update ready" : "Update available"}</div>
      <div className="mt-0.5 text-[12px] text-subtle">Pigeon {update.latest}</div>
      <div className="mt-2 flex gap-1.5">
        {ready ? (
          <button
            onClick={onInstall}
            className="h-7 flex-1 rounded-md bg-primary text-[12.5px] font-medium text-primary-foreground transition-colors hover:bg-primary-hover"
          >
            Restart to update
          </button>
        ) : (
          <button
            onClick={() => api.openExternal(update.url)}
            className="h-7 flex-1 rounded-md bg-primary text-[12.5px] font-medium text-primary-foreground transition-colors hover:bg-primary-hover"
          >
            Download
          </button>
        )}
        <button onClick={onDetails} className="h-7 rounded-md px-2 text-[12.5px] text-muted transition-colors hover:bg-hover hover:text-text">
          Details
        </button>
      </div>
    </div>
  );
}

/** Tooltip only when there's something to say (e.g. an account's address). */
function MaybeTip({ label, children }: { label?: string; children: React.ReactElement }) {
  return label ? (
    <Tip label={label} side="right">
      {children}
    </Tip>
  ) : (
    children
  );
}
