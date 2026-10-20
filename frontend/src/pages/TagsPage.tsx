import { GitMerge, Palette, Pin, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router";
import {
  type Tag,
  type TagColor,
  useCreateTag,
  useDeleteTag,
  useMergeTag,
  useTags,
  useUpdateTag,
} from "../api/organize";
import { Dialog } from "../components/Dialog";
import { ColorPicker, TagDot } from "../components/tags";
import { Button, ErrorText, Spinner } from "../components/ui";
import { formatNumber } from "../lib/format";

export function TagsPage() {
  const { data: tags, isPending, error } = useTags();
  const create = useCreateTag();
  const [name, setName] = useState("");

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Tags</h1>
        <p className="mt-1 text-sm text-muted">
          Your own labels for models, such as Painted, Gift or Needs supports. Pin a tag to show its colour on every tile.
        </p>
      </div>

      <form
        className="flex max-w-md gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) create.mutate({ name }, { onSuccess: () => setName("") });
        }}
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="New tag name"
          aria-label="New tag name"
          maxLength={60}
          className="min-w-0 flex-1 rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
        />
        <Button type="submit" disabled={!name.trim()} loading={create.isPending}>
          Add tag
        </Button>
      </form>
      <ErrorText>{create.error?.message}</ErrorText>

      {isPending ? (
        <Spinner />
      ) : error ? (
        <ErrorText>{error.message}</ErrorText>
      ) : tags.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-6 py-14 text-center">
          <p className="font-medium">No tags yet</p>
          <p className="mx-auto mt-1 max-w-sm text-sm text-muted">
            Add one above, or tag models as you browse: open a file, or choose Select in the library and tag many at once.
          </p>
          <Link to="/library" className="mt-4 inline-block text-sm text-accent underline-offset-2 hover:underline">
            Go to the library
          </Link>
        </div>
      ) : (
        <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {tags.map((t) => (
            <TagRow key={t.id} tag={t} others={tags.filter((o) => o.id !== t.id)} />
          ))}
        </ul>
      )}
    </div>
  );
}

