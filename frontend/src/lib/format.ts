const timeFmt = new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" });
const dayFmt = new Intl.DateTimeFormat(undefined, { weekday: "short" });
const dateFmt = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" });
const fullFmt = new Intl.DateTimeFormat(undefined, {
  weekday: "long",
  month: "long",
  day: "numeric",
  year: "numeric",
  hour: "numeric",
  minute: "2-digit",
});

/** Compact list date: 3:42 PM today, "Tue" this week, "Nov 2" otherwise. */
export function shortDate(iso: string): string {
  const d = new Date(iso);
  const now = new Date();
  if (d.toDateString() === now.toDateString()) return timeFmt.format(d);
  const days = (now.getTime() - d.getTime()) / 86_400_000;
  if (days < 6) return dayFmt.format(d);
  return dateFmt.format(d);
}

export function fullDate(iso: string): string {
  return fullFmt.format(new Date(iso));
}

export function initials(name: string, email: string): string {
  const src = name.trim() || email;
  const parts = src.split(/\s+/).filter(Boolean);
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
  return src.slice(0, 2).toUpperCase();
}
