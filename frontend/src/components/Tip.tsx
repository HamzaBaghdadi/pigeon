"use client";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

/**
 * A shadcn Tooltip around one element. The child must be a single element
 * that accepts a ref (a button, usually); its accessible name stays on the
 * element itself (aria-label), the tooltip is a visual hint.
 */
export function Tip({
  label,
  shortcut,
  side = "bottom",
  children,
}: {
  label: string;
  shortcut?: string;
  side?: "top" | "bottom" | "left" | "right";
  children: React.ReactElement;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side={side} sideOffset={6} className="flex items-center gap-2">
        {label}
        {shortcut && <kbd className="rounded bg-white/15 px-1 font-sans text-[10.5px] opacity-80">{shortcut}</kbd>}
      </TooltipContent>
    </Tooltip>
  );
}