function TagRow({ tag, others }: { tag: Tag; others: Tag[] }) {
  const update = useUpdateTag();
  const merge = useMergeTag();
  const remove = useDeleteTag();
  const [panel, setPanel] = useState<"color" | "merge" | null>(null);
  const [name, setName] = useState(tag.name);
  const [into, setInto] = useState("");
  const [confirming, setConfirming] = useState(false);

  // Follow the server (a merge or rename elsewhere) unless the field is being edited.
  useEffect(() => setName(tag.name), [tag.name]);

  function rename() {
    const n = name.trim();
    if (!n || n === tag.name) {
      setName(tag.name);
      return;
    }
    update.mutate({ id: tag.id, name: n }, { onError: () => setName(tag.name) });
  }

  const error = update.error ?? merge.error ?? remove.error;

  return (
    <li className="px-3 py-2 sm:px-4">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <button
          onClick={() => setPanel(panel === "color" ? null : "color")}
          aria-expanded={panel === "color"}
          aria-label={`Colour of ${tag.name}: ${tag.color}`}
          title="Change colour"
          className="grid size-10 place-items-center rounded-lg hover:bg-surface-2 focus-visible:outline-2 focus-visible:outline-accent"
        >
          <TagDot color={tag.color} className="size-4" />
        </button>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          onBlur={rename}
          onKeyDown={(e) => {
            if (e.key === "Enter") e.currentTarget.blur();
            if (e.key === "Escape") {
              setName(tag.name);
              e.currentTarget.blur();
            }
          }}
          aria-label={`Name of ${tag.name}`}
          maxLength={60}
          className="min-w-0 flex-1 basis-40 rounded-lg border border-transparent bg-transparent px-2 py-1.5 text-sm font-medium outline-none hover:border-border focus:border-accent focus:bg-surface focus:ring-2 focus:ring-accent/25"
        />
        <Link
          to={`/library?tag=${tag.id}&kind=all`}
          className="w-16 shrink-0 text-right text-sm text-muted tabular-nums underline-offset-2 hover:text-fg hover:underline"
        >
          {formatNumber(tag.files)} {tag.files === 1 ? "file" : "files"}
        </Link>
        <div className="flex shrink-0 flex-wrap items-center">
          <ActionButton
            label="Colour"
            pressed={panel === "color"}
            onClick={() => setPanel(panel === "color" ? null : "color")}
            describe={`Change the colour of ${tag.name}`}
          >
            <Palette className="size-4" aria-hidden />
          </ActionButton>
          <ActionButton
            label={tag.pinned ? "On tiles" : "Show on tiles"}
            pressed={tag.pinned}
            onClick={() => update.mutate({ id: tag.id, pinned: !tag.pinned })}
            describe={tag.pinned ? `Stop showing ${tag.name} on tiles` : `Show ${tag.name} on tiles`}
          >
            <Pin className="size-4" aria-hidden />
          </ActionButton>
          <ActionButton
            label="Merge"
            pressed={panel === "merge"}
            disabled={others.length === 0}
            onClick={() => setPanel(panel === "merge" ? null : "merge")}
            describe={`Merge ${tag.name} into another tag`}
          >
            <GitMerge className="size-4" aria-hidden />
          </ActionButton>
          <ActionButton label="Delete" onClick={() => setConfirming(true)} describe={`Delete ${tag.name}`}>
            <Trash2 className="size-4" aria-hidden />
          </ActionButton>
        </div>
      </div>

      {panel === "color" && (
        <div className="pt-1 pb-2 pl-1">
          <ColorPicker
            value={tag.color}
            label={`Colour for ${tag.name}`}
            onChange={(c: TagColor) => update.mutate({ id: tag.id, color: c })}
          />
        </div>
      )}
      {panel === "merge" && (
        <form
          className="flex flex-wrap items-center gap-2 pt-1 pb-2 pl-1"
          onSubmit={(e) => {
            e.preventDefault();
            if (into) merge.mutate({ id: tag.id, into: Number(into) });
          }}
        >
          <label className="text-sm text-muted" htmlFor={`merge-${tag.id}`}>
            Move all its files to
          </label>
          <select
            id={`merge-${tag.id}`}
            value={into}
            onChange={(e) => setInto(e.target.value)}
            className="min-w-40 rounded-lg border border-border bg-surface px-3 py-2 text-sm"
          >
            <option value="">Choose a tag…</option>
            {others.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name}
              </option>
            ))}
          </select>
          <Button type="submit" variant="secondary" disabled={!into} loading={merge.isPending}>
            Merge
          </Button>
          <p className="basis-full text-xs text-muted">“{tag.name}” is removed afterwards. Files keep the other tag.</p>
        </form>
      )}
      {error && <div className="pb-1"><ErrorText>{error.message}</ErrorText></div>}

      {confirming && (
        <Dialog
          title={`Delete “${tag.name}”?`}
          onClose={() => setConfirming(false)}
          footer={
            <>
              <Button variant="secondary" onClick={() => setConfirming(false)}>
                Cancel
              </Button>
              <Button
                className="bg-danger text-accent-fg"
                loading={remove.isPending}
                onClick={() => remove.mutate(tag.id, { onSuccess: () => setConfirming(false) })}
              >
                Delete tag
              </Button>
            </>
          }
        >
          <p className="text-sm text-muted">
            This takes the tag off {formatNumber(tag.files)} {tag.files === 1 ? "file" : "files"} and out of any smart
            collection that uses it. <span className="text-fg">The files themselves are not touched.</span>
          </p>
          <ErrorText>{remove.error?.message}</ErrorText>
        </Dialog>
      )}
    </li>
  );
}

function ActionButton({
  label,
  describe,
  pressed,
  disabled,
  onClick,
  children,
}: {
  label: string;
  describe: string;
  pressed?: boolean;
  disabled?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      aria-label={describe}
      aria-pressed={pressed}
      className={`inline-flex h-10 items-center gap-1.5 rounded-lg px-2.5 text-sm transition focus-visible:outline-2 focus-visible:outline-accent disabled:cursor-not-allowed disabled:opacity-40 ${
        pressed ? "bg-accent/10 text-accent" : "text-muted hover:bg-surface-2 hover:text-fg"
      }`}
    >
      {children}
      <span>{label}</span>
    </button>
  );
}
