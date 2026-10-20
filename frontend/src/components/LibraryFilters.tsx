import { Check, Folder, FolderX, Minus, RotateCcw, X } from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router";
import { useClearFolderCover, useFolders, useTags } from "../api/organize";
import { formatNumber } from "../lib/format";
import { Thumbnail } from "./Thumbnail";
import { TagChip, TagDot } from "./tags";

export type TagState = { tags: number[]; notTags: number[]; tagAny: boolean; unfiled: boolean };

/**
 * Tags in the library's side panel. Click a tag to require it, again to rule it out,
 * a third time to clear it. "Not in a collection" shows what hasn't been filed yet.
 */
export function TagFilter({ value, onChange }: { value: TagState; onChange: (patch: Partial<TagState>) => void }) {
  const { data: tags = [], isPending } = useTags();
  const [find, setFind] = useState("");
  const shown = useMemo(() => {
    const q = find.trim().toLowerCase();
    return q ? tags.filter((t) => t.name.toLowerCase().includes(q)) : tags;
  }, [tags, find]);

  function cycle(id: number) {
    if (value.tags.includes(id)) onChange({ tags: value.tags.filter((t) => t !== id), notTags: [...value.notTags, id] });
    else if (value.notTags.includes(id)) onChange({ notTags: value.notTags.filter((t) => t !== id) });
    else onChange({ tags: [...value.tags, id] });
  }

  return (
    <section aria-label="Tags" className="space-y-1">
      <h2 className="px-2 pb-1 text-xs font-medium text-muted">Tags</h2>

      {tags.length > 12 && (
        <input
          type="search"
          value={find}
          onChange={(e) => setFind(e.target.value)}
          placeholder="Find a tag…"
          aria-label="Find a tag"
          className="mb-1 w-full rounded-lg border border-border bg-surface px-2.5 py-1.5 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
        />
      )}

      {isPending ? (
        <div className="space-y-1.5 px-2" aria-label="Loading tags">
          {[70, 55, 80].map((w) => (
            <div key={w} className="h-4 animate-pulse rounded bg-surface-2" style={{ width: `${w}%` }} />
          ))}
        </div>
      ) : tags.length === 0 ? (
        <p className="px-2 text-sm text-muted">
          No tags yet. Open a file to tag it, or choose Select above the grid to tag many at once.
        </p>
      ) : (
        <ul>
          {shown.map((t) => {
            const on = value.tags.includes(t.id);
            const off = value.notTags.includes(t.id);
            return (
              <li key={t.id}>
                <button
                  onClick={() => cycle(t.id)}
                  aria-label={`${t.name}, ${t.files} files. ${on ? "Required. Click to rule out." : off ? "Ruled out. Click to clear." : "Click to require."}`}
                  className={`flex min-h-9 w-full items-center gap-2 rounded-md px-2 py-1 text-left text-sm transition focus-visible:outline-2 focus-visible:outline-accent ${
                    on ? "bg-accent/10 font-medium text-accent" : "text-muted hover:bg-surface-2 hover:text-fg"
                  }`}
                >
                  <TagDot color={t.color} />
                  <span className={`truncate ${off ? "line-through" : ""}`}>{t.name}</span>
                  <span className="ml-auto flex items-center gap-1.5">
                    <span className="text-xs tabular-nums opacity-70">{formatNumber(t.files)}</span>
                    <span className="grid size-4 place-items-center">
                      {on && <Check className="size-3.5" aria-hidden />}
                      {off && <Minus className="size-3.5" aria-hidden />}
                    </span>
                  </span>
                </button>
              </li>
            );
          })}
          {shown.length === 0 && <li className="px-2 py-1 text-sm text-muted">No tag matches “{find}”.</li>}
        </ul>
      )}

      {value.tags.length > 1 && (
        <div role="radiogroup" aria-label="Match" className="mx-2 mt-2 flex overflow-hidden rounded-lg border border-border text-xs">
          {([false, true] as const).map((any) => (
            <button
              key={String(any)}
              role="radio"
              aria-checked={value.tagAny === any}
              onClick={() => onChange({ tagAny: any })}
              className={`h-8 flex-1 ${value.tagAny === any ? "bg-accent text-accent-fg" : "text-muted hover:text-fg"}`}
            >
              {any ? "Any of them" : "All of them"}
            </button>
          ))}
        </div>
      )}

      <button
        onClick={() => onChange({ unfiled: !value.unfiled })}
        aria-pressed={value.unfiled}
        className={`mt-2 flex min-h-9 w-full items-center gap-2 rounded-md px-2 py-1 text-left text-sm transition focus-visible:outline-2 focus-visible:outline-accent ${
          value.unfiled ? "bg-accent/10 font-medium text-accent" : "text-muted hover:bg-surface-2 hover:text-fg"
        }`}
      >
        <FolderX className="size-4" aria-hidden />
        Not in a collection
      </button>
      <Link
        to="/tags"
        className="block rounded-md px-2 py-1.5 text-xs text-muted underline-offset-2 hover:text-fg hover:underline"
      >
        Manage tags
      </Link>
    </section>
  );
}

