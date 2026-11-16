"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  api,
  errorMessage,
  onEvent,
  type Account,
  type Folder,
  type FolderRole,
  type MessageSummary,
  type SyncStatus,
} from "@/lib/api";
import { applyTheme, type ThemeChoice } from "@/lib/theme";
import Sidebar from "./Sidebar";
import MessageList from "./MessageList";
import Reader from "./Reader";
import Splitter from "./Splitter";
import SettingsDialog, { type Section } from "./SettingsDialog";
import AddAccountDialog from "./AddAccountDialog";
import Welcome from "./Welcome";

const SIDEBAR = { min: 184, max: 340, default: 232 };
const LIST = { min: 300, max: 620, default: 380 };

/** Pane width remembered across launches (per machine). */
function usePaneWidth(key: string, fallback: number) {
  const [width, setWidth] = useState(() => {
    try {
      const v = Number(localStorage.getItem(key));
      return v > 0 ? v : fallback;
    } catch {
      return fallback;
    }
  });
  useEffect(() => {
    try {
      localStorage.setItem(key, String(width));
    } catch {
      /* storage unavailable: width just won't persist */
    }
  }, [key, width]);
  return [width, setWidth] as const;
}

type Toast = { text: string; tone?: "error"; undo?: () => void };

export default function Mailbox() {
  const [accounts, setAccounts] = useState<Account[] | null>(null); // null = still loading
  const [folders, setFolders] = useState<Folder[]>([]);
  const [accountId, setAccountId] = useState(""); // "" = all inboxes
  const [role, setRole] = useState<FolderRole>("inbox" as FolderRole);
  const [query, setQuery] = useState("");
  const [messages, setMessages] = useState<MessageSummary[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [toast, setToast] = useState<Toast | null>(null);
  const [reload, setReload] = useState(0);
  const [sync, setSync] = useState<SyncStatus | null>(null);
  const [theme, setTheme] = useState<ThemeChoice>("system");
  const [settingsSection, setSettingsSection] = useState<Section | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [sidebarW, setSidebarW] = usePaneWidth("pigeon.pane.sidebar", SIDEBAR.default);
  const [listW, setListW] = usePaneWidth("pigeon.pane.list", LIST.default);
  const searchRef = useRef<HTMLInputElement>(null);

  const fail = useCallback((err: unknown) => setToast({ text: errorMessage(err), tone: "error" }), []);

  const refreshAccounts = useCallback(() => {
    api.listAccounts().then(setAccounts, fail);
    api.listFolders("").then(setFolders, fail);
  }, [fail]);

  // Initial load: accounts, theme (Go settings are the source of truth), sync status.
  useEffect(() => {
    refreshAccounts();
    api.getSettings().then((s) => {
      const t = (s.theme || "system") as ThemeChoice;
      setTheme(t);
      applyTheme(t);
    });
    api.getSyncStatus().then(setSync);
  }, [refreshAccounts]);

  // Live updates from background sync.
  useEffect(() => {
    const offMail = onEvent<string>("mail:changed", () => {
      refreshAccounts();
      setReload((n) => n + 1);
    });
    const offSync = onEvent<SyncStatus>("sync:status", setSync);
    return () => {
      offMail();
      offSync();
    };
  }, [refreshAccounts]);

  // If the chosen account was removed, show all inboxes.
  const activeAccount = accountId && accounts?.some((a) => a.id === accountId) ? accountId : "";

  useEffect(() => {
    let cancelled = false;
    api.listMessages(activeAccount, role, query).then((list) => {
      if (cancelled) return;
      setMessages(list);
      setSelectedId((cur) => (cur && list.some((m) => m.id === cur) ? cur : null));
    }, fail);
    return () => {
      cancelled = true;
    };
  }, [activeAccount, role, query, reload, fail]);

  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), toast.tone === "error" ? 8000 : 5000);
    return () => clearTimeout(t);
  }, [toast]);

  const accountsById = useMemo(() => new Map((accounts ?? []).map((a) => [a.id, a])), [accounts]);
  const selected = messages.find((m) => m.id === selectedId) ?? null;

  const changeTheme = useCallback((t: ThemeChoice) => {
    setTheme(t);
    applyTheme(t);
    api.setTheme(t);
  }, []);

  const patch = useCallback((id: string, change: Partial<MessageSummary>) => {
    setMessages((list) => list.map((m) => (m.id === id ? { ...m, ...change } : m)));
  }, []);

  const select = useCallback(
    (m: MessageSummary) => {
      setSelectedId(m.id);
      if (m.unread) {
        patch(m.id, { unread: false });
        api.setRead(m.id, true).then(refreshAccounts, (err) => {
          patch(m.id, { unread: true });
          fail(err);
        });
      }
    },
    [patch, refreshAccounts, fail],
  );

  const toggleStar = useCallback(
    (m: MessageSummary) => {
      patch(m.id, { starred: !m.starred });
      api.setStarred(m.id, !m.starred).catch((err) => {
        patch(m.id, { starred: m.starred });
        fail(err);
      });
    },
    [patch, fail],
  );

  const toggleRead = useCallback(
    (m: MessageSummary) => {
      patch(m.id, { unread: !m.unread });
      api.setRead(m.id, m.unread).then(refreshAccounts, (err) => {
        patch(m.id, { unread: m.unread });
        fail(err);
      });
    },
    [patch, refreshAccounts, fail],
  );

  /** Archive or delete, select the next message, offer undo. */
  const move = useCallback(
    async (m: MessageSummary, to: FolderRole, verb: string) => {
      const idx = messages.findIndex((x) => x.id === m.id);
      const next = messages[idx + 1] ?? messages[idx - 1] ?? null;
      setMessages((list) => list.filter((x) => x.id !== m.id));
      setSelectedId(next ? next.id : null);
      try {
        const res = await api.move(m.id, to);
        refreshAccounts();
        setToast({
          text: verb,
          undo: res.id
            ? async () => {
                setToast(null);
                try {
                  const back = await api.move(res.id, res.from);
                  setReload((n) => n + 1);
                  refreshAccounts();
                  if (back.id) setSelectedId(back.id);
                } catch (err) {
                  fail(err);
                }
              }
            : undefined,
        });
      } catch (err) {
        setReload((n) => n + 1); // put it back in the list
        fail(err);
      }
    },
    [messages, refreshAccounts, fail],
  );
  const archive = useCallback((m: MessageSummary) => move(m, "archive" as FolderRole, "Archived"), [move]);
  const trash = useCallback((m: MessageSummary) => move(m, "trash" as FolderRole, "Moved to Trash"), [move]);

  const dialogOpen = settingsSection !== null || addOpen;

  // Keyboard-first: j/k move, e archive, #/Delete trash, s star, u read, / or Ctrl+K search, z undo.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (dialogOpen) return;
      if (e.key === "," && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        setSettingsSection("appearance");
        return;
      }
      const target = e.target as HTMLElement;
      if (target.tagName === "INPUT" || target.tagName === "TEXTAREA") {
        if (e.key === "Escape") {
          setQuery("");
          target.blur();
        }
        return;
      }
      if (target.getAttribute("role") === "separator") return;
      if ((e.key === "k" && (e.ctrlKey || e.metaKey)) || e.key === "/") {
        e.preventDefault();
        searchRef.current?.focus();
        return;
      }
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      const idx = messages.findIndex((m) => m.id === selectedId);
      if (e.key === "j" || e.key === "ArrowDown") {
        e.preventDefault();
        const next = messages[Math.min(idx + 1, messages.length - 1)];
        if (next) select(next);
      } else if (e.key === "k" || e.key === "ArrowUp") {
        e.preventDefault();
        const prev = messages[Math.max(idx - 1, 0)];
        if (prev) select(prev);
      } else if (e.key === "z" && toast?.undo) {
        toast.undo();
      } else if (!selected) {
        return;
      } else if (e.key === "s") {
        toggleStar(selected);
      } else if (e.key === "u") {
        toggleRead(selected);
      } else if (e.key === "e") {
        archive(selected);
      } else if (e.key === "#" || e.key === "Delete") {
        trash(selected);
      } else if (e.key === "Escape") {
        setSelectedId(null);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [dialogOpen, messages, selectedId, selected, toast, select, toggleStar, toggleRead, archive, trash]);

  const dialogs = (
    <>
      {settingsSection && (
        <SettingsDialog
          initialSection={settingsSection}
          theme={theme}
          onThemeChange={changeTheme}
          onClose={() => setSettingsSection(null)}
          accounts={accounts ?? []}
          hasSamples={(accounts ?? []).some((a) => a.demo)}
          syncErrors={sync?.errors ?? {}}
          onAddAccount={() => {
            setSettingsSection(null);
            setAddOpen(true);
          }}
          onRemoveAccount={async (id) => {
            try {
              await api.removeAccount(id);
              refreshAccounts();
              setReload((n) => n + 1);
            } catch (err) {
              fail(err);
            }
          }}
          onRemoveSamples={async () => {
            try {
              await api.removeSampleData();
              refreshAccounts();
              setReload((n) => n + 1);
            } catch (err) {
              fail(err);
            }
          }}
        />
      )}
      {addOpen && (
        <AddAccountDialog
          onClose={() => setAddOpen(false)}
          onAdded={() => {
            setAddOpen(false);
            refreshAccounts();
            setReload((n) => n + 1);
            setToast({ text: "Account added. Syncing your mail…" });
          }}
        />
      )}
      {toast && (
        <div
          role={toast.tone === "error" ? "alert" : "status"}
          className="absolute bottom-5 left-1/2 z-[60] flex max-w-[min(560px,90vw)] items-center gap-4 rounded-lg bg-toast py-1.5 pr-1.5 pl-4 text-[13px] text-toast-fg shadow-lg [animation:toast-in_160ms_ease-out]"
        >
          {toast.tone === "error" && <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-danger" aria-hidden />}
          <span className="py-1">{toast.text}</span>
          {toast.undo ? (
            <button
              onClick={toast.undo}
              className="shrink-0 rounded-md px-2.5 py-1 font-medium text-toast-action transition-colors duration-150 hover:bg-white/10"
            >
              Undo <span className="ml-1 opacity-60">Z</span>
            </button>
          ) : (
            <button
              onClick={() => setToast(null)}
              aria-label="Dismiss"
              className="shrink-0 rounded-md px-2 py-1 text-toast-fg/60 transition-colors hover:bg-white/10 hover:text-toast-fg"
            >
              ✕
            </button>
          )}
        </div>
      )}
    </>
  );

  if (accounts === null) return <div className="h-screen bg-bg" />;

  if (accounts.length === 0) {
    return (
      <div className="relative">
        <Welcome
          onAddAccount={() => setAddOpen(true)}
          onTrySamples={async () => {
            try {
              await api.loadSampleData();
              refreshAccounts();
              setReload((n) => n + 1);
            } catch (err) {
              fail(err);
            }
          }}
        />
        {dialogs}
      </div>
    );
  }

  return (
    <div className="relative flex h-screen bg-bg text-text">
      <div style={{ width: sidebarW }} className="flex min-w-[168px] shrink-[1]">
        <Sidebar
          accounts={accounts}
          folders={folders}
          accountId={activeAccount}
          role={role}
          sync={sync}
          onSelectAccount={(id) => {
            setAccountId(id);
            setSelectedId(null);
          }}
          onSelectRole={(r) => {
            setRole(r);
            setSelectedId(null);
          }}
          onOpenSettings={(section) => setSettingsSection(section ?? "appearance")}
          onAddAccount={() => setAddOpen(true)}
          onSyncNow={() => api.syncNow()}
        />
      </div>
      <Splitter label="Resize sidebar" value={sidebarW} min={SIDEBAR.min} max={SIDEBAR.max} defaultValue={SIDEBAR.default} onChange={setSidebarW} />
      <div style={{ width: listW }} className="flex min-w-[260px] shrink-[2]">
        <MessageList
          messages={messages}
          accountsById={accountsById}
          showAccount={activeAccount === "" && accounts.length > 1}
          role={role}
          accountName={accountsById.get(activeAccount)?.name ?? "All inboxes"}
          selectedId={selectedId}
          query={query}
          onQuery={setQuery}
          searchRef={searchRef}
          onSelect={select}
          onToggleStar={toggleStar}
          onArchive={archive}
          onTrash={trash}
        />
      </div>
      <Splitter label="Resize message list" value={listW} min={LIST.min} max={LIST.max} defaultValue={LIST.default} onChange={setListW} />
      <Reader
        message={selected}
        account={selected ? accountsById.get(selected.accountId) : undefined}
        onToggleStar={toggleStar}
        onToggleRead={toggleRead}
        onArchive={archive}
        onTrash={trash}
      />
      {dialogs}
    </div>
  );
}
