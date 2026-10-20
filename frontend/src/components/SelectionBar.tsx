import { FolderMinus, FolderPlus, ListPlus, Tag, X } from "lucide-react";
import { useEffect, useState } from "react";
import {
  type Selection,
  useAddPrintItems,
  useAddToCollection,
  useBulkTags,
  useCollections,
  useCreateCollection,
  useCreatePrintList,
  usePrintLists,
  useTags,
} from "../api/organize";
import { treeOrder } from "../lib/collections";
import { formatNumber } from "../lib/format";
import { Button } from "./ui";
import { TagCombobox } from "./tags";

type Panel = "tag" | "collection" | "queue" | null;

/**
 * The bar that appears while files are selected: tag them, file them in a collection,
 * or queue them for printing. It floats over the grid as frosted chrome.
 */
export function SelectionBar({
  count,
  total,
  allMatching,
  selection,
  onSelectAll,
  onClear,
  onTakeOut,
}: {
  count: number;
  /** How many files match the current filter, when that is more than are loaded. */
  total?: number;
  allMatching: boolean;
  selection: Selection;
  onSelectAll: () => void;
  onClear: () => void;
  /** When browsing a hand-filled collection: take the selection out of it. */
  onTakeOut?: { label: string; run: () => void; pending: boolean };
}) {
  const [panel, setPanel] = useState<Panel>(null);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(null), 4000);
    return () => clearTimeout(t);
  }, [notice]);

  const files = (n: number) => `${formatNumber(n)} ${n === 1 ? "file" : "files"}`;
  const toggle = (p: Panel) => setPanel(panel === p ? null : p);
  const canSelectAll = !allMatching && total !== undefined && total > count;

  return (
    <div
      role="region"
      aria-label="Selected files"
      className="fixed inset-x-3 bottom-3 z-30 mx-auto max-w-2xl rounded-xl border border-border bg-surface/90 backdrop-blur print:hidden"
    >
      {panel === "tag" && (
        <TagPanel selection={selection} files={files(count)} onDone={(m) => setNotice(m)} onClose={() => setPanel(null)} />
      )}
      {panel === "collection" && (
        <CollectionPanel selection={selection} onDone={(m) => { setNotice(m); setPanel(null); }} />
      )}
      {panel === "queue" && (
        <QueuePanel selection={selection} onDone={(m) => { setNotice(m); setPanel(null); }} />
      )}

      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
        <p className="text-sm tabular-nums" aria-live="polite">
          <span className="font-medium">{files(count)}</span> selected
          {canSelectAll && (
            <>
              {" · "}
              <button onClick={onSelectAll} className="text-accent underline-offset-2 hover:underline">
                Select all {formatNumber(total)}
              </button>
            </>
          )}
        </p>
        {notice && (
          <p role="status" className="text-sm text-success">
            {notice}
          </p>
        )}
        <div className="ml-auto flex flex-wrap items-center justify-end gap-1.5">
          <BarButton active={panel === "tag"} onClick={() => toggle("tag")} icon={<Tag className="size-4" aria-hidden />}>
            Tag
          </BarButton>
          <BarButton active={panel === "collection"} onClick={() => toggle("collection")} icon={<FolderPlus className="size-4" aria-hidden />}>
            Collection
          </BarButton>
          <BarButton active={panel === "queue"} onClick={() => toggle("queue")} icon={<ListPlus className="size-4" aria-hidden />}>
            Print queue
          </BarButton>
          {onTakeOut && (
            <button
              onClick={onTakeOut.run}
              disabled={onTakeOut.pending}
              className="inline-flex h-10 items-center gap-1.5 rounded-lg border border-border bg-surface px-3 text-sm font-medium transition hover:bg-surface-2 disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-accent"
            >
              <FolderMinus className="size-4" aria-hidden />
              <span>{onTakeOut.label}</span>
            </button>
          )}
          <button
            onClick={onClear}
            aria-label="Clear selection"
            title="Clear selection (Esc)"
            className="grid size-10 place-items-center rounded-lg text-muted hover:bg-surface-2 hover:text-fg focus-visible:outline-2 focus-visible:outline-accent"
          >
            <X className="size-4" aria-hidden />
          </button>
        </div>
      </div>
    </div>
  );
}

function BarButton({
  active,
  onClick,
  icon,
  children,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      aria-expanded={active}
      className={`inline-flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition focus-visible:outline-2 focus-visible:outline-accent ${
        active ? "border-accent bg-accent/10 text-accent" : "border-border bg-surface hover:bg-surface-2"
      }`}
    >
      {icon}
      <span>{children}</span>
    </button>
  );
}

function PanelShell({ children, label }: { children: React.ReactNode; label: string }) {
  return (
    <div className="space-y-2 border-b border-border p-3" role="group" aria-label={label}>
      {children}
    </div>
  );
}