/** The tag filters in effect, as chips that can be removed one by one. */
export function ActiveFilters({ value, onChange }: { value: TagState; onChange: (patch: Partial<TagState>) => void }) {
  const { data: tags = [] } = useTags();
  const named = (ids: number[]) => ids.map((id) => tags.find((t) => t.id === id)).filter((t) => !!t);
  const inc = named(value.tags);
  const exc = named(value.notTags);
  if (inc.length + exc.length === 0 && !value.unfiled) return null;

  return (
    <div className="flex flex-wrap items-center gap-1.5" aria-label="Active tag filters">
      {inc.map((t, i) => (
        <span key={t.id} className="inline-flex items-center gap-1.5">
          {i > 0 && <span className="text-xs text-muted">{value.tagAny ? "or" : "and"}</span>}
          <TagChip tag={t} onRemove={() => onChange({ tags: value.tags.filter((x) => x !== t.id) })} removeLabel={`Stop requiring ${t.name}`} />
        </span>
      ))}
      {exc.map((t) => (
        <span key={t.id} className="inline-flex items-center gap-1.5">
          <span className="text-xs text-muted">not</span>
          <TagChip tag={t} onRemove={() => onChange({ notTags: value.notTags.filter((x) => x !== t.id) })} removeLabel={`Stop ruling out ${t.name}`} />
        </span>
      ))}
      {value.unfiled && (
        <span className="inline-flex items-center gap-1 rounded-full border border-border bg-surface-2 py-0.5 pr-1 pl-2.5 text-xs">
          Not in a collection
          <button
            onClick={() => onChange({ unfiled: false })}
            aria-label="Show files in collections too"
            className="grid size-6 place-items-center rounded-full text-muted hover:bg-border hover:text-fg"
          >
            <X className="size-3.5" aria-hidden />
          </button>
        </span>
      )}
      <button
        onClick={() => onChange({ tags: [], notTags: [], unfiled: false })}
        className="ml-1 text-xs text-muted underline-offset-2 hover:text-fg hover:underline"
      >
        Clear
      </button>
    </div>
  );
}

/** The folders inside the current one, as compact cards with a cover each. */
export function FolderCards({
  sourceId,
  dir,
  kinds,
  onOpen,
}: {
  sourceId: number;
  dir: string;
  kinds: string[];
  onOpen: (path: string) => void;
}) {
  const { data, isPending } = useFolders(sourceId, dir, kinds);
  const reset = useClearFolderCover();

  if (isPending) {
    return (
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(14rem,1fr))]" aria-label="Loading folders">
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-16 animate-pulse rounded-xl bg-surface-2 sm:h-[4.5rem]" />
        ))}
      </div>
    );
  }
  if (!data || data.length === 0) return null;

  return (
    <section aria-label="Folders in this folder">
      <ul className="grid grid-cols-2 gap-2 sm:grid-cols-[repeat(auto-fill,minmax(14rem,1fr))] sm:gap-3">
        {data.map((f) => (
          <li key={f.path} className="group relative">
            <button
              onClick={() => onOpen(f.path)}
              className="flex h-16 w-full items-center gap-2.5 rounded-xl border border-border bg-surface p-2 text-left sm:h-[4.5rem] sm:gap-3 transition hover:border-accent/60 focus-visible:outline-2 focus-visible:outline-accent"
            >
              <span className="grid size-11 shrink-0 place-items-center overflow-hidden rounded-lg bg-surface-2 sm:size-14">
                {f.cover ? <Thumbnail file={f.cover} /> : <Folder className="size-6 text-muted/60" aria-hidden />}
              </span>
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium">{f.name}</span>
                <span className="block text-xs text-muted tabular-nums">
                  {formatNumber(f.files)} {f.files === 1 ? "file" : "files"}
                </span>
              </span>
            </button>
            {f.custom && (
              <button
                onClick={() => {
                  // The folder's own path is the key the cover was saved under.
                  reset.mutate({ sourceId, dir: f.path });
                }}
                aria-label={`Use the automatic cover for ${f.name}`}
                title="Use the automatic cover"
                className="absolute top-1.5 right-1.5 grid size-7 place-items-center rounded-md border border-border bg-surface/90 text-muted opacity-0 backdrop-blur transition group-focus-within:opacity-100 group-hover:opacity-100 hover:text-fg focus-visible:outline-2 focus-visible:outline-accent max-md:opacity-100"
              >
                <RotateCcw className="size-3.5" aria-hidden />
              </button>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
