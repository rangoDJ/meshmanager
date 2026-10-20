import { Check, CheckSquare } from "lucide-react";
import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { VirtuosoGrid } from "react-virtuoso";
import { type FileFilters, type FileItem, filterBody, useFiles } from "../api/library";
import { type Selection, useRemoveFromCollection } from "../api/organize";
import { formatBytes, formatNumber } from "../lib/format";
import { FileDrawer } from "./FileDrawer";
import { SelectionBar } from "./SelectionBar";
import { Thumbnail } from "./Thumbnail";
import { TagDot } from "./tags";
import { ErrorText, Spinner } from "./ui";

// The viewers pull in three.js; load them only when a file is opened.
const FilePreview = lazy(() => import("../viewer/FilePreview"));

/** Files that open in a viewer rather than only downloading. */
export function previewable(f: Pick<FileItem, "viewable" | "inline" | "format">) {
  return f.viewable || f.inline || f.format === "gcode";
}

/** The open viewer lives in the URL (?view=<id>), so back and reload work. */
export function useViewerParam() {
  const [params, setParams] = useSearchParams();
  const id = Number(params.get("view")) || null;
  // Stepping between files replaces the entry, so Back closes the viewer in one press.
  const set = (next: number | null, stepping = false) =>
    setParams(
      (p) => {
        const n = new URLSearchParams(p);
        if (next) n.set("view", String(next));
        else n.delete("view");
        return n;
      },
      { replace: next === null || stepping },
    );
  return [id, set] as const;
}

/**
 * A filtered, scrolling grid of files with its file panel, viewer and multi-select.
 * The library and every collection use it.
 */
export function FileResults({
  filters,
  tile,
  scope,
  lead,
  controls,
  inCollection,
  empty = "No files match these filters.",
}: {
  filters: FileFilters;
  tile: number;
  /** Words after "N files", such as "in Downloads / Toys". */
  scope?: React.ReactNode;
  /** Shown above the count, such as folder cards. */
  lead?: React.ReactNode;
  /** Extra controls on the count row, such as a sort menu. */
  controls?: React.ReactNode;
  /** Set when showing a hand-filled collection, so the selection can be taken out of it. */
  inCollection?: { id: number; name: string };
  empty?: React.ReactNode;
}) {
  const takeOut = useRemoveFromCollection();
  const [viewing, setViewing] = useViewerParam();
  const [openId, setOpenId] = useState<number | null>(null);
  const files = useFiles(filters);
  const items = useMemo(() => files.data?.pages.flatMap((p) => p.items) ?? [], [files.data]);
  const total = files.data?.pages[0]?.total;

  const [selecting, setSelecting] = useState(false);
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [allMatching, setAllMatching] = useState(false);
  const anchor = useRef<number | null>(null);

  // A different filter is a different set of files: start the selection over.
  const filterKey = JSON.stringify(filters);
  useEffect(() => {
    setPicked(new Set());
    setAllMatching(false);
    anchor.current = null;
  }, [filterKey]);

  function stopSelecting() {
    setSelecting(false);
    setPicked(new Set());
    setAllMatching(false);
    anchor.current = null;
  }

  useEffect(() => {
    if (!selecting) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && openId === null && viewing === null) stopSelecting();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selecting, openId, viewing]);

  function toggle(index: number, range: boolean) {
    const id = items[index].id;
    setAllMatching(false);
    setPicked((prev) => {
      const next = new Set(prev);
      if (range && anchor.current !== null) {
        const [a, b] = [Math.min(anchor.current, index), Math.max(anchor.current, index)];
        for (let i = a; i <= b; i++) next.add(items[i].id);
      } else if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
    anchor.current = index;
  }

  const selection: Selection = allMatching ? { filter: filterBody(filters) } : { ids: [...picked] };
  const selectedCount = allMatching ? (total ?? picked.size) : picked.size;

  return (
    <div className="space-y-4">
      {lead}
      <div className="flex items-center gap-3">
        <p className="text-sm text-muted">
          {total !== undefined ? `${formatNumber(total)} ${total === 1 ? "file" : "files"}` : " "}
          {scope}
        </p>
        <div className="ml-auto flex items-center gap-2">
          {controls}
          {items.length > 0 && (
          <button
            onClick={() => (selecting ? stopSelecting() : setSelecting(true))}
            aria-pressed={selecting}
            className={`inline-flex h-9 items-center gap-1.5 rounded-lg border px-3 text-sm transition focus-visible:outline-2 focus-visible:outline-accent ${
              selecting ? "border-accent bg-accent/10 text-accent" : "border-border bg-surface text-muted hover:text-fg"
            }`}
          >
            <CheckSquare className="size-4" aria-hidden />
            {selecting ? "Done" : "Select"}
          </button>
          )}
        </div>
      </div>

      <ErrorText>{files.error?.message}</ErrorText>
      {files.isPending ? (
        <Spinner />
      ) : items.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-6 py-16 text-center text-sm text-muted">{empty}</div>
      ) : (
        <div style={{ "--tile": `${tile}px` } as React.CSSProperties} className={selectedCount > 0 ? "pb-24" : ""}>
          <VirtuosoGrid
            useWindowScroll
            data={items}
            increaseViewportBy={600}
            listClassName="grid gap-3 grid-cols-[repeat(auto-fill,minmax(var(--tile),1fr))]"
            endReached={() => files.hasNextPage && !files.isFetchingNextPage && files.fetchNextPage()}
            itemContent={(index, file) => (
              <Tile
                file={file}
                selecting={selecting}
                selected={allMatching || picked.has(file.id)}
                onOpen={(e) => (selecting ? toggle(index, e.shiftKey) : setOpenId(file.id))}
                onView={!selecting && previewable(file) ? () => setViewing(file.id) : undefined}
              />
            )}
          />
          {files.isFetchingNextPage && (
            <div className="flex justify-center py-6">
              <Spinner />
            </div>
          )}
        </div>
      )}

      {selecting && selectedCount > 0 && (
        <SelectionBar
          count={selectedCount}
          total={total}
          allMatching={allMatching}
          selection={selection}
          onSelectAll={() => setAllMatching(true)}
          onClear={stopSelecting}
          onTakeOut={
            inCollection && {
              label: `Take out of ${inCollection.name}`,
              pending: takeOut.isPending,
              run: () => takeOut.mutate({ id: inCollection.id, selection }, { onSuccess: stopSelecting }),
            }
          }
        />
      )}
      <FileOverlays openId={openId} onClose={() => setOpenId(null)} />
    </div>
  );
}

