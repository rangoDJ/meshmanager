import { ChevronLeft, Folder, FolderPlus, HardDrive, Pencil, RefreshCw, Trash2 } from "lucide-react";
import { type FormEvent, useState } from "react";
import { Link, useSearchParams } from "react-router";
import {
  type Source,
  type ThumbnailMode,
  useCreateSource,
  useDeleteSource,
  useDirs,
  useScanSource,
  useSources,
  useUpdateSource,
} from "../api/library";
import { Dialog } from "../components/Dialog";
import { Button, Card, ErrorText, Field, Spinner } from "../components/ui";
import { formatBytes, formatNumber, formatRelative } from "../lib/format";

export function SourcesPage() {
  const { data, isPending, error } = useSources();
  const [adding, setAdding] = useState(false);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Sources</h1>
          <p className="mt-1 text-sm text-muted">
            Folders MeshManager indexes in place. Nothing is moved, renamed or uploaded.
          </p>
        </div>
        <Button onClick={() => setAdding(true)}>
          <FolderPlus className="size-4" aria-hidden /> Add source
        </Button>
      </div>

      {isPending ? (
        <Spinner />
      ) : error ? (
        <ErrorText>{error.message}</ErrorText>
      ) : data.length === 0 ? (
        <div className="flex flex-col items-center rounded-xl border border-dashed border-border px-6 py-16 text-center">
          <HardDrive className="mb-3 size-8 text-muted" aria-hidden />
          <p className="font-medium">No sources yet</p>
          <p className="mt-1 max-w-sm text-sm text-muted">
            Add a folder where your STL and 3MF files live to start building your library.
          </p>
          <Button className="mt-4" onClick={() => setAdding(true)}>
            Add your first source
          </Button>
        </div>
      ) : (
        <div className="space-y-4">
          {data.map((s) => (
            <SourceCard key={s.id} source={s} />
          ))}
        </div>
      )}

      {adding && <AddSourceDialog onClose={() => setAdding(false)} />}
    </div>
  );
}

function SourceCard({ source: s }: { source: Source }) {
  const scan = useScanSource();
  const update = useUpdateSource();
  const remove = useDeleteSource();
  const [params, setParams] = useSearchParams();
  const [confirming, setConfirming] = useState(false);
  // ?edit=<id> (from a file's "set this source's network location" link) opens the form.
  const [editing, setEditingState] = useState(() => params.get("edit") === String(s.id));
  const setEditing = (on: boolean) => {
    setEditingState(on);
    if (!on && params.has("edit")) {
      setParams(
        (p) => {
          const n = new URLSearchParams(p);
          n.delete("edit");
          return n;
        },
        { replace: true },
      );
    }
  };
  const job = s.last_scan;
  const active = job?.state === "queued" || job?.state === "running";

  if (editing) {
    return (
      <Card>
        <EditSource source={s} onDone={() => setEditing(false)} />
      </Card>
    );
  }

  return (
    <Card>
      <div className="flex flex-wrap items-start gap-4">
        <div className="min-w-0 flex-1">
          <Link to={`/library?source=${s.id}`} className="font-semibold hover:text-accent">
            {s.name}
          </Link>
          <p className="truncate font-mono text-xs text-muted" title={s.path}>
            {s.path}
          </p>
          {s.host_path && (
            <p className="truncate text-xs text-muted" title={s.host_path}>
              Network location: <span className="font-mono">{s.host_path}</span>
            </p>
          )}
          <p className="mt-2 text-sm text-muted">
            <span className="text-fg">{formatNumber(s.model_count)}</span> models ·{" "}
            {formatNumber(s.file_count)} files · {formatBytes(s.total_size)}
            {s.last_scan_finished_at && <> · scanned {formatRelative(s.last_scan_finished_at)}</>}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="ghost" onClick={() => setEditing(true)} aria-label={`Edit ${s.name}`} title="Edit name and location">
            <Pencil className="size-4" aria-hidden />
          </Button>
          <select
            value={s.thumbnail_mode}
            onChange={(e) => update.mutate({ id: s.id, thumbnail_mode: e.target.value as ThumbnailMode })}
            className="rounded-lg border border-border bg-surface px-2.5 py-2 text-sm"
            aria-label="Thumbnail mode"
            title="When thumbnails are rendered"
          >
            <option value="lazy">Thumbnails as you browse</option>
            <option value="eager">Thumbnails for everything</option>
          </select>
          <Button variant="secondary" onClick={() => scan.mutate(s.id)} disabled={active} loading={scan.isPending}>
            <RefreshCw className={`size-4 ${active ? "animate-spin" : ""}`} aria-hidden />
            {active ? "Scanning" : "Scan now"}
          </Button>
          <Button variant="ghost" onClick={() => setConfirming(true)} aria-label={`Remove ${s.name}`}>
            <Trash2 className="size-4" aria-hidden />
          </Button>
        </div>
      </div>

      {job && (
        <div className="mt-4">
          {active && (
            <div className="mb-1.5 h-1.5 overflow-hidden rounded-full bg-surface-2">
              <div
                className="h-full rounded-full bg-accent transition-all"
                style={{ width: `${Math.max(job.progress * 100, job.state === "running" ? 3 : 0)}%` }}
              />
            </div>
          )}
          <p className={`text-xs ${job.state === "failed" ? "text-danger" : "text-muted"}`}>
            {job.state === "queued" && "Waiting for a worker…"}
            {job.state !== "queued" && (job.error ?? job.message)}
          </p>
        </div>
      )}
      <ErrorText>{scan.error?.message ?? update.error?.message}</ErrorText>

      {confirming && (
        <Dialog
          title={`Remove ${s.name}?`}
          onClose={() => setConfirming(false)}
          footer={
            <>
              <Button variant="secondary" onClick={() => setConfirming(false)}>
                Cancel
              </Button>
              <Button
                className="bg-danger text-white"
                loading={remove.isPending}
                onClick={() => remove.mutate(s.id, { onSuccess: () => setConfirming(false) })}
              >
                Remove source
              </Button>
            </>
          }
        >
          <p className="text-sm text-muted">
            This removes the folder and its {formatNumber(s.file_count)} files from the index and deletes
            their thumbnails. <span className="text-fg">The files on disk are not touched.</span>
          </p>
          <ErrorText>{remove.error?.message}</ErrorText>
        </Dialog>
      )}
    </Card>
  );
}

