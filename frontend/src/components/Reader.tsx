"use client";

import { useEffect, useState } from "react";
import { Archive, Forward, ImageOff, Mail, MailOpen, Reply, ReplyAll, Star, Trash2 } from "lucide-react";
import { api, errorMessage, type Account, type MessageBody as Body, type MessageSummary } from "@/lib/api";
import { fullDate, initials } from "@/lib/format";
import MessageBody from "./MessageBody";

type Props = {
  message: MessageSummary | null;
  account?: Account;
  onToggleStar: (m: MessageSummary) => void;
  onToggleRead: (m: MessageSummary) => void;
  onArchive: (m: MessageSummary) => void;
  onTrash: (m: MessageSummary) => void;
};

export default function Reader(props: Props) {
  if (!props.message) return <EmptyReader />;
  // Keyed by message so body and image-permission state reset per message.
  return <MessageView key={props.message.id} {...props} message={props.message} />;
}

function MessageView({
  message,
  account,
  onToggleStar,
  onToggleRead,
  onArchive,
  onTrash,
}: Props & { message: MessageSummary }) {
  const [body, setBody] = useState<Body | null>(null);
  const [allowRemote, setAllowRemote] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    api.getMessage(message.id).then(
      (b) => {
        if (cancelled) return;
        setBody(b);
        setLoadError(null);
      },
      (err) => !cancelled && setLoadError(errorMessage(err)),
    );
    return () => {
      cancelled = true;
    };
  }, [message.id, attempt]);

  return (
    <main className="flex min-w-[340px] flex-1 flex-col bg-surface">
      <div className="flex h-[52px] shrink-0 items-center gap-0.5 border-b border-border px-3" role="toolbar" aria-label="Message actions">
        <ToolButton icon={Archive} label="Archive" shortcut="E" onClick={() => onArchive(message)} />
        <ToolButton icon={Trash2} label="Delete" shortcut="#" onClick={() => onTrash(message)} />
        <Divider />
        <ToolButton
          icon={message.unread ? MailOpen : Mail}
          label={message.unread ? "Mark as read" : "Mark as unread"}
          shortcut="U"
          onClick={() => onToggleRead(message)}
        />
        <ToolButton
          icon={Star}
          label={message.starred ? "Unstar" : "Star"}
          shortcut="S"
          pressed={message.starred}
          onClick={() => onToggleStar(message)}
        />
        <div className="flex-1" />
        <ToolButton icon={Reply} label="Reply" shortcut="R" text="Reply" />
        <ToolButton icon={ReplyAll} label="Reply all" shortcut="A" />
        <ToolButton icon={Forward} label="Forward" shortcut="F" />
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <article className="mx-auto max-w-[820px] px-8 pt-6 pb-10">
          <h2 className="selectable text-[20px] leading-[1.3] font-semibold tracking-[-0.015em] text-balance">
            {message.subject || "(no subject)"}
          </h2>

          <div className="mt-4 flex items-start gap-3">
            <div
              className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-active text-[12px] font-semibold text-muted"
              aria-hidden
            >
              {initials(message.from.name, message.from.email)}
            </div>
            <div className="selectable min-w-0 flex-1 leading-tight">
              <div className="flex items-baseline gap-2">
                <span className="truncate text-[13.5px] font-semibold">{message.from.name}</span>
                <span className="truncate text-[12.5px] text-subtle">{message.from.email}</span>
              </div>
              <div className="mt-1 truncate text-[12.5px] text-muted">
                To {(message.to ?? []).map((t) => t.name || t.email).join(", ")}
                {account && (
                  <span className="text-subtle">
                    {" "}
                    · via{" "}
                    <span className="inline-flex items-center gap-1">
                      <span className="h-1.5 w-1.5 rounded-[2px]" style={{ background: account.color }} aria-hidden />
                      {account.name}
                    </span>
                  </span>
                )}
              </div>
            </div>
            <time dateTime={message.date} className="shrink-0 pt-0.5 text-[12px] text-subtle tabular-nums">
              {fullDate(message.date)}
            </time>
          </div>

          {body && body.remoteImages > 0 && !allowRemote && (
            <div className="mt-5 flex items-center gap-2.5 rounded-md bg-bg px-3 py-2 text-[12.5px] text-muted">
              <ImageOff size={15} strokeWidth={1.75} className="shrink-0" aria-hidden />
              <span className="flex-1">
                {body.remoteImages} remote {body.remoteImages === 1 ? "image" : "images"} blocked to stop the sender
                tracking when you open this email.
              </span>
              <button
                onClick={() => setAllowRemote(true)}
                className="shrink-0 rounded px-2 py-1 font-medium text-primary transition-colors duration-150 hover:bg-primary-soft"
              >
                Show images
              </button>
            </div>
          )}

          <div className="mt-5">
            {body ? (
              <MessageBody html={body.html} allowRemote={allowRemote} onLink={(url) => api.openExternal(url)} />
            ) : loadError ? (
              <div role="alert" className="rounded-lg border border-border px-4 py-6 text-center text-[13px]">
                <p className="text-muted">{loadError}</p>
                <button
                  onClick={() => {
                    setLoadError(null);
                    setAttempt((n) => n + 1);
                  }}
                  className="mt-3 rounded-md border border-border px-3 py-1.5 text-[12.5px] font-medium transition-colors hover:bg-hover"
                >
                  Try again
                </button>
              </div>
            ) : (
              <div className="h-40 animate-pulse rounded-lg bg-bg" />
            )}
          </div>
        </article>
      </div>
    </main>
  );
}

function EmptyReader() {
  return (
    <main className="flex min-w-[340px] flex-1 flex-col items-center justify-center bg-surface px-6 text-center">
      <p className="text-[13.5px] font-medium text-muted">No message selected</p>
      <div className="mt-3 flex flex-wrap justify-center gap-x-4 gap-y-1 text-[12px] text-subtle">
        <Hint k="J / K">navigate</Hint>
        <Hint k="E">archive</Hint>
        <Hint k="S">star</Hint>
        <Hint k="/">search</Hint>
      </div>
    </main>
  );
}

function Hint({ k, children }: { k: string; children: React.ReactNode }) {
  return (
    <span>
      <kbd className="mr-1.5 rounded border border-border px-1 py-px font-sans text-[11px] text-muted">{k}</kbd>
      {children}
    </span>
  );
}

function Divider() {
  return <span className="mx-1.5 h-4 w-px bg-border" aria-hidden />;
}

function ToolButton(props: {
  icon: React.ComponentType<{ size?: number; fill?: string; strokeWidth?: number; "aria-hidden"?: boolean }>;
  label: string;
  shortcut?: string;
  text?: string;
  pressed?: boolean;
  onClick?: () => void;
}) {
  const Icon = props.icon;
  const tip = props.shortcut ? `${props.label} (${props.shortcut})` : props.label;
  return (
    <button
      onClick={props.onClick}
      title={tip}
      aria-label={props.text ? undefined : props.label}
      aria-pressed={props.pressed}
      className={`flex h-8 items-center gap-1.5 rounded-md text-[13px] transition-colors duration-150 hover:bg-hover ${
        props.text ? "px-2.5" : "w-8 justify-center"
      } ${props.pressed ? "text-star" : "text-muted hover:text-text"}`}
    >
      <Icon size={16} strokeWidth={1.75} fill={props.pressed ? "currentColor" : "none"} aria-hidden />
      {props.text}
    </button>
  );
}
