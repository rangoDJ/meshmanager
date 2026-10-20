import { FolderHeart, FolderInput, FolderPlus, FolderX, Pencil, Trash2, Wand2 } from "lucide-react";
import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import {
  type Collection,
  type Rules,
  useCollections,
  useCreateCollection,
  useDeleteCollection,
  useUpdateCollection,
} from "../api/organize";
import { Dialog } from "../components/Dialog";
import { FileResults } from "../components/FileResults";
import { RulesEditor, rulesAreSet } from "../components/RulesEditor";
import { Button, ErrorText, Spinner } from "../components/ui";
import { descendantIds, treeOrder } from "../lib/collections";
import { formatNumber } from "../lib/format";
import { useStoredState } from "../lib/hooks";

/** Which pane is open lives in the URL: ?c=<id>, ?c=unfiled or ?c=new. */
export function CollectionsPage() {
  const [params, setParams] = useSearchParams();
  const { data: collections = [], isPending, error } = useCollections();
  const rows = treeOrder(collections);
  const param = params.get("c");
  const choose = (value: string | null) => setParams(value ? { c: value } : {}, { replace: false });

  const selected = param && /^\d+$/.test(param) ? collections.find((c) => c.id === Number(param)) : undefined;
  // With nothing chosen, open the first collection (or teach, when there are none).
  const fallback = param === null && !isPending ? rows[0]?.collection : undefined;
  const current = selected ?? fallback;
  const mode = param === "new" ? "new" : param === "unfiled" ? "unfiled" : current ? "collection" : "intro";

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Collections</h1>
        <p className="mt-1 text-sm text-muted">
          Group models across folders without moving anything. Fill a collection by hand, or make a smart one that files by tag.
        </p>
      </div>

      <div className="gap-6 lg:grid lg:grid-cols-[17rem_minmax(0,1fr)]">
        <nav aria-label="Collections" className="mb-4 lg:mb-0">
          {/* Phones pick from a list; wider screens get the tree. */}
          <div className="flex gap-2 lg:hidden">
            <select
              value={mode === "collection" ? String(current?.id) : (param ?? "")}
              onChange={(e) => choose(e.target.value || null)}
              aria-label="Collection"
              className="min-w-0 flex-1 rounded-lg border border-border bg-surface px-3 py-2.5 text-sm"
            >
              <option value="unfiled">Not in a collection</option>
              {rows.map(({ collection: c, depth }) => (
                <option key={c.id} value={c.id}>
                  {"  ".repeat(depth)}
                  {c.name}
                </option>
              ))}
              {mode === "intro" && <option value="">Choose…</option>}
            </select>
            <Button variant="secondary" onClick={() => choose("new")}>
              <FolderPlus className="size-4" aria-hidden /> New
            </Button>
          </div>

          <div className="hidden lg:block">
            <ul className="space-y-0.5 text-sm">
              <NavRow active={mode === "unfiled"} onClick={() => choose("unfiled")} icon={<FolderX className="size-4" aria-hidden />}>
                Not in a collection
              </NavRow>
              {isPending && (
                <li className="space-y-1.5 px-2 pt-2" aria-label="Loading collections">
                  {[80, 60, 70].map((w) => (
                    <div key={w} className="h-4 animate-pulse rounded bg-surface-2" style={{ width: `${w}%` }} />
                  ))}
                </li>
              )}
              {rows.map(({ collection: c, depth }) => (
                <NavRow
                  key={c.id}
                  depth={depth}
                  active={mode === "collection" && current?.id === c.id}
                  onClick={() => choose(String(c.id))}
                  icon={c.rules ? <Wand2 className="size-4" aria-label="Smart collection" /> : <FolderHeart className="size-4" aria-hidden />}
                  count={c.files}
                >
                  {c.name}
                </NavRow>
              ))}
            </ul>
            <Button variant="secondary" className="mt-3 w-full" onClick={() => choose("new")}>
              <FolderPlus className="size-4" aria-hidden /> New collection
            </Button>
          </div>
        </nav>

        <div className="min-w-0">
          <ErrorText>{error?.message}</ErrorText>
          {isPending ? (
            <Spinner />
          ) : mode === "new" ? (
            <NewCollection all={collections} onCreated={(id) => choose(String(id))} onCancel={() => choose(null)} />
          ) : mode === "unfiled" ? (
            <Unfiled />
          ) : mode === "collection" && current ? (
            <CollectionView key={current.id} collection={current} all={collections} onGone={() => choose(null)} />
          ) : (
            <Intro onNew={() => choose("new")} />
          )}
        </div>
      </div>
    </div>
  );
}

