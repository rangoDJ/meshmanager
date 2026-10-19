import { FolderTree, Search, X } from "lucide-react";
import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { VirtuosoGrid } from "react-virtuoso";
import { type FileFilters, type FileItem, useFacets, useFiles, useSources } from "../api/library";
import { FileDrawer } from "../components/FileDrawer";
import { FolderPanel } from "../components/FolderPanel";
import { Thumbnail } from "../components/Thumbnail";
import { ErrorText, Spinner } from "../components/ui";
import { formatBytes, formatNumber } from "../lib/format";
import { useDebounced, useStoredState } from "../lib/hooks";

// The viewers pull in three.js; load them only when a file is opened.
const FilePreview = lazy(() => import("../viewer/FilePreview"));

/** Files that open in a viewer rather than only downloading. */
function previewable(f: Pick<FileItem, "viewable" | "inline" | "format">) {
  return f.viewable || f.inline || f.format === "gcode";
}

const kindLabels: Record<string, string> = {
  model: "Models",
  archive: "Archives",
  image: "Images",
  document: "Documents",
  laser: "Laser & CNC",
  gcode: "G-code",
  video: "Videos",
};

/** Filters live in the URL so reloads, links and the back button keep them. */
function useFilters() {
  const [params, setParams] = useSearchParams();
  const kindParam = params.getAll("kind");
  const filters: FileFilters = {
    source: Number(params.get("source")) || undefined,
    dir: params.get("dir") ?? "",
    // No kind in the URL means models; "all" means every kind.
    kinds: kindParam.length === 0 ? ["model"] : kindParam.includes("all") ? [] : kindParam,
    formats: params.getAll("format"),
    q: params.get("q") ?? "",
    sort: params.get("sort") ?? "-mtime",
  };

  function update(patch: Partial<FileFilters>, replace = false) {
    const next = { ...filters, ...patch };
    const p = new URLSearchParams();
    if (next.source) p.set("source", String(next.source));
    if (next.source && next.dir) p.set("dir", next.dir);
    if (next.kinds.length === 0) p.set("kind", "all");
    else if (!(next.kinds.length === 1 && next.kinds[0] === "model")) next.kinds.forEach((k) => p.append("kind", k));
    next.formats.forEach((f) => p.append("format", f));
    if (next.q) p.set("q", next.q);
    if (next.sort !== "-mtime") p.set("sort", next.sort);
    setParams(p, { replace });
  }
  return [filters, update] as const;
}