/** Edits a source in place: its name, and where its folder lives on the network. */
function EditSource({ source: s, onDone }: { source: Source; onDone: () => void }) {
  const update = useUpdateSource();
  const [name, setName] = useState(s.name);
  const [hostPath, setHostPath] = useState(s.host_path ?? "");

  function submit(e: FormEvent) {
    e.preventDefault();
    update.mutate({ id: s.id, name: name.trim(), host_path: hostPath.trim() }, { onSuccess: onDone });
  }

  return (
    <form onSubmit={submit} className="space-y-4" aria-label={`Edit ${s.name}`}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Name" name="name" required value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        <Field
          label="Network location"
          name="host_path"
          value={hostPath}
          onChange={(e) => setHostPath(e.target.value)}
          placeholder="\\nas\downloads"
          hint="For example \\nas\downloads or /mnt/downloads. Used for Copy location on files; leave empty if you don't need it."
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
        />
      </div>
      <p className="text-xs text-muted">
        Inside MeshManager this folder is <span className="font-mono">{s.path}</span>.
      </p>
      <ErrorText>{update.error?.message}</ErrorText>
      <div className="flex gap-2">
        <Button type="submit" loading={update.isPending}>
          Save
        </Button>
        <Button type="button" variant="ghost" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

function AddSourceDialog({ onClose }: { onClose: () => void }) {
  const [path, setPath] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [mode, setMode] = useState<ThumbnailMode>("lazy");
  const dirs = useDirs(path);
  const create = useCreateSource();
  const current = dirs.data?.path ?? "";

  return (
    <Dialog
      title="Add source"
      onClose={onClose}
      wide
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button
            disabled={!dirs.data}
            loading={create.isPending}
            onClick={() =>
              create.mutate(
                { path: current, name: name.trim() || undefined, thumbnail_mode: mode },
                { onSuccess: onClose },
              )
            }
          >
            Add and scan
          </Button>
        </>
      }
    >
      <div className="space-y-5">
        <div>
          <p className="mb-2 text-sm font-medium">Folder</p>
          <div className="overflow-hidden rounded-lg border border-border">
            <div className="flex items-center gap-2 border-b border-border bg-surface-2 px-3 py-2">
              <button
                className="rounded p-1 text-muted hover:text-fg disabled:opacity-30"
                disabled={!dirs.data?.parent}
                onClick={() => setPath(dirs.data?.parent ?? null)}
                aria-label="Parent folder"
              >
                <ChevronLeft className="size-4" />
              </button>
              <span className="min-w-0 flex-1 truncate font-mono text-xs" title={current}>
                {current || "…"}
              </span>
            </div>
            <ul className="h-64 overflow-y-auto">
              {dirs.isError && (
                <li className="p-3">
                  <ErrorText>{dirs.error.message}</ErrorText>
                </li>
              )}
              {dirs.data?.dirs.length === 0 && (
                <li className="p-4 text-sm text-muted">No subfolders. Use this folder, or go back.</li>
              )}
              {dirs.data?.dirs.map((d) => (
                <li key={d.path}>
                  <button
                    onClick={() => setPath(d.path)}
                    className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-sm hover:bg-surface-2"
                  >
                    <Folder className="size-4 shrink-0 text-accent" aria-hidden />
                    <span className="truncate">{d.name}</span>
                  </button>
                </li>
              ))}
            </ul>
          </div>
          <p className="mt-1.5 text-xs text-muted">
            Only folders mounted under <span className="font-mono">{dirs.data?.root ?? "the library root"}</span>{" "}
            are listed. Mount more in docker-compose.yml.
          </p>
        </div>

        <Field
          label="Name"
          name="source_name"
          placeholder={current.split(/[\\/]/).pop() || "My models"}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />

        <fieldset>
          <legend className="mb-2 text-sm font-medium">Thumbnails</legend>
          <div className="grid gap-2 sm:grid-cols-2">
            {(
              [
                ["lazy", "As you browse", "Rendered when a model first scrolls into view. Best for huge libraries."],
                ["eager", "Everything up front", "Rendered in the background after every scan."],
              ] as const
            ).map(([value, label, hint]) => (
              <label
                key={value}
                className={`cursor-pointer rounded-lg border p-3 text-sm ${
                  mode === value ? "border-accent bg-accent/5" : "border-border"
                }`}
              >
                <input
                  type="radio"
                  name="mode"
                  value={value}
                  checked={mode === value}
                  onChange={() => setMode(value)}
                  className="sr-only"
                />
                <span className="font-medium">{label}</span>
                <span className="mt-0.5 block text-xs text-muted">{hint}</span>
              </label>
            ))}
          </div>
        </fieldset>
        <ErrorText>{create.error?.message}</ErrorText>
      </div>
    </Dialog>
  );
}
