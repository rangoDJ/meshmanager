import { Box, Download, FolderOpen, X } from "lucide-react";
import { useEffect } from "react";
import { Link } from "react-router";
import { useFile } from "../api/library";
import { formatBytes, formatDate, formatDims, formatNumber } from "../lib/format";
import { Thumbnail } from "./Thumbnail";
import { Spinner } from "./ui";

const PLA_DENSITY = 1.24; // g/cm³

/** Side panel with a file's details and a way into the 3D viewer. */
export function FileDrawer({
  id,
  onClose,
  onView,
}: {
  id: number;
  onClose: () => void;
  onView: (id: number) => void;
}) {
  const { data: f } = useFile(id);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <>
      <div className="fixed inset-0 z-40 bg-black/40 lg:hidden" onClick={onClose} />
      <aside
        className="fixed inset-y-0 right-0 z-50 flex w-full flex-col border-l border-border bg-surface shadow-xl sm:w-96"
        aria-label="File details"
      >
        <header className="flex items-center gap-2 border-b border-border px-4 py-3">
          <h2 className="min-w-0 flex-1 truncate font-semibold" title={f?.name}>
            {f?.name ?? "…"}
          </h2>
          <button onClick={onClose} className="rounded p-1 text-muted hover:text-fg" aria-label="Close details">
            <X className="size-5" />
          </button>
        </header>
        {!f ? (
          <div className="p-6">
            <Spinner />
          </div>
        ) : (
          <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-4">
            <button
              onClick={() => f.viewable && onView(f.id)}
              disabled={!f.viewable}
              className="group relative block aspect-square w-full overflow-hidden rounded-xl bg-surface-2 p-3 enabled:cursor-pointer"
              aria-label={f.viewable ? "Open in 3D viewer" : undefined}
            >
              <Thumbnail file={f} />
              {f.viewable && (
                <span className="absolute right-2 bottom-2 rounded-md bg-surface/90 px-2 py-1 text-xs opacity-0 transition group-hover:opacity-100">
                  Open 3D view
                </span>
              )}
            </button>
            {f.thumb === "failed" && f.thumb_error && (
              <p className="text-xs text-warning">Preview unavailable: {f.thumb_error}</p>
            )}

            <div className="flex gap-2">
              {f.viewable && (
                <button
                  onClick={() => onView(f.id)}
                  className="flex flex-1 items-center justify-center gap-2 rounded-lg bg-accent px-3.5 py-2 text-sm font-medium text-accent-fg hover:opacity-90"
                >
                  <Box className="size-4" aria-hidden /> Open 3D view
                </button>
              )}
              {f.kind === "archive" && f.format === "zip" && (
                <Link
                  to={`/library?source=${f.source_id}&dir=${encodeURIComponent(f.rel_path)}&kind=all`}
                  onClick={onClose}
                  className="flex flex-1 items-center justify-center gap-2 rounded-lg bg-accent px-3.5 py-2 text-sm font-medium text-accent-fg hover:opacity-90"
                >
                  <FolderOpen className="size-4" aria-hidden /> Browse contents
                </Link>
              )}
              <a
                href={`/api/files/${f.id}/download`}
                download={f.name}
                className={`flex flex-1 items-center justify-center gap-2 rounded-lg px-3.5 py-2 text-sm font-medium ${
                  f.viewable || (f.kind === "archive" && f.format === "zip")
                    ? "border border-border hover:bg-surface-2"
                    : "bg-accent text-accent-fg hover:opacity-90"
                }`}
              >
                <Download className="size-4" aria-hidden /> Download
              </a>
            </div>

            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
              <Fact label="Location">
                <span className="break-all">
                  {f.source_name} / {f.rel_path}
                </span>
                {f.archive_id !== null && <span className="block text-xs text-muted">Inside an archive</span>}
              </Fact>
              <Fact label="Format">
                <span className="uppercase">{f.format}</span> · {f.kind}
              </Fact>
              <Fact label="Size">{formatBytes(f.size)}</Fact>
              <Fact label="Modified">{formatDate(f.mtime)}</Fact>
              {f.dimensions && <Fact label="Dimensions">{formatDims(f.dimensions)}</Fact>}
              {f.triangles !== null && <Fact label="Triangles">{formatNumber(f.triangles)}</Fact>}
              {f.watertight !== null && (
                <Fact label="Mesh">
                  {f.watertight ? (
                    <span className="text-success">Closed (watertight)</span>
                  ) : (
                    <span className="text-warning">Open: has holes or loose edges</span>
                  )}
                </Fact>
              )}
              {f.volume_mm3 !== null && f.volume_mm3 > 0 && (
                <Fact label="Volume">
                  {(f.volume_mm3 / 1000).toFixed(1)} cm³
                  <span className="text-muted"> · ~{Math.round((f.volume_mm3 / 1000) * PLA_DENSITY)} g PLA solid</span>
                </Fact>
              )}
              {f.content_hash && (
                <Fact label="BLAKE3">
                  <span className="font-mono text-xs" title={f.content_hash}>
                    {f.content_hash.slice(0, 16)}…
                  </span>
                </Fact>
              )}
            </dl>
          </div>
        )}
      </aside>
    </>
  );
}

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-muted">{label}</dt>
      <dd className="min-w-0">{children}</dd>
    </>
  );
}
