import { ChevronLeft, ChevronRight, X } from "lucide-react";
import { useEffect, type ReactNode } from "react";
import type { ViewPreset } from "./stage";

export const views: [ViewPreset, string][] = [
  ["iso", "Iso"],
  ["front", "Front"],
  ["back", "Back"],
  ["left", "Left"],
  ["right", "Right"],
  ["top", "Top"],
  ["bottom", "Bottom"],
];

export const backgrounds: [string, string][] = [
  ["#16161b", "Dark"],
  ["#2b2d33", "Graphite"],
  ["#8a8f99", "Studio grey"],
  ["#f2f2f5", "Light"],
];

/** Stepping through the previewable files of the folder the open file sits in. */
export type FileNavigation = {
  index: number;
  count: number;
  prev?: () => void;
  next?: () => void;
};

export function ToolButton({
  active,
  onClick,
  label,
  children,
}: {
  active?: boolean;
  onClick: () => void;
  label: string;
  children: ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      title={label}
      aria-label={label}
      aria-pressed={active}
      className={`px-2.5 py-1.5 transition-colors focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent ${
        active ? "bg-accent text-accent-fg" : "text-muted hover:text-fg"
      }`}
    >
      {children}
    </button>
  );
}

/** Previous / position / next, and the close button: the top-right corner of every viewer. */
export function ViewerCorner({ nav, onClose }: { nav?: FileNavigation; onClose: () => void }) {
  return (
    <div className="absolute top-3 right-3 z-10 flex items-center gap-1.5">
      {nav && nav.count > 1 && (
        <div className="flex items-center overflow-hidden rounded-lg border border-border bg-surface/90 backdrop-blur">
          <button
            onClick={nav.prev}
            disabled={!nav.prev}
            className="p-2 text-muted transition-colors hover:text-fg disabled:opacity-30 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent"
            aria-label="Previous file in this folder"
            title="Previous file (←)"
          >
            <ChevronLeft className="size-4" />
          </button>
          <span className="px-1 text-xs text-muted tabular-nums" aria-live="polite">
            {nav.index + 1} of {nav.count}
          </span>
          <button
            onClick={nav.next}
            disabled={!nav.next}
            className="p-2 text-muted transition-colors hover:text-fg disabled:opacity-30 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent"
            aria-label="Next file in this folder"
            title="Next file (→)"
          >
            <ChevronRight className="size-4" />
          </button>
        </div>
      )}
      <button
        onClick={onClose}
        className="rounded-lg border border-border bg-surface/90 p-2 text-muted backdrop-blur transition-colors hover:text-fg focus-visible:outline-2 focus-visible:outline-accent"
        aria-label="Close"
        title="Close (Esc)"
      >
        <X className="size-5" />
      </button>
    </div>
  );
}

/** Escape closes; ← and → step through the folder (unless a control has the keys). */
export function useViewerKeys(onClose: () => void, nav?: FileNavigation) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      const typing = t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable);
      if (e.key === "Escape") onClose();
      else if (!typing && e.key === "ArrowLeft" && nav?.prev) nav.prev();
      else if (!typing && e.key === "ArrowRight" && nav?.next) nav.next();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, nav]);
}

/** The right-hand (desktop) or bottom (phone) panel every viewer shares. */
export function InfoPanel({ children }: { children: ReactNode }) {
  return (
    <aside className="max-h-[42vh] shrink-0 overflow-y-auto border-t border-border bg-surface p-4 lg:max-h-none lg:w-80 lg:border-t-0 lg:border-l">
      {children}
    </aside>
  );
}

/** Loading a viewer's content: a quiet progress line in the content area. */
export function LoadingContent({ progress, label }: { progress?: number; label: string }) {
  return (
    <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-3">
      <div className="h-1.5 w-48 overflow-hidden rounded-full bg-surface-2">
        <div
          className={`h-full rounded-full bg-accent transition-[width] duration-200 ${progress === undefined ? "w-1/3 animate-pulse" : ""}`}
          style={progress === undefined ? undefined : { width: `${Math.max(progress * 100, 4)}%` }}
        />
      </div>
      <p className="text-xs text-muted">{label}</p>
    </div>
  );
}

export function formatDuration(seconds: number) {
  if (!seconds) return "";
  const h = Math.floor(seconds / 3600);
  const m = Math.round((seconds % 3600) / 60);
  return h ? `${h} h ${m} min` : `${m} min`;
}
