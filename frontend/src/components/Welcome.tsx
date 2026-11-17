"use client";

import { useState } from "react";
import { ImageOff, KeyRound, Loader2, ShieldCheck } from "lucide-react";

type Props = {
  onAddAccount: () => void;
  onTrySamples: () => Promise<void>;
};

/** First-run screen, shown when no accounts are linked. */
export default function Welcome({ onAddAccount, onTrySamples }: Props) {
  const [loading, setLoading] = useState(false);
  return (
    <main className="flex h-full flex-col items-center justify-center px-6 text-center">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img src="/pigeon.png" alt="" className="h-16 w-16" draggable={false} />
      <h1 className="display-heading mt-5 font-display text-[22px] font-semibold tracking-[-0.02em]">Welcome to Pigeon</h1>
      <p className="mt-2 max-w-[380px] text-[13.5px] leading-relaxed text-muted">
        All your email accounts in one fast, private inbox. Add your first account to get started.
      </p>

      <div className="mt-7 flex flex-col items-center gap-2.5">
        <button
          onClick={onAddAccount}
          className="h-9 rounded-md bg-primary px-5 text-[13.5px] font-medium text-primary-foreground transition-colors duration-150 hover:bg-primary-hover"
        >
          Add an account
        </button>
        <button
          onClick={async () => {
            setLoading(true);
            try {
              await onTrySamples();
            } finally {
              setLoading(false);
            }
          }}
          disabled={loading}
          className="flex h-8 items-center gap-2 rounded-md px-3 text-[13px] text-muted transition-colors duration-150 hover:bg-hover hover:text-text"
        >
          {loading && <Loader2 size={14} className="animate-spin" aria-hidden />}
          Explore with sample mail
        </button>
      </div>

      <ul className="mt-12 grid max-w-[560px] grid-cols-1 gap-4 text-left text-[12.5px] text-subtle sm:grid-cols-3">
        <Point icon={ShieldCheck} title="Safe by default">
          Scripts in emails never run.
        </Point>
        <Point icon={ImageOff} title="No tracking pixels">
          Remote images wait for your OK.
        </Point>
        <Point icon={KeyRound} title="Secrets stay local">
          Passwords live in Windows Credential Manager.
        </Point>
      </ul>
    </main>
  );
}

function Point(props: { icon: React.ComponentType<{ size?: number; className?: string }>; title: string; children: React.ReactNode }) {
  const Icon = props.icon;
  return (
    <li className="flex gap-2.5">
      <Icon size={16} className="mt-px shrink-0 text-muted" />
      <span>
        <span className="block font-medium text-text">{props.title}</span>
        {props.children}
      </span>
    </li>
  );
}