function NavRow({
  active,
  depth = 0,
  icon,
  count,
  onClick,
  children,
}: {
  active: boolean;
  depth?: number;
  icon: React.ReactNode;
  count?: number;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <li>
      <button
        onClick={onClick}
        aria-current={active ? "page" : undefined}
        style={{ paddingLeft: `${0.5 + depth * 0.875}rem` }}
        className={`flex min-h-9 w-full items-center gap-2 rounded-md py-1 pr-2 text-left transition focus-visible:outline-2 focus-visible:outline-accent ${
          active ? "bg-accent/10 font-medium text-accent" : "text-muted hover:bg-surface-2 hover:text-fg"
        }`}
      >
        <span className="shrink-0">{icon}</span>
        <span className="truncate">{children}</span>
        {count !== undefined && <span className="ml-auto pl-2 text-xs tabular-nums opacity-70">{formatNumber(count)}</span>}
      </button>
    </li>
  );
}

function Intro({ onNew }: { onNew: () => void }) {
  return (
    <div className="rounded-xl border border-dashed border-border px-6 py-14 text-center">
      <p className="font-medium">No collections yet</p>
      <p className="mx-auto mt-1 max-w-md text-sm text-muted">
        A hand-filled collection holds what you add to it, such as “Christmas prints”. A smart collection files itself:
        “everything tagged Painted and Fantasy”. Collections can sit inside each other, and the files stay where they are.
      </p>
      <Button className="mt-4" onClick={onNew}>
        <FolderPlus className="size-4" aria-hidden /> Make a collection
      </Button>
    </div>
  );
}

function Unfiled() {
  const [tile] = useStoredState("mm.tileSize", 180);
  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold">Not in a collection</h2>
        <p className="text-sm text-muted">
          Models that no collection holds yet, hand-filled or smart. Choose Select, pick some, then Collection to file them.
        </p>
      </div>
      <FileResults
        filters={{ kinds: ["model"], formats: [], q: "", sort: "-mtime", unfiled: true }}
        tile={tile}
        empty="Every model is in a collection."
      />
    </div>
  );
}

