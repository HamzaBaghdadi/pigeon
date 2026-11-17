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
  type UpdateStatus,
  type Draft,
} from "@/lib/api";
import { applyTheme, type ThemeChoice } from "@/lib/theme";
import Sidebar from "./Sidebar";
import MessageList from "./MessageList";
import Reader from "./Reader";
import Splitter from "./Splitter";
import SettingsDialog, { type Section } from "./SettingsDialog";
import AddAccountDialog from "./AddAccountDialog";
import Welcome from "./Welcome";
import Composer, { type ComposerState } from "./Composer";
import MovePicker from "./MovePicker";

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

type Toast = { text: string; tone?: "error"; undo?: () => void; undoLabel?: string };

const UNDO_SEND_MS = 5000;

export default function Mailbox() {
  const [accounts, setAccounts] = useState<Account[] | null>(null); // null = still loading
  const [folders, setFolders] = useState<Folder[]>([]);
  const [accountId, setAccountId] = useState(""); // "" = all inboxes
  const [role, setRole] = useState<FolderRole>("inbox" as FolderRole);
  const [folderId, setFolderId] = useState(""); // a specific folder, or "" for a role view
  const [history, setHistory] = useState<{ key: string; loading: boolean; done: boolean }>({ key: "", loading: false, done: false });
  const [serverSearch, setServerSearch] = useState<{ key: string; busy: boolean; matched: number | null }>({ key: "", busy: false, matched: null });
  const [moving, setMoving] = useState<MessageSummary | null>(null);
  const [query, setQuery] = useState("");
  const [messages, setMessages] = useState<MessageSummary[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [toast, setToast] = useState<Toast | null>(null);
  const [reload, setReload] = useState(0);
  const [sync, setSync] = useState<SyncStatus | null>(null);
  const [update, setUpdate] = useState<UpdateStatus | null>(null);
  const [theme, setTheme] = useState<ThemeChoice>("system");
  const [settingsSection, setSettingsSection] = useState<Section | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [composer, setComposer] = useState<ComposerState | null>(null);
  const pendingOpen = useRef<string | null>(null);
  const sendTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
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
    api.getUpdateStatus().then(setUpdate);
  }, [refreshAccounts]);

  // Live updates from background sync.
  useEffect(() => {
    const offMail = onEvent<string>("mail:changed", () => {
      refreshAccounts();
      setReload((n) => n + 1);
    });
    const offSync = onEvent<SyncStatus>("sync:status", setSync);
    const offUpdate = onEvent<UpdateStatus>("update:status", setUpdate);
    // Clicking a new-mail notification opens that message in All inboxes.
    const offOpen = onEvent<string>("open:message", (id) => {
      setSettingsSection(null);
      setAccountId("");
      setRole("inbox" as FolderRole);
      setQuery("");
      pendingOpen.current = id;
      setReload((n) => n + 1);
    });
    return () => {
      offMail();
      offSync();
      offUpdate();
      offOpen();
    };
  }, [refreshAccounts]);

  // If the chosen account was removed, show all inboxes.
  const activeAccount = accountId && accounts?.some((a) => a.id === accountId) ? accountId : "";
  const activeFolder = folderId && folders.some((f) => f.id === folderId) ? folders.find((f) => f.id === folderId) : undefined;
  const viewKey = activeFolder ? activeFolder.id : `${activeAccount}:${role}`;
  const historyState = history.key === viewKey ? history : { key: viewKey, loading: false, done: false };
  const searchKey = `${viewKey}|${query.trim()}`;
  const searchState = serverSearch.key === searchKey ? serverSearch : { key: searchKey, busy: false, matched: null };

  useEffect(() => {
    let cancelled = false;
    const load = activeFolder ? api.listFolderMessages(activeFolder.id, query) : api.listMessages(activeAccount, role, query);
    load.then((list) => {
      if (cancelled) return;
      const open = pendingOpen.current ? list.find((m) => m.id === pendingOpen.current) : undefined;
      if (open) {
        // Opened from a notification: select it and mark it read.
        pendingOpen.current = null;
        if (open.unread) {
          open.unread = false;
          api.setRead(open.id, true).then(refreshAccounts, fail);
        }
        setMessages(list);
        setSelectedId(open.id);
        return;
      }
      setMessages(list);
      setSelectedId((cur) => (cur && list.some((m) => m.id === cur) ? cur : null));
    }, fail);
    return () => {
      cancelled = true;
    };
  }, [activeAccount, activeFolder, role, query, reload, fail, refreshAccounts]);

  // Folders outside background sync are refreshed when opened.
  const openedFolder = activeFolder?.id;
  useEffect(() => {
    if (!openedFolder) return;
    let cancelled = false;
    api.openFolder(openedFolder).then(
      () => !cancelled && setReload((n) => n + 1),
      (err) => !cancelled && fail(err),
    );
    return () => {
      cancelled = true;
    };
  }, [openedFolder, fail]);

  const loadOlder = useCallback(async () => {
    if (historyState.loading || historyState.done) return;
    const key = viewKey;
    setHistory({ key, loading: true, done: false });
    try {
      const res = await api.loadOlder(activeAccount, role, activeFolder?.id ?? "");
      setHistory({ key, loading: false, done: res.done });
      if (res.added > 0) setReload((n) => n + 1);
    } catch (err) {
      setHistory({ key, loading: false, done: false });
      fail(err);
    }
  }, [historyState.loading, historyState.done, viewKey, activeAccount, role, activeFolder, fail]);

  const searchOnServer = useCallback(async () => {
    const key = searchKey;
    setServerSearch({ key, busy: true, matched: null });
    try {
      const res = await api.searchServer(activeAccount, role, activeFolder?.id ?? "", query);
      setServerSearch({ key, busy: false, matched: res.matched });
      if (res.added > 0) setReload((n) => n + 1);
    } catch (err) {
      setServerSearch({ key, busy: false, matched: null });
      fail(err);
    }
  }, [searchKey, activeAccount, role, activeFolder, query, fail]);

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
                  const back = await api.moveToFolder(res.id, res.fromFolder);
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
  const moveToFolder = useCallback(
    async (m: MessageSummary, target: Folder) => {
      setMoving(null);
      const idx = messages.findIndex((x) => x.id === m.id);
      const next = messages[idx + 1] ?? messages[idx - 1] ?? null;
      setMessages((list) => list.filter((x) => x.id !== m.id));
      setSelectedId(next ? next.id : null);
      try {
        const res = await api.moveToFolder(m.id, target.id);
        refreshAccounts();
        setToast({
          text: `Moved to ${target.name}`,
          undo: res.id
            ? async () => {
                setToast(null);
                try {
                  const back = await api.moveToFolder(res.id, res.fromFolder);
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
        setReload((n) => n + 1);
        fail(err);
      }
    },
    [messages, refreshAccounts, fail],
  );
  const trash = useCallback((m: MessageSummary) => move(m, "trash" as FolderRole, "Moved to Trash"), [move]);

  const compose = useCallback(() => {
    const list = accounts ?? [];
    const real = list.filter((a) => !a.demo);
    const pool = real.length ? real : list;
    const from = pool.find((a) => a.id === activeAccount) ?? pool[0];
    if (!from) return;
    setComposer({
      title: "New message",
      draft: { accountId: from.id, to: [], cc: [], bcc: [], subject: "", body: "", replyToId: "", attachments: [] },
    });
  }, [accounts, activeAccount]);

  const respond = useCallback(
    async (m: MessageSummary, mode: "reply" | "replyAll" | "forward") => {
      try {
        const draft = await api.newDraft(m.id, mode);
        setComposer({ draft, title: mode === "forward" ? "Forward" : mode === "replyAll" ? "Reply all" : "Reply" });
      } catch (err) {
        fail(err);
      }
    },
    [fail],
  );

  /** Undo send: the message waits a few seconds before it really goes out. */
  const queueSend = useCallback(
    (draft: Draft, title: string) => {
      setComposer(null);
      if (sendTimer.current) clearTimeout(sendTimer.current);
      sendTimer.current = setTimeout(async () => {
        sendTimer.current = null;
        setToast({ text: "Sending…" });
        try {
          await api.send(draft);
          setToast({ text: "Message sent" });
          setReload((n) => n + 1);
        } catch (err) {
          setToast({ text: `Couldn't send: ${errorMessage(err)}`, tone: "error" });
          setComposer({ draft, title }); // give the draft back
        }
      }, UNDO_SEND_MS);
      setToast({
        text: "Sending…",
        undoLabel: "Undo",
        undo: () => {
          if (sendTimer.current) clearTimeout(sendTimer.current);
          sendTimer.current = null;
          setToast(null);
          setComposer({ draft, title });
        },
      });
    },
    [],
  );

  const dialogOpen = settingsSection !== null || addOpen || moving !== null;

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
      } else if (e.key === "c" && !composer) {
        e.preventDefault();
        compose();
      } else if (!selected) {
        return;
      } else if ((e.key === "r" || e.key === "a" || e.key === "f") && !composer) {
        e.preventDefault();
        respond(selected, e.key === "r" ? "reply" : e.key === "a" ? "replyAll" : "forward");
      } else if (e.key === "s") {
        toggleStar(selected);
      } else if (e.key === "u") {
        toggleRead(selected);
      } else if (e.key === "m") {
        e.preventDefault();
        setMoving(selected);
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
  }, [dialogOpen, messages, selectedId, selected, toast, select, toggleStar, toggleRead, archive, trash, composer, compose, respond]);

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
          update={update}
          onUpdateChange={setUpdate}
          onInstallUpdate={async () => {
            try {
              await api.installUpdate();
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
      {moving && (
        <MovePicker
          message={moving}
          folders={folders.filter((f) => f.accountId === moving.accountId)}
          onPick={(f) => moveToFolder(moving, f)}
          onClose={() => setMoving(null)}
        />
      )}
      {composer && accounts && (
        <Composer
          key={JSON.stringify(composer.draft).length + composer.title}
          state={composer}
          accounts={accounts}
          onSend={(d) => queueSend(d, composer.title)}
          onClose={() => setComposer(null)}
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
              {toast.undoLabel ?? "Undo"} {!toast.undoLabel && <span className="ml-1 opacity-60">Z</span>}
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
          update={update}
          onInstallUpdate={async () => {
            try {
              await api.installUpdate();
            } catch (err) {
              fail(err);
            }
          }}
          onSelectAccount={(id) => {
            setFolderId("");
            setAccountId(id);
            setSelectedId(null);
          }}
          folderId={activeFolder?.id ?? ""}
          onSelectFolder={(f) => {
            setFolderId(f.id);
            setAccountId(f.accountId);
            setQuery("");
            setSelectedId(null);
          }}
          onSelectRole={(r) => {
            setFolderId("");
            setRole(r);
            setSelectedId(null);
          }}
          onOpenSettings={(section) => setSettingsSection(section ?? "appearance")}
          onAddAccount={() => setAddOpen(true)}
          onSyncNow={() => api.syncNow()}
          onCompose={compose}
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
          folderName={activeFolder?.name}
          canLoadOlder={!(accounts ?? []).every((a) => a.demo)}
          olderLoading={historyState.loading}
          olderDone={historyState.done}
          onLoadOlder={loadOlder}
          serverSearchBusy={searchState.busy}
          serverSearchMatched={searchState.matched}
          onSearchServer={searchOnServer}
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
        onRespond={respond}
        onMove={(m) => setMoving(m)}
        onToast={(text, action, error) =>
          setToast({ text, tone: error ? "error" : undefined, undo: action?.run, undoLabel: action?.label })
        }
      />
      {dialogs}
    </div>
  );
}
