import { ExternalLink, FolderHeart, ImagePlus, ListTodo, Wand2, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import type { FileItem } from "../api/library";
import {
  type PrintStatus,
  useAddPrintItems,
  useAddToCollection,
  useBulkTags,
  useCollections,
  useCreatePrintList,
  useDeletePhoto,
  useRemoveFromCollection,
  usePrintLists,
  useSetFolderCover,
  useUpdateMeta,
  useUpdatePhoto,
  useUploadPhoto,
} from "../api/organize";
import { treeOrder } from "../lib/collections";
import { TagChip, TagCombobox } from "./tags";
import { Button } from "./ui";

const statusLabel: Record<PrintStatus, string> = { todo: "Waiting", printing: "Printing", done: "Done" };

/** Everything about a file that is the owner's own: tags, notes, where it came from, and so on. */
export function FileOrganize({ file }: { file: FileItem }) {
  return (
    <div className="space-y-6">
      <Tags file={file} />
      <Notes key={`notes-${file.id}`} file={file} />
      <Provenance key={`prov-${file.id}`} file={file} />
      <Collections file={file} />
      <PrintQueue file={file} />
      <Photos file={file} />
    </div>
  );
}

function Section({ title, children, hint }: { title: string; children: React.ReactNode; hint?: string }) {
  return (
    <section className="space-y-2">
      <div>
        <h3 className="text-sm font-medium">{title}</h3>
        {hint && <p className="text-xs text-muted">{hint}</p>}
      </div>
      {children}
    </section>
  );
}

function Tags({ file }: { file: FileItem }) {
  const bulk = useBulkTags();
  const selection = { ids: [file.id] };
  return (
    <Section title="Tags">
      {file.tags.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {file.tags.map((t) => (
            <li key={t.id} className="max-w-full">
              <TagChip tag={t} onRemove={() => bulk.mutate({ selection, remove_ids: [t.id] })} />
            </li>
          ))}
        </ul>
      )}
      <TagCombobox skip={file.tags.map((t) => t.id)} onAdd={(name) => bulk.mutate({ selection, add_names: [name] })} />
      {bulk.error && <p role="alert" className="text-sm text-danger">{bulk.error.message}</p>}
    </Section>
  );
}

/**
 * Saves edits shortly after typing stops, and once more when the panel closes, so a
 * note typed just before pressing Escape isn't lost. `baseline` is what the server has;
 * nothing is sent until it is known and differs from `value`.
 */