function CollectionView({ collection, all, onGone }: { collection: Collection; all: Collection[]; onGone: () => void }) {
  const [tile] = useStoredState("mm.tileSize", 180);
  const [sort, setSort] = useState("-mtime");
  const [panel, setPanel] = useState<"rename" | "move" | "rules" | null>(null);
  const [confirming, setConfirming] = useState(false);
  const remove = useDeleteCollection();
  const children = all.filter((c) => c.parent_id === collection.id);
  const smart = collection.rules !== null;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start gap-x-3 gap-y-2">
        <div className="min-w-0 flex-1 basis-56">
          <h2 className="truncate text-lg font-semibold">{collection.name}</h2>
          <p className="text-sm text-muted">
            {smart ? "Smart collection: files itself from its rules." : "Hand-filled collection."}
          </p>
        </div>
        <div className="flex items-center gap-1.5">
          {smart && (
            <PaneButton active={panel === "rules"} onClick={() => setPanel(panel === "rules" ? null : "rules")} icon={<Wand2 className="size-4" aria-hidden />}>
              Rules
            </PaneButton>
          )}
          <PaneButton active={panel === "rename"} onClick={() => setPanel(panel === "rename" ? null : "rename")} icon={<Pencil className="size-4" aria-hidden />}>
            Rename
          </PaneButton>
          <PaneButton active={panel === "move"} onClick={() => setPanel(panel === "move" ? null : "move")} icon={<FolderInput className="size-4" aria-hidden />}>
            Move
          </PaneButton>
          <PaneButton active={false} onClick={() => setConfirming(true)} icon={<Trash2 className="size-4" aria-hidden />}>
            Delete
          </PaneButton>
        </div>
      </div>

      {panel === "rename" && <Rename collection={collection} onDone={() => setPanel(null)} />}
      {panel === "move" && <Move collection={collection} all={all} onDone={() => setPanel(null)} />}
      {panel === "rules" && smart && <EditRules collection={collection} onDone={() => setPanel(null)} />}

      {children.length > 0 && (
        <ul className="flex flex-wrap items-center gap-1.5" aria-label="Collections inside this one">
          <li className="text-sm text-muted">Inside:</li>
          {children.map((c) => (
            <li key={c.id}>
              <Link
                to={`/collections?c=${c.id}`}
                className="inline-flex h-8 items-center gap-1.5 rounded-full border border-border bg-surface px-3 text-sm hover:bg-surface-2"
              >
                {c.rules ? <Wand2 className="size-3.5 text-muted" aria-hidden /> : <FolderHeart className="size-3.5 text-muted" aria-hidden />}
                {c.name}
                <span className="text-xs text-muted tabular-nums">{formatNumber(c.files)}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}

      <FileResults
        filters={{ kinds: [], formats: [], q: "", sort, collection: collection.id }}
        tile={tile}
        inCollection={smart ? undefined : { id: collection.id, name: collection.name }}
        controls={
          <select
            value={sort}
            onChange={(e) => setSort(e.target.value)}
            aria-label="Sort"
            className="h-9 rounded-lg border border-border bg-surface px-2.5 text-sm"
          >
            <option value="-mtime">Newest first</option>
            <option value="name">Name</option>
            <option value="-size">Largest first</option>
          </select>
        }
        empty={
          smart ? (
            "Nothing matches these rules yet. Files join as soon as they do."
          ) : (
            <>
              This collection is empty. In the{" "}
              <Link to="/library" className="text-accent underline-offset-2 hover:underline">
                library
              </Link>
              , choose Select, pick some models, then Collection.
            </>
          )
        }
      />

      {confirming && (
        <Dialog
          title={`Delete “${collection.name}”?`}
          onClose={() => setConfirming(false)}
          footer={
            <>
              <Button variant="secondary" onClick={() => setConfirming(false)}>
                Cancel
              </Button>
              <Button
                className="bg-danger text-accent-fg"
                loading={remove.isPending}
                onClick={() => remove.mutate(collection.id, { onSuccess: onGone })}
              >
                Delete collection
              </Button>
            </>
          }
        >
          <p className="text-sm text-muted">
            {children.length > 0 && "The collections inside it move up one level. "}
            <span className="text-fg">The files themselves are not touched.</span>
          </p>
          <ErrorText>{remove.error?.message}</ErrorText>
        </Dialog>
      )}
    </div>
  );
}

function PaneButton({
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
      className={`inline-flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm transition focus-visible:outline-2 focus-visible:outline-accent ${
        active ? "border-accent bg-accent/10 text-accent" : "border-border bg-surface text-muted hover:text-fg"
      }`}
    >
      {icon}
      <span>{children}</span>
    </button>
  );
}

function PanelBox({ children }: { children: React.ReactNode }) {
  return <div className="space-y-3 rounded-xl border border-border bg-surface p-4">{children}</div>;
}

function Rename({ collection, onDone }: { collection: Collection; onDone: () => void }) {
  const update = useUpdateCollection();
  const [name, setName] = useState(collection.name);
  return (
    <PanelBox>
      <form
        className="flex flex-wrap gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) update.mutate({ id: collection.id, name }, { onSuccess: onDone });
        }}
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
          aria-label="Collection name"
          maxLength={100}
          className="min-w-0 flex-1 basis-48 rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
        />
        <Button type="submit" disabled={!name.trim() || name.trim() === collection.name} loading={update.isPending}>
          Save
        </Button>
        <Button type="button" variant="ghost" onClick={onDone}>
          Cancel
        </Button>
      </form>
      <ErrorText>{update.error?.message}</ErrorText>
    </PanelBox>
  );
}

function Move({ collection, all, onDone }: { collection: Collection; all: Collection[]; onDone: () => void }) {
  const update = useUpdateCollection();
  const blocked = descendantIds(all, collection.id);
  const options = treeOrder(all).filter((r) => !blocked.has(r.collection.id));
  const [parent, setParent] = useState(collection.parent_id === null ? "" : String(collection.parent_id));
  return (
    <PanelBox>
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          update.mutate({ id: collection.id, parent_id: parent ? Number(parent) : null }, { onSuccess: onDone });
        }}
      >
        <label htmlFor="move-parent" className="text-sm text-muted">
          Put it inside
        </label>
        <select
          id="move-parent"
          value={parent}
          onChange={(e) => setParent(e.target.value)}
          className="min-w-48 rounded-lg border border-border bg-surface px-3 py-2 text-sm"
        >
          <option value="">Nothing (top level)</option>
          {options.map(({ collection: c, depth }) => (
            <option key={c.id} value={c.id}>
              {"  ".repeat(depth)}
              {c.name}
            </option>
          ))}
        </select>
        <Button type="submit" loading={update.isPending}>
          Move
        </Button>
        <Button type="button" variant="ghost" onClick={onDone}>
          Cancel
        </Button>
      </form>
      <ErrorText>{update.error?.message}</ErrorText>
    </PanelBox>
  );
}

