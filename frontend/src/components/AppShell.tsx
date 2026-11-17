"use client";

import { TooltipProvider } from "@/components/ui/tooltip";
import Ambience from "./Ambience";
import TitleBar from "./TitleBar";
import Mailbox from "./Mailbox";

/**
 * The window: Pigeon's own title bar above the app. The body paints the
 * theme's background; Ambience animates over it, behind the panes.
 */
export default function AppShell() {
  return (
    <TooltipProvider delayDuration={400} skipDelayDuration={200}>
      <Ambience />
      <div className="relative z-[1] flex h-screen flex-col">
        <TitleBar />
        <div className="relative min-h-0 flex-1">
          <Mailbox />
        </div>
      </div>
    </TooltipProvider>
  );
}