function TagPanel({
  selection,
  files,
  onDone,
  onClose,
}: {
  selection: Selection;
  files: string;
  onDone: (message: string) => void;
  onClose: () => void;
}) {
  const bulk = useBulkTags();
  const { data: tags = [] } = useTags();
  const [mode, setMode] = useState<"add" | "remove">("add");

  function apply(name: string) {
    if (mode === "add") {
      bulk.mutate({ selection, add_names: [name] }, { onSuccess: () => onDone(`Tagged ${files} “${name}”`) });
      return;
    }
    const tag = tags.find((t) => t.name.toLowerCase() === name.toLowerCase());
    if (tag) bulk.mutate({ selection, remove_ids: [tag.id] }, { onSuccess: () => onDone(`Removed “${tag.name}” from ${files}`) });
  }

  return (
    <PanelShell label="Tag the selected files">
      <div className="flex items-center gap-2">
        <div role="radiogroup" aria-label="Add or remove" className="flex overflow-hidden rounded-lg border border-border text-sm">
          {(["add", "remove"] as const).map((m) => (
            <button
              key={m}
              role="radio"
              aria-checked={mode === m}
              onClick={() => setMode(m)}
              className={`h-9 px-3 ${mode === m ? "bg-accent text-accent-fg" : "text-muted hover:text-fg"}`}
            >
              {m === "add" ? "Add tag" : "Remove tag"}
            </button>
          ))}
        </div>
        <button onClick={onClose} className="ml-auto text-sm text-muted hover:text-fg">
          Close
        </button>
      </div>
      <TagCombobox
        key={mode}
        autoFocus
        onAdd={apply}
        placeholder={mode === "add" ? "Type a tag, or pick one…" : "Pick the tag to remove…"}
      />
      {bulk.error && <p role="alert" className="text-sm text-danger">{bulk.error.message}</p>}
    </PanelShell>
  );
}

function CollectionPanel({ selection, onDone }: { selection: Selection; onDone: (message: string) => void }) {
  const { data: collections = [] } = useCollections();
  const add = useAddToCollection();
  const create = useCreateCollection();
  const [name, setName] = useState("");
  const manual = treeOrder(collections).filter((r) => !r.collection.rules);
  const error = add.error?.message ?? create.error?.message;

  function addTo(id: number, label: string) {
    add.mutate({ id, selection }, { onSuccess: (r) => onDone(`Added ${formatNumber(r.files)} to ${label}`) });
  }

  return (
    <PanelShell label="Add the selected files to a collection">
      {manual.length > 0 && (
        <ul className="max-h-48 overflow-y-auto">
          {manual.map(({ collection: c, depth }) => (
            <li key={c.id}>
              <button
                onClick={() => addTo(c.id, c.name)}
                disabled={add.isPending}
                style={{ paddingLeft: `${0.75 + depth * 1}rem` }}
                className="flex min-h-10 w-full items-center rounded-lg pr-3 text-left text-sm hover:bg-surface-2 disabled:opacity-50"
              >
                <span className="truncate">{c.name}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (!name.trim()) return;
          create.mutate({ name }, { onSuccess: (c) => addTo(c.id, c.name) });
        }}
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Or start a new collection…"
          aria-label="New collection name"
          maxLength={100}
          className="min-w-0 flex-1 rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
        />
        <Button type="submit" variant="secondary" disabled={!name.trim()} loading={create.isPending}>
          Create and add
        </Button>
      </form>
      {error && <p role="alert" className="text-sm text-danger">{error}</p>}
    </PanelShell>
  );
}

function QueuePanel({ selection, onDone }: { selection: Selection; onDone: (message: string) => void }) {
  const { data: lists = [] } = usePrintLists();
  const add = useAddPrintItems();
  const create = useCreatePrintList();
  const [name, setName] = useState("");
  const error = add.error?.message ?? create.error?.message;

  function addTo(listId: number, label: string) {
    add.mutate({ listId, selection }, { onSuccess: (r) => onDone(`Queued ${formatNumber(r.files)} on ${label}`) });
  }

  return (
    <PanelShell label="Add the selected files to a print list">
      {lists.length > 0 && (
        <ul className="max-h-48 overflow-y-auto">
          {lists.map((l) => (
            <li key={l.id}>
              <button
                onClick={() => addTo(l.id, l.name)}
                disabled={add.isPending}
                className="flex min-h-10 w-full items-center gap-2 rounded-lg px-3 text-left text-sm hover:bg-surface-2 disabled:opacity-50"
              >
                <span className="truncate">{l.name}</span>
                {l.printer && <span className="truncate text-xs text-muted">{l.printer}</span>}
                <span className="ml-auto text-xs text-muted tabular-nums">{l.todo + l.printing} waiting</span>
              </button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (!name.trim()) return;
          create.mutate({ name }, { onSuccess: (l) => addTo(l.id, l.name) });
        }}
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Or start a new list…"
          aria-label="New print list name"
          maxLength={100}
          className="min-w-0 flex-1 rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
        />
        <Button type="submit" variant="secondary" disabled={!name.trim()} loading={create.isPending}>
          Create and add
        </Button>
      </form>
      {error && <p role="alert" className="text-sm text-danger">{error}</p>}
    </PanelShell>
  );
}