function EditRules({ collection, onDone }: { collection: Collection; onDone: () => void }) {
  const update = useUpdateCollection();
  const [rules, setRules] = useState<Rules>(collection.rules ?? {});
  return (
    <PanelBox>
      <RulesEditor value={rules} onChange={setRules} />
      <div className="flex gap-2">
        <Button disabled={!rulesAreSet(rules)} loading={update.isPending} onClick={() => update.mutate({ id: collection.id, rules }, { onSuccess: onDone })}>
          Save rules
        </Button>
        <Button variant="ghost" onClick={onDone}>
          Cancel
        </Button>
      </div>
      <ErrorText>{update.error?.message}</ErrorText>
    </PanelBox>
  );
}

function NewCollection({
  all,
  onCreated,
  onCancel,
}: {
  all: Collection[];
  onCreated: (id: number) => void;
  onCancel: () => void;
}) {
  const create = useCreateCollection();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<"manual" | "smart">("manual");
  const [parent, setParent] = useState("");
  const [rules, setRules] = useState<Rules>({});
  const ready = name.trim() && (kind === "manual" || rulesAreSet(rules));

  return (
    <form
      className="space-y-5"
      onSubmit={(e) => {
        e.preventDefault();
        if (!ready) return;
        create.mutate(
          { name, parent_id: parent ? Number(parent) : null, rules: kind === "smart" ? rules : undefined },
          { onSuccess: (c) => onCreated(c.id) },
        );
      }}
    >
      <h2 className="text-lg font-semibold">New collection</h2>

      <label className="block max-w-md">
        <span className="mb-1.5 block text-sm font-medium">Name</span>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
          maxLength={100}
          className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
        />
      </label>

      <fieldset>
        <legend className="mb-1.5 text-sm font-medium">How does it fill?</legend>
        <div className="grid max-w-xl gap-2 sm:grid-cols-2">
          {(
            [
              ["manual", "By hand", "You add models to it, from the library or a file's panel."],
              ["smart", "By rules", "Anything that matches its tags and search joins it automatically."],
            ] as const
          ).map(([value, label, hint]) => (
            <label
              key={value}
              className={`cursor-pointer rounded-lg border p-3 text-sm ${kind === value ? "border-accent bg-accent/5" : "border-border"}`}
            >
              <input type="radio" name="kind" value={value} checked={kind === value} onChange={() => setKind(value)} className="sr-only" />
              <span className="font-medium">{label}</span>
              <span className="mt-0.5 block text-xs text-muted">{hint}</span>
            </label>
          ))}
        </div>
      </fieldset>

      {kind === "smart" && (
        <div className="max-w-xl">
          <RulesEditor value={rules} onChange={setRules} />
        </div>
      )}

      {all.length > 0 && (
        <label className="block max-w-md">
          <span className="mb-1.5 block text-sm font-medium">Put it inside</span>
          <select
            value={parent}
            onChange={(e) => setParent(e.target.value)}
            className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm"
          >
            <option value="">Nothing (top level)</option>
            {treeOrder(all).map(({ collection: c, depth }) => (
              <option key={c.id} value={c.id}>
                {"  ".repeat(depth)}
                {c.name}
              </option>
            ))}
          </select>
        </label>
      )}

      <ErrorText>{create.error?.message}</ErrorText>
      <div className="flex gap-2">
        <Button type="submit" disabled={!ready} loading={create.isPending}>
          Create collection
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