/** The open viewer lives in the URL (?view=<id>), so back and reload work. */
function useViewerParam() {
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

export function LibraryPage() {
  const [filters, setFilters] = useFilters();
  const [viewing, setViewing] = useViewerParam();
  const [search, setSearch] = useState(filters.q);
  const debounced = useDebounced(search, 250);
  const [tile, setTile] = useStoredState("mm.tileSize", 180);
  const [selected, setSelected] = useState<number | null>(null);
  const [foldersOpen, setFoldersOpen] = useState(false);

  useEffect(() => {
    if (debounced !== filters.q) setFilters({ q: debounced }, true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced]);

  const sources = useSources();
  const files = useFiles(filters);
  const facets = useFacets(filters);
  const items = useMemo(() => files.data?.pages.flatMap((p) => p.items) ?? [], [files.data]);
  const total = files.data?.pages[0]?.total;

  const kindCounts = useMemo(() => {
    const m = new Map<string, number>();
    facets.data?.forEach((f) => m.set(f.kind, (m.get(f.kind) ?? 0) + f.count));
    return m;
  }, [facets.data]);
  const allCount = [...kindCounts.values()].reduce((a, b) => a + b, 0);
  const formatCounts = useMemo(
    () => facets.data?.filter((f) => filters.kinds.length === 0 || filters.kinds.includes(f.kind)) ?? [],
    [facets.data, filters.kinds],
  );

  if (sources.data?.length === 0) {
    return (
      <div className="flex flex-col items-center rounded-xl border border-dashed border-border px-6 py-20 text-center">
        <p className="font-medium">Your library is empty</p>
        <p className="mt-1 text-sm text-muted">Add a folder with your 3D models to get started.</p>
        <Link to="/sources" className="mt-4 rounded-lg bg-accent px-3.5 py-2 text-sm font-medium text-accent-fg">
          Add a source
        </Link>
      </div>
    );
  }

  const currentSource = sources.data?.find((s) => s.id === filters.source);
  const folderPanel = sources.data && (
    <FolderPanel
      sources={sources.data}
      sourceId={filters.source}
      dir={filters.dir ?? ""}
      kinds={filters.kinds}
      onSelect={(source, dir) => {
        setFilters({ source, dir });
        setFoldersOpen(false);
      }}
    />
  );

  return (
    <div className="flex gap-6">
      <aside className="hidden w-60 shrink-0 lg:block">
        <div className="sticky top-6 max-h-[calc(100vh-3rem)] overflow-y-auto pr-1">{folderPanel}</div>
      </aside>
      {foldersOpen && (
        <div className="fixed inset-0 z-40 bg-black/40 lg:hidden" onClick={() => setFoldersOpen(false)}>
          <div className="h-full w-72 overflow-y-auto bg-surface p-3" onClick={(e) => e.stopPropagation()}>
            {folderPanel}
          </div>
        </div>
      )}

      <div className="min-w-0 flex-1 space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => setFoldersOpen(true)}
            className="rounded-lg border border-border bg-surface p-2 text-muted lg:hidden"
            aria-label="Folders"
          >
            <FolderTree className="size-4" />
          </button>
          <div className="relative min-w-48 flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted" aria-hidden />
            <input
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search names and folders…"
              aria-label="Search"
              className="w-full rounded-lg border border-border bg-surface py-2 pr-8 pl-9 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
            />
            {search && (
              <button
                onClick={() => setSearch("")}
                className="absolute top-1/2 right-2 -translate-y-1/2 rounded p-1 text-muted hover:text-fg"
                aria-label="Clear search"
              >
                <X className="size-3.5" />
              </button>
            )}
          </div>
          <select
            value={filters.sort}
            onChange={(e) => setFilters({ sort: e.target.value })}
            className="rounded-lg border border-border bg-surface px-2.5 py-2 text-sm"
            aria-label="Sort"
          >
            <option value="-mtime">Newest first</option>
            <option value="name">Name</option>
            <option value="-size">Largest first</option>
          </select>
          <input
            type="range"
            min={110}
            max={320}
            step={10}
            value={tile}
            onChange={(e) => setTile(Number(e.target.value))}
            className="hidden w-24 accent-accent sm:block"
            aria-label="Thumbnail size"
          />
        </div>

        <div className="flex flex-wrap gap-1.5">
          <Chip active={filters.kinds.length === 0} onClick={() => setFilters({ kinds: [], formats: [] })}>
            All <Count n={allCount} />
          </Chip>
          {Object.entries(kindLabels)
            .filter(([k]) => kindCounts.get(k) || filters.kinds.includes(k))
            .map(([k, label]) => (
              <Chip
                key={k}
                active={filters.kinds.length === 1 && filters.kinds[0] === k}
                onClick={() => setFilters({ kinds: [k], formats: [] })}
              >
                {label} <Count n={kindCounts.get(k) ?? 0} />
              </Chip>
            ))}
        </div>
        {formatCounts.length > 1 && (
          <div className="flex flex-wrap gap-1.5">
            {formatCounts.map((f) => {
              const on = filters.formats.includes(f.format);
              return (
                <Chip
                  key={`${f.kind}-${f.format}`}
                  small
                  active={on}
                  onClick={() =>
                    setFilters({
                      formats: on ? filters.formats.filter((x) => x !== f.format) : [...filters.formats, f.format],
                    })
                  }
                >
                  <span className="uppercase">{f.format}</span> <Count n={f.count} />
                </Chip>
              );
            })}
          </div>
        )}

        <p className="text-sm text-muted">
          {total !== undefined ? `${formatNumber(total)} ${total === 1 ? "file" : "files"}` : " "}
          {currentSource && (
            <>
              {" in "}
              <span className="text-fg">
                {currentSource.name}
                {filters.dir ? ` / ${filters.dir}` : ""}
              </span>
            </>
          )}
        </p>

        <ErrorText>{files.error?.message}</ErrorText>
        {files.isPending ? (
          <Spinner />
        ) : items.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border px-6 py-16 text-center text-sm text-muted">
            No files match these filters.
          </div>
        ) : (
          <div style={{ "--tile": `${tile}px` } as React.CSSProperties}>
            <VirtuosoGrid
              useWindowScroll
              data={items}
              increaseViewportBy={600}
              listClassName="grid gap-3 grid-cols-[repeat(auto-fill,minmax(var(--tile),1fr))]"
              endReached={() => files.hasNextPage && !files.isFetchingNextPage && files.fetchNextPage()}
              itemContent={(_, file) => (
                <Tile
                  file={file}
                  onOpen={() => setSelected(file.id)}
                  onView={previewable(file) ? () => setViewing(file.id) : undefined}
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
      </div>

      {selected !== null && (
        <FileDrawer id={selected} onClose={() => setSelected(null)} onView={(id) => setViewing(id)} />
      )}
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
    </div>
  );
}

function Tile({ file, onOpen, onView }: { file: FileItem; onOpen: () => void; onView?: () => void }) {
  return (
    <button
      onClick={onOpen}
      onDoubleClick={onView}
      className="group block w-full text-left"
      title={onView ? `${file.rel_path}
Double-click to open` : file.rel_path}
    >
      <div className="aspect-square overflow-hidden rounded-xl border border-border bg-surface p-2 transition group-hover:border-accent/60 group-focus-visible:ring-2 group-focus-visible:ring-accent">
        <Thumbnail file={file} className="transition group-hover:scale-[1.03]" />
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

function Chip({
  active,
  small,
  onClick,
  children,
}: {
  active: boolean;
  small?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      aria-pressed={active}
      className={`rounded-full border ${small ? "px-2.5 py-0.5 text-xs" : "px-3 py-1 text-sm"} transition ${
        active ? "border-accent bg-accent text-accent-fg" : "border-border bg-surface text-muted hover:text-fg"
      }`}
    >
      {children}
    </button>
  );
}

function Count({ n }: { n: number }) {
  return <span className="tabular-nums opacity-70">{formatNumber(n)}</span>;
}
