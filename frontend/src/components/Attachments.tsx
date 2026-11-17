"use client";

import { useState } from "react";
import { Download, ExternalLink, File, FileArchive, FileImage, FileSpreadsheet, FileText, Loader2, ShieldAlert } from "lucide-react";
import { Tip } from "./Tip";
import { api, errorMessage, type Attachment } from "@/lib/api";

// Mirrors the Go list; the backend enforces it, this only adjusts the UI.
const DANGEROUS =
  /\.(exe|com|scr|pif|bat|cmd|ps1|psm1|ps1xml|psc1|vbs|vbe|js|jse|wsf|wsh|hta|msi|msp|msix|msixbundle|appx|appxbundle|jar|lnk|reg|cpl|inf|scf|url|iso|img|vhd|vhdx|dll|sys|chm|application|gadget|settingcontent-ms)[.\s]*$/i;

function iconFor(a: Attachment) {
  const ct = a.contentType;
  const name = a.filename.toLowerCase();
  if (ct.startsWith("image/")) return FileImage;
  if (/zip|compressed|tar|rar|7z/.test(ct) || /\.(zip|rar|7z|tar|gz)$/.test(name)) return FileArchive;
  if (/sheet|excel|csv/.test(ct) || /\.(xlsx?|csv|ods)$/.test(name)) return FileSpreadsheet;
  if (ct.startsWith("text/") || /pdf|word|document|presentation/.test(ct) || /\.(pdf|docx?|pptx?|txt|md|rtf)$/.test(name)) return FileText;
  return File;
}

export function formatSize(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

type Props = {
  messageId: string;
  attachments: Attachment[];
  onToast: (text: string, action?: { label: string; run: () => void }, error?: boolean) => void;
};

export default function Attachments({ messageId, attachments, onToast }: Props) {
  const [busy, setBusy] = useState<string | null>(null); // "open:2" / "save:2"

  async function run(kind: "open" | "save", a: Attachment) {
    setBusy(`${kind}:${a.index}`);
    try {
      if (kind === "open") {
        await api.openAttachment(messageId, a.index);
      } else {
        const path = await api.saveAttachment(messageId, a.index);
        if (path) {
          onToast(`Saved ${a.filename || "attachment"}`, { label: "Show in folder", run: () => api.showInFolder(path) });
        }
      }
    } catch (err) {
      onToast(errorMessage(err), undefined, true);
    } finally {
      setBusy(null);
    }
  }

  if (attachments.length === 0) return null;
  const total = attachments.reduce((n, a) => n + a.size, 0);

  return (
    <section className="mt-5" aria-label="Attachments">
      <div className="mb-2 text-[12px] text-subtle">
        {attachments.length} {attachments.length === 1 ? "attachment" : "attachments"} · {formatSize(total)}
      </div>
      <ul className="flex flex-wrap gap-2">
        {attachments.map((a) => {
          const Icon = iconFor(a);
          const name = a.filename || `Attachment ${a.index + 1}`;
          const risky = DANGEROUS.test(name);
          return (
            <li
              key={a.index}
              className="group flex w-[240px] items-center gap-2.5 rounded-lg border border-border bg-surface px-3 py-2 transition-colors hover:border-border-strong"
            >
              {risky ? (
                <ShieldAlert size={18} strokeWidth={1.75} className="shrink-0 text-star" aria-label="Potentially unsafe file type" />
              ) : (
                <Icon size={18} strokeWidth={1.75} className="shrink-0 text-muted" aria-hidden />
              )}
              <div className="min-w-0 flex-1">
                <Tip label={name} side="top">
                  <div className="truncate text-[12.5px] font-medium">{name}</div>
                </Tip>
                <div className="text-[11.5px] text-subtle">{risky ? "Can run code · save to scan" : formatSize(a.size)}</div>
              </div>
              <div className="flex shrink-0 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
                {!risky && (
                  <IconButton label={`Open ${name}`} busy={busy === `open:${a.index}`} onClick={() => run("open", a)}>
                    <ExternalLink size={14} strokeWidth={1.75} />
                  </IconButton>
                )}
                <IconButton label={`Save ${name}`} busy={busy === `save:${a.index}`} onClick={() => run("save", a)}>
                  <Download size={14} strokeWidth={1.75} />
                </IconButton>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function IconButton(props: { label: string; busy: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <Tip label={props.label} side="top">
    <button
      onClick={props.onClick}
      disabled={props.busy}
      aria-label={props.label}
      className="rounded-md p-1.5 text-muted transition-colors hover:bg-hover hover:text-text disabled:opacity-60"
    >
      {props.busy ? <Loader2 size={14} className="animate-spin" /> : props.children}
    </button>
    </Tip>
  );
}