/** A file's side panel and, when one is open in the URL, its viewer. */
export function FileOverlays({ openId, onClose }: { openId: number | null; onClose: () => void }) {
  const [viewing, setViewing] = useViewerParam();
  return (
    <>
      {openId !== null && <FileDrawer id={openId} onClose={onClose} onView={(id) => setViewing(id)} />}
      {viewing !== null && (
        <Suspense
          fallback={
            <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg">
              <Spinner />
            </div>
          }
        >
          <FilePreview fileId={viewing} onOpen={(id) => setViewing(id, true)} onClose={() => setViewing(null)} />
        </Suspense>
      )}
    </>
  );
}

function Tile({
  file,
  selecting,
  selected,
  onOpen,
  onView,
}: {
  file: FileItem;
  selecting: boolean;
  selected: boolean;
  onOpen: (e: React.MouseEvent) => void;
  onView?: () => void;
}) {
  const pinned = file.tags.filter((t) => t.pinned).slice(0, 5);
  return (
    <button
      onClick={onOpen}
      onDoubleClick={onView}
      aria-pressed={selecting ? selected : undefined}
      className="group block w-full text-left"
      title={onView ? `${file.rel_path}\nDouble-click to open` : file.rel_path}
    >
      <div
        className={`relative aspect-square overflow-hidden rounded-xl border bg-surface p-2 transition group-focus-visible:ring-2 group-focus-visible:ring-accent ${
          selected ? "border-accent ring-2 ring-accent/40" : "border-border group-hover:border-accent/60"
        }`}
      >
        <Thumbnail file={file} className="transition group-hover:scale-[1.03]" />
        {selecting && (
          <span
            aria-hidden
            className={`absolute top-2 left-2 grid size-6 place-items-center rounded-md border ${
              selected ? "border-accent bg-accent text-accent-fg" : "border-border bg-surface/90 text-transparent"
            }`}
          >
            <Check className="size-4" />
          </span>
        )}
        {pinned.length > 0 && (
          <span
            role="img"
            aria-label={`Tags: ${pinned.map((t) => t.name).join(", ")}`}
            title={pinned.map((t) => t.name).join(", ")}
            className="absolute bottom-2 left-2 flex items-center gap-1 rounded-full border border-border bg-surface/90 px-1.5 py-1 backdrop-blur"
          >
            {pinned.map((t) => (
              <TagDot key={t.id} color={t.color} />
            ))}
          </span>
        )}
      </div>
      <p className="mt-1.5 truncate text-sm">{file.name}</p>
      <p className="truncate text-xs text-muted">
        <span className="uppercase">{file.format}</span> · {formatBytes(file.size)}
        {file.archive_id !== null && (
          <span className="ml-1.5 rounded bg-surface-2 px-1 py-px text-[10px] font-medium" title="Inside an archive">
            ZIP
          </span>
        )}
      </p>
    </button>
  );
}
