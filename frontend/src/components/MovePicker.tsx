"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Archive, Folder as FolderIcon, Inbox, type LucideIcon, Search, ShieldAlert, Trash2 } from "lucide-react";
import type { Folder, MessageSummary } from "@/lib/api";
import { Input } from "@/components/ui/input";

type Props = {
  message: MessageSummary;
  folders: Folder[]; // the message's account
  onPick: (f: Folder) => void;
  onClose: () => void;
};

const roleOrder: Record<string, number> = { inbox: 0, archive: 1, spam: 2, trash: 3 };
const roleIcon: Record<string, LucideIcon> = { inbox: Inbox, archive: Archive, spam: ShieldAlert, trash: Trash2 };

/** Folders a message can be moved to: standard ones first, then the user's own. */
export function movableFolders(folders: Folder[], currentFolderId: string): Folder[] {
  return folders
    .filter((f) => f.id !== currentFolderId)
    .filter((f) => f.role in roleOrder || (f.role === "custom" && !f.path.startsWith("[Gmail]")))
    .sort((a, b) => {
      const ra = roleOrder[a.role] ?? 9;
      const rb = roleOrder[b.role] ?? 9;
      return ra !== rb ? ra - rb : a.path.localeCompare(b.path);
    });
}

export default function MovePicker({ message, folders, onPick, onClose }: Props) {
  const [filter, setFilter] = useState("");
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  const options = useMemo(() => {
    const all = movableFolders(folders, message.folderId);
    const q = filter.trim().toLowerCase();
    return q ? all.filter((f) => f.path.toLowerCase().includes(q) || f.name.toLowerCase().includes(q)) : all;
  }, [folders, message.folderId, filter]);
  const current = Math.min(active, Math.max(options.length - 1, 0));

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/35 pt-[14vh] [animation:fade-in_120ms_ease-out]"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        role="dialog"
        aria-label="Move to folder"
        className="w-[min(420px,90vw)] overflow-hidden rounded-xl border border-border bg-surface shadow-2xl [animation:dialog-in_140ms_cubic-bezier(0.16,1,0.3,1)]"
      >
        <div className="flex items-center gap-2 border-b border-border px-3.5">
          <Search size={15} className="shrink-0 text-subtle" aria-hidden />
          <Input
            ref={inputRef}
            value={filter}
            onChange={(e) => {
              setFilter(e.target.value);
              setActive(0);
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                e.preventDefault();
                const d = e.key === "ArrowDown" ? 1 : -1;
                setActive((current + d + options.length) % Math.max(options.length, 1));
              } else if (e.key === "Enter" && options[current]) {
                e.preventDefault();
                onPick(options[current]);
              } else if (e.key === "Escape") {
                e.preventDefault();
                e.stopPropagation();
                onClose();
              }
            }}
            placeholder="Move to…"
            aria-label="Filter folders"
            aria-controls="move-options"
            aria-activedescendant={options[current] ? `move-${current}` : undefined}
            className="h-11 flex-1 border-0 bg-transparent px-0 shadow-none focus-visible:ring-0 dark:bg-transparent rounded-none text-[13.5px] md:text-[13.5px]"
          />
          <kbd className="rounded border border-border px-1.5 text-[11px] text-subtle">Esc</kbd>
        </div>
        <ul id="move-options" role="listbox" className="max-h-[320px] overflow-y-auto py-1">
          {options.length === 0 && <li className="px-4 py-3 text-[13px] text-subtle">No matching folders</li>}
          {options.map((f, i) => {
            const Icon = roleIcon[f.role] ?? FolderIcon;
            const parent = f.path.includes("/") ? f.path.slice(0, f.path.lastIndexOf("/")) : "";
            return (
              <li
                key={f.id}
                id={`move-${i}`}
                role="option"
                aria-selected={i === current}
                onMouseEnter={() => setActive(i)}
                onMouseDown={(e) => {
                  e.preventDefault();
                  onPick(f);
                }}
                className={`flex items-center gap-2.5 px-4 py-2 text-[13px] ${i === current ? "bg-selected" : ""}`}
              >
                <Icon size={15} strokeWidth={1.75} className="shrink-0 text-muted" aria-hidden />
                <span className="truncate">{f.name}</span>
                {parent && <span className="truncate text-[12px] text-subtle">{parent}</span>}
              </li>
            );
          })}
        </ul>
      </div>
    </div>
  );
}
