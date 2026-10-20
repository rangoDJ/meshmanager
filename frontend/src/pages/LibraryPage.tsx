import { FolderTree, Search, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { type FileFilters, useFacets, useFiles, useSources } from "../api/library";
import { FileResults } from "../components/FileResults";
import { FolderPanel } from "../components/FolderPanel";
import { ActiveFilters, FolderCards, TagFilter, type TagState } from "../components/LibraryFilters";
import { Button, Spinner } from "../components/ui";
import { formatNumber } from "../lib/format";
import { useDebounced, useStoredState } from "../lib/hooks";

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
    tags: params.getAll("tag").map(Number).filter(Boolean),
    notTags: params.getAll("notag").map(Number).filter(Boolean),
    tagAny: params.get("tags") === "any",
    unfiled: params.get("unfiled") === "1",
  };

  function update(patch: Partial<FileFilters>, replace = false) {
    const next = { ...filters, ...patch };
    // The viewer and details ride along in the URL; leave them be when filters change.
    const p = new URLSearchParams();
    if (next.source) p.set("source", String(next.source));
    if (next.source && next.dir) p.set("dir", next.dir);
    if (next.kinds.length === 0) p.set("kind", "all");
    else if (!(next.kinds.length === 1 && next.kinds[0] === "model")) next.kinds.forEach((k) => p.append("kind", k));
    next.formats.forEach((f) => p.append("format", f));
    if (next.q) p.set("q", next.q);
    if (next.sort !== "-mtime") p.set("sort", next.sort);
    next.tags?.forEach((t) => p.append("tag", String(t)));
    next.notTags?.forEach((t) => p.append("notag", String(t)));
    if (next.tagAny && (next.tags?.length ?? 0) > 1) p.set("tags", "any");
    if (next.unfiled) p.set("unfiled", "1");
    setParams(p, { replace });
  }
  return [filters, update] as const;
}

export function LibraryPage() {
  const [filters, setFilters] = useFilters();
  const [search, setSearch] = useState(filters.q);
  const debounced = useDebounced(search, 250);
  const [tile, setTile] = useStoredState("mm.tileSize", 180);
  const [foldersOpen, setFoldersOpen] = useState(false);

  useEffect(() => {
    if (debounced !== filters.q) setFilters({ q: debounced }, true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced]);

  const sources = useSources();
  const facets = useFacets(filters);
  const matching = useFiles(filters).data?.pages[0]?.total;

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

  if (sources.isPending) {
    return (
      <div className="flex justify-center py-20">
        <Spinner />
      </div>
    );
  }
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
  const tagState: TagState = {
    tags: filters.tags ?? [],
    notTags: filters.notTags ?? [],
    tagAny: !!filters.tagAny,
    unfiled: !!filters.unfiled,
  };
  const tagFiltered = tagState.tags.length + tagState.notTags.length > 0 || tagState.unfiled;
  const panels = (
    <div className="space-y-5">
      {sources.data && (
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
      )}
      <div className="border-t border-border pt-4">
        <TagFilter value={tagState} onChange={(patch) => setFilters(patch)} />
      </div>
    </div>
  );

  return (
    <div className="flex gap-6">
      <aside className="hidden w-60 shrink-0 lg:block">
        <div className="sticky top-6 max-h-[calc(100vh-3rem)] overflow-y-auto pr-1">{panels}</div>
      </aside>
      {foldersOpen && (
        <div className="fixed inset-0 z-40 bg-black/40 lg:hidden" onClick={() => setFoldersOpen(false)}>
          <div
            role="dialog"
            aria-label="Folders and tags"
            className="flex h-full w-72 flex-col bg-surface"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-border py-1.5 pr-1.5 pl-3">
              <h2 className="text-sm font-medium">Folders and tags</h2>
              <button
                onClick={() => setFoldersOpen(false)}
                aria-label="Close"
                className="grid size-10 place-items-center rounded-lg text-muted hover:bg-surface-2 hover:text-fg"
              >
                <X className="size-5" aria-hidden />
              </button>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto p-3">{panels}</div>
            <div className="border-t border-border p-3">
              <Button className="h-11 w-full" onClick={() => setFoldersOpen(false)}>
                {matching === undefined ? "Show files" : `Show ${formatNumber(matching)} ${matching === 1 ? "file" : "files"}`}
              </Button>
            </div>
          </div>
        </div>
      )}

      <div className="min-w-0 flex-1 space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => setFoldersOpen(true)}
            className="relative grid size-10 place-items-center rounded-lg border border-border bg-surface text-muted lg:hidden"
            aria-label={tagFiltered ? "Folders and tags (filtering by tags)" : "Folders and tags"}
          >
            <FolderTree className="size-4" />
            {tagFiltered && <span aria-hidden className="absolute top-1.5 right-1.5 size-2 rounded-full bg-accent" />}
          </button>
          <div className="relative min-w-48 flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted" aria-hidden />
            <input
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search names, folders and tags…"
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

        <FileResults
          filters={filters}
          tile={tile}
          scope={
            currentSource && (
              <>
                {" in "}
                <span className="text-fg">
                  {currentSource.name}
                  {filters.dir ? ` / ${filters.dir}` : ""}
                </span>
              </>
            )
          }
          lead={
            <>
              <ActiveFilters value={tagState} onChange={(patch) => setFilters(patch)} />
              {filters.source && !filters.q && !tagFiltered && (
                <FolderCards
                  sourceId={filters.source}
                  dir={filters.dir ?? ""}
                  kinds={filters.kinds}
                  onOpen={(path) => setFilters({ dir: path })}
                />
              )}
            </>
          }
        />
      </div>
    </div>
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