function useAutosave<T>(value: T, baseline: T | undefined, save: (value: T) => void) {
  const latest = useRef({ value, baseline, save });
  latest.current = { value, baseline, save };
  const dirty = (v: T, b: T | undefined) => b !== undefined && JSON.stringify(v) !== JSON.stringify(b);

  useEffect(() => {
    if (!dirty(value, baseline)) return;
    const t = setTimeout(() => save(value), 800);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value, baseline]);

  useEffect(
    () => () => {
      const l = latest.current;
      if (dirty(l.value, l.baseline)) l.save(l.value);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );
}

/** "Saving…" while a save is under way, "Saved" for a moment after. */
function SaveStatus({ pending, done }: { pending: boolean; done: boolean }) {
  const [shown, setShown] = useState(false);
  useEffect(() => {
    if (!done) return;
    setShown(true);
    const t = setTimeout(() => setShown(false), 1800);
    return () => clearTimeout(t);
  }, [done]);
  return <span role="status">{pending ? "Saving…" : shown ? "Saved" : ""}</span>;
}

function Notes({ file }: { file: FileItem }) {
  const update = useUpdateMeta();
  const [text, setText] = useState(file.meta?.notes ?? "");
  const loaded = useRef(file.meta !== undefined);

  // The details arrive after the file does; take them once, without trampling edits.
  useEffect(() => {
    if (!loaded.current && file.meta) {
      loaded.current = true;
      setText(file.meta.notes);
    }
  }, [file.meta]);

  useAutosave(text, file.meta?.notes, (notes) => update.mutate({ id: file.id, notes }));

  return (
    <Section title="Notes" hint="Print settings, what went wrong last time, ideas.">
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={3}
        maxLength={10000}
        aria-label="Notes"
        className="w-full resize-y rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none transition focus:border-accent focus:ring-2 focus:ring-accent/25"
      />
      <p className="h-4 text-xs text-muted">
        <SaveStatus pending={update.isPending} done={update.isSuccess} />
      </p>
      {update.error && <p role="alert" className="text-sm text-danger">{update.error.message}</p>}
    </Section>
  );
}

type Source = { source_url: string; designer: string; license: string };

function Provenance({ file }: { file: FileItem }) {
  const update = useUpdateMeta();
  const meta = file.meta;
  const pick = (m: FileItem["meta"]): Source | undefined =>
    m && { source_url: m.source_url, designer: m.designer, license: m.license };
  const [values, setValues] = useState<Source>(pick(meta) ?? { source_url: "", designer: "", license: "" });
  const loaded = useRef(meta !== undefined);

  useEffect(() => {
    if (!loaded.current && meta) {
      loaded.current = true;
      setValues(pick(meta)!);
    }
  }, [meta]);

  // Only the fields that changed are sent.
  useAutosave(values, pick(meta), (v) => {
    const base = pick(meta);
    const patch: Partial<Source> = {};
    for (const k of Object.keys(v) as (keyof Source)[]) if (!base || v[k] !== base[k]) patch[k] = v[k];
    update.mutate({ id: file.id, ...patch });
  });

  const link = meta?.source_url && /^https?:\/\//i.test(meta.source_url) ? meta.source_url : null;
  const fields: { key: keyof Source; label: string; placeholder: string; type?: string }[] = [
    { key: "source_url", label: "Where it's from", placeholder: "https://…", type: "url" },
    { key: "designer", label: "Designer", placeholder: "Who made it" },
    { key: "license", label: "Licence", placeholder: "For example CC BY-NC" },
  ];

  return (
    <Section title="Source">
      <div className="space-y-2.5">
        {fields.map((f) => (
          <label key={f.key} className="block">
            <span className="mb-1 block text-xs text-muted">{f.label}</span>
            <input
              type={f.type ?? "text"}
              value={values[f.key]}
              onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
              placeholder={f.placeholder}
              maxLength={f.key === "source_url" ? 2000 : 200}
              spellCheck={false}
              className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none transition focus:border-accent focus:ring-2 focus:ring-accent/25"
            />
          </label>
        ))}
      </div>
      <div className="flex h-5 items-center gap-3 text-xs text-muted">
        <SaveStatus pending={update.isPending} done={update.isSuccess} />
        {link && (
          <a
            href={link}
            target="_blank"
            rel="noopener noreferrer"
            className="ml-auto inline-flex items-center gap-1 text-accent underline-offset-2 hover:underline"
          >
            Open the page <ExternalLink className="size-3" aria-hidden />
          </a>
        )}
      </div>
      {update.error && <p role="alert" className="text-sm text-danger">{update.error.message}</p>}
    </Section>
  );
}

function Collections({ file }: { file: FileItem }) {
  const { data: all = [] } = useCollections();
  const add = useAddToCollection();
  const remove = useRemoveFromCollection();
  const selection = { ids: [file.id] };
  const mine = file.collections ?? [];
  const joinable = treeOrder(all).filter((r) => !r.collection.rules && !mine.some((m) => m.id === r.collection.id));

  return (
    <Section title="Collections">
      {mine.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {mine.map((c) => (
            <li
              key={c.id}
              className="inline-flex max-w-full items-center gap-1.5 rounded-full border border-border bg-surface-2 py-0.5 pr-1 pl-2.5 text-xs"
            >
              {c.smart ? <Wand2 className="size-3 shrink-0 text-muted" aria-label="Smart collection" /> : <FolderHeart className="size-3 shrink-0 text-muted" aria-hidden />}
              <Link to={`/collections?c=${c.id}`} className="truncate hover:underline">
                {c.name}
              </Link>
              {c.smart ? (
                <span className="pr-1.5" />
              ) : (
                <button
                  onClick={() => remove.mutate({ id: c.id, selection })}
                  aria-label={`Take out of ${c.name}`}
                  className="grid size-6 place-items-center rounded-full text-muted hover:bg-border hover:text-fg pointer-coarse:size-9"
                >
                  <X className="size-3.5" aria-hidden />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {joinable.length > 0 ? (
        <select
          value=""
          onChange={(e) => e.target.value && add.mutate({ id: Number(e.target.value), selection })}
          aria-label="Add to a collection"
          className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm"
        >
          <option value="">Add to a collection…</option>
          {joinable.map(({ collection: c, depth }) => (
            <option key={c.id} value={c.id}>
              {"  ".repeat(depth)}
              {c.name}
            </option>
          ))}
        </select>
      ) : (
        mine.length === 0 && (
          <p className="text-sm text-muted">
            Not in a collection. <Link to="/collections" className="text-accent underline-offset-2 hover:underline">Make one</Link> to group models across folders.
          </p>
        )
      )}
      {(add.error || remove.error) && <p role="alert" className="text-sm text-danger">{(add.error ?? remove.error)?.message}</p>}
    </Section>
  );
}

function PrintQueue({ file }: { file: FileItem }) {
  const { data: lists = [] } = usePrintLists();
  const add = useAddPrintItems();
  const create = useCreatePrintList();
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState("");
  const selection = { ids: [file.id] };
  const queued = file.queue ?? [];

  return (
    <Section title="Print queue">
      {queued.length > 0 && (
        <ul className="space-y-1">
          {queued.map((q) => (
            <li key={q.item_id} className="flex items-center gap-2 text-sm">
              <ListTodo className="size-4 shrink-0 text-muted" aria-hidden />
              <Link to={`/queue?list=${q.list_id}`} className="min-w-0 truncate hover:underline">
                {q.list_name}
              </Link>
              <span className="ml-auto shrink-0 text-xs text-muted tabular-nums">
                {statusLabel[q.status]} · ×{q.quantity}
              </span>
            </li>
          ))}
        </ul>
      )}
      {naming || lists.length === 0 ? (
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (!name.trim()) return;
            create.mutate(
              { name },
              {
                onSuccess: (l) =>
                  add.mutate({ listId: l.id, selection }, { onSuccess: () => { setName(""); setNaming(false); } }),
              },
            );
          }}
        >
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Name a print list…"
            aria-label="New print list name"
            maxLength={100}
            className="min-w-0 flex-1 rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
          />
          <Button type="submit" variant="secondary" disabled={!name.trim()} loading={create.isPending || add.isPending}>
            Queue it
          </Button>
        </form>
      ) : (
        <select
          value=""
          onChange={(e) => {
            if (e.target.value === "new") setNaming(true);
            else if (e.target.value) add.mutate({ listId: Number(e.target.value), selection });
          }}
          aria-label="Add to a print list"
          className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm"
        >
          <option value="">Add to a print list…</option>
          {lists.map((l) => (
            <option key={l.id} value={l.id}>
              {l.name}
              {l.printer ? ` (${l.printer})` : ""}
            </option>
          ))}
          <option value="new">New list…</option>
        </select>
      )}
      {(add.error || create.error) && <p role="alert" className="text-sm text-danger">{(add.error ?? create.error)?.message}</p>}
    </Section>
  );
}

function Photos({ file }: { file: FileItem }) {
  const photos = file.photos ?? [];
  const upload = useUploadPhoto();
  const update = useUpdatePhoto();
  const remove = useDeletePhoto();
  const folderCover = useSetFolderCover();
  const input = useRef<HTMLInputElement>(null);
  const [coverSet, setCoverSet] = useState(false);
  const [deleting, setDeleting] = useState<number | null>(null);
  const error = upload.error ?? update.error ?? remove.error ?? folderCover.error;

  useEffect(() => setCoverSet(false), [file.id]);

  async function pick(files: FileList | null) {
    if (!files) return;
    for (const photo of Array.from(files)) {
      try {
        await upload.mutateAsync({ fileId: file.id, photo });
      } catch {
        break; // the error shows below
      }
    }
    if (input.current) input.current.value = "";
  }

  return (
    <Section title="Photos" hint="Pictures of the print. One can stand in for the thumbnail.">
      {photos.length > 0 && (
        <ul className="grid grid-cols-2 gap-3">
          {photos.map((p) => (
            <li key={p.id} className="space-y-1">
              <a
                href={p.url}
                target="_blank"
                rel="noopener noreferrer"
                className={`relative block aspect-square overflow-hidden rounded-lg border bg-surface-2 ${p.is_cover ? "border-accent" : "border-border"}`}
                aria-label={`Open photo ${p.width} by ${p.height}`}
              >
                <img src={p.thumb_url} alt="" loading="lazy" className="size-full object-cover" />
                {p.is_cover && (
                  <span className="absolute bottom-1 left-1 rounded bg-surface/90 px-1.5 py-0.5 text-[10px] font-medium backdrop-blur">
                    Thumbnail
                  </span>
                )}
              </a>
              <div className="flex flex-col">
                <button
                  onClick={() => update.mutate({ id: p.id, cover: !p.is_cover })}
                  className="min-h-9 rounded-lg px-2 text-left text-xs text-muted hover:bg-surface-2 hover:text-fg"
                >
                  {p.is_cover ? "Use the generated thumbnail" : "Use as thumbnail"}
                </button>
                <button
                  onClick={() => (deleting === p.id ? remove.mutate(p.id, { onSettled: () => setDeleting(null) }) : setDeleting(p.id))}
                  onBlur={() => setDeleting((d) => (d === p.id ? null : d))}
                  className={`min-h-9 rounded-lg px-2 text-left text-xs hover:bg-surface-2 ${
                    deleting === p.id ? "font-medium text-danger" : "text-muted hover:text-danger"
                  }`}
                >
                  {deleting === p.id ? "Tap again to delete" : "Delete"}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <input
        ref={input}
        type="file"
        accept="image/jpeg,image/png,image/gif,image/webp"
        multiple
        hidden
        onChange={(e) => pick(e.target.files)}
      />
      <Button variant="secondary" onClick={() => input.current?.click()} loading={upload.isPending}>
        <ImagePlus className="size-4" aria-hidden /> Add a photo
      </Button>
      {file.dir && (
        <div>
          <button
            onClick={() => folderCover.mutate({ sourceId: file.source_id, dir: file.dir, fileId: file.id }, { onSuccess: () => setCoverSet(true) })}
            className="text-sm text-muted underline-offset-2 hover:text-fg hover:underline"
          >
            {coverSet ? "This is now the folder's cover" : "Use this as the cover of its folder"}
          </button>
        </div>
      )}
      {error && <p role="alert" className="text-sm text-danger">{error.message}</p>}
    </Section>
  );
}
