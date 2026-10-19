import { useQuery } from "@tanstack/react-query";
import { Download, ExternalLink, FileText, FileWarning } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { FileItem } from "../api/library";
import { CopyLocation } from "../components/CopyLocation";
import { formatBytes, formatDate } from "../lib/format";
import { useStoredState } from "../lib/hooks";
import { type FileNavigation, InfoPanel, useViewerKeys, ViewerCorner } from "./chrome";
import Markdown from "./Markdown";

const IMAGE = new Set(["jpg", "png", "gif", "webp", "bmp", "avif"]);
const VIDEO = new Set(["mp4", "mov", "webm"]);
const MAX_TEXT = 2 << 20; // larger text files are downloaded, not shown

type ImageInfo = { width: number; height: number; dpi_x: number; dpi_y: number };

/** Photos, SVGs, PDFs, readmes and videos, opened in place. */
export default function FileViewer({
  file,
  nav,
  onClose,
}: {
  file: FileItem;
  nav?: FileNavigation;
  onClose: () => void;
}) {
  useViewerKeys(onClose, nav);
  const src = `/api/files/${file.id}/content`;

  // Swipe left/right on the content steps through the folder (phones and tablets). Not
  // where a sideways drag already means something: video seeking, wide tables and code,
  // panning an image at actual size.
  const touch = useRef<{ x: number; y: number } | null>(null);
  const swipe = {
    onTouchStart: (e: React.TouchEvent) => {
      const noSwipe = (e.target as Element).closest("video, pre, table, [data-noswipe]");
      touch.current = noSwipe ? null : { x: e.touches[0].clientX, y: e.touches[0].clientY };
    },
    onTouchEnd: (e: React.TouchEvent) => {
      const start = touch.current;
      touch.current = null;
      if (!start) return;
      const dx = e.changedTouches[0].clientX - start.x;
      const dy = e.changedTouches[0].clientY - start.y;
      if (Math.abs(dx) > 60 && Math.abs(dx) > Math.abs(dy) * 1.5) (dx < 0 ? nav?.next : nav?.prev)?.();
    },
  };

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-bg lg:flex-row" role="dialog" aria-label={`Viewing ${file.name}`}>
      <div className="relative min-h-0 flex-1 bg-surface-2/40" {...swipe}>
        <ViewerCorner nav={nav} onClose={onClose} />
        <Content file={file} src={src} />
      </div>

      <InfoPanel>
        <h2 className="truncate font-semibold" title={file.name}>
          {file.name}
        </h2>
        <p className="mb-4 text-xs text-muted uppercase">
          {file.format} · {formatBytes(file.size)}
        </p>
        <div className="space-y-5 text-sm">
          {IMAGE.has(file.format) && <PrintSize key={file.id} file={file} />}
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
            <dt className="text-muted">Modified</dt>
            <dd>{formatDate(file.mtime)}</dd>
            {file.archive_id !== null && (
              <>
                <dt className="text-muted">Stored</dt>
                <dd>Inside an archive</dd>
              </>
            )}
          </dl>
          <CopyLocation file={file} />
          <div className="flex gap-2">
            <a
              href={`/api/files/${file.id}/download`}
              download={file.name}
              className="flex flex-1 items-center justify-center gap-2 rounded-lg border border-border px-3 py-2 hover:bg-surface-2"
            >
              <Download className="size-4" aria-hidden /> Download
            </a>
            {(file.format === "pdf" || IMAGE.has(file.format) || file.format === "svg") && (
              <a
                href={src}
                target="_blank"
                rel="noopener"
                className="flex flex-1 items-center justify-center gap-2 rounded-lg border border-border px-3 py-2 hover:bg-surface-2"
              >
                <ExternalLink className="size-4" aria-hidden /> New tab
              </a>
            )}
          </div>
        </div>
      </InfoPanel>
    </div>
  );
}

function Content({ file, src }: { file: FileItem; src: string }) {
  const [failed, setFailed] = useState(false);
  const [actualSize, setActualSize] = useState(false);
  useEffect(() => {
    setFailed(false);
    setActualSize(false);
  }, [file.id]);

  if (failed) {
    return (
      <Problem
        title="This file couldn't be shown"
        detail="It may be damaged, or in a variant your browser can't display. Download it to open it in another app."
      />
    );
  }

  if (IMAGE.has(file.format)) {
    return (
      <div
        data-noswipe={actualSize || undefined}
        className={`absolute inset-0 ${actualSize ? "overflow-auto" : "flex items-center justify-center p-4 pt-16 lg:p-10"}`}
      >
        <img
          key={file.id}
          src={src}
          alt={file.name}
          onError={() => setFailed(true)}
          onClick={() => setActualSize(!actualSize)}
          className={actualSize ? "max-w-none cursor-zoom-out" : "max-h-full max-w-full cursor-zoom-in object-contain"}
          title={actualSize ? "Click to fit" : "Click for actual size"}
        />
      </div>
    );
  }
  if (file.format === "svg") {
    // Cut files are usually dark lines on nothing: show them on paper.
    return (
      <div className="absolute inset-0 flex items-center justify-center p-4 pt-16 lg:p-10">
        <img
          key={file.id}
          src={src}
          alt={file.name}
          onError={() => setFailed(true)}
          className="max-h-full max-w-full rounded-lg bg-white object-contain p-4 shadow-[0_2px_12px_rgb(0_0_0/0.25)]"
        />
      </div>
    );
  }
  if (VIDEO.has(file.format)) {
    return (
      <div className="absolute inset-0 flex items-center justify-center p-4 pt-16 lg:p-10">
        <video
          key={file.id}
          src={src}
          controls
          playsInline
          preload="metadata"
          onError={() => setFailed(true)}
          className="max-h-full max-w-full rounded-lg bg-black"
        />
      </div>
    );
  }
  if (file.format === "pdf") {
    // Phone and tablet browsers show embedded PDFs blank or as a single page; hand them
    // to the browser's own reader instead.
    if (window.matchMedia("(pointer: coarse)").matches) {
      return (
        <div className="absolute inset-0 flex items-center justify-center p-6">
          <div className="max-w-sm text-center">
            <FileText className="mx-auto mb-3 size-8 text-muted" aria-hidden />
            <p className="font-medium">PDFs open in your browser's reader</p>
            <a
              href={src}
              target="_blank"
              rel="noopener"
              className="mt-4 inline-flex items-center gap-2 rounded-lg bg-accent px-4 py-2.5 text-sm font-medium text-accent-fg hover:opacity-90"
            >
              <ExternalLink className="size-4" aria-hidden /> Open PDF
            </a>
          </div>
        </div>
      );
    }
    // The strip above the frame keeps the viewer controls off the browser's PDF toolbar.
    return (
      <div className="absolute inset-0 pt-14">
        <iframe key={file.id} src={src} title={file.name} className="h-full w-full border-t border-border" />
      </div>
    );
  }
  if (file.format === "md" || file.format === "txt") {
    return <TextContent file={file} src={src} />;
  }
  return <Problem title="No preview for this file type" detail="Download it to open it in another app." />;
}

function TextContent({ file, src }: { file: FileItem; src: string }) {
  const tooBig = file.size > MAX_TEXT;
  const { data, isPending, error } = useQuery({
    queryKey: ["file-text", file.id, file.mtime],
    queryFn: async () => {
      const res = await fetch(src, { credentials: "same-origin" });
      if (!res.ok) throw new Error(`Couldn't load the file (${res.status})`);
      return res.text();
    },
    enabled: !tooBig,
    staleTime: Infinity,
  });

  if (tooBig) {
    return (
      <Problem
        title={`${formatBytes(file.size)} is too long to show here`}
        detail="Download it to read it in a text editor."
      />
    );
  }
  if (error) return <Problem title="This file couldn't be loaded" detail={error.message} />;

  return (
    <div className="absolute inset-0 overflow-y-auto">
      <article className="mx-auto max-w-[72ch] px-5 pt-16 pb-12 lg:px-10">
        {isPending ? (
          <div className="space-y-3" aria-label="Loading">
            {[92, 80, 86, 60, 74].map((w, i) => (
              <div key={i} className="h-3 animate-pulse rounded bg-surface-2" style={{ width: `${w}%` }} />
            ))}
          </div>
        ) : file.format === "md" ? (
          <Markdown text={data ?? ""} />
        ) : (
          <pre className="font-mono text-[13px] leading-relaxed break-words whitespace-pre-wrap">{data}</pre>
        )}
      </article>
    </div>
  );
}

function Problem({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="absolute inset-0 flex items-center justify-center p-6">
      <div className="max-w-sm text-center">
        <FileWarning className="mx-auto mb-3 size-8 text-muted" aria-hidden />
        <p className="font-medium">{title}</p>
        <p className="mt-1 text-sm text-muted">{detail}</p>
      </div>
    </div>
  );
}

const DPI_CHOICES = [150, 254, 300, 600];

/** Pixel size, the file's own resolution, and how big it prints at a chosen DPI. */
function PrintSize({ file }: { file: FileItem }) {
  const { data, isPending } = useQuery({
    queryKey: ["image-info", file.id, file.mtime],
    queryFn: () => api<ImageInfo>(`/files/${file.id}/image-info`),
    staleTime: Infinity,
  });
  // Start at the file's own resolution when it records one, else the last one picked.
  const [preferred, setPreferred] = useStoredState("mm.print.dpi", 300);
  const [picked, setPicked] = useState<number | null>(null);

  if (isPending) return <div className="h-16 animate-pulse rounded-lg bg-surface-2" aria-label="Loading image details" />;
  if (!data) return null;

  const mm = (px: number, d: number) => (px / d) * 25.4;
  const fileDpi = data.dpi_x > 0 ? Math.round(data.dpi_x) : 0;
  const dpi = picked ?? (fileDpi || preferred);
  const choices = [...new Set([...DPI_CHOICES, ...(fileDpi ? [fileDpi] : []), dpi])].sort((a, b) => a - b);
  const setDpi = (d: number) => {
    setPicked(d);
    setPreferred(d);
  };
  return (
    <section aria-label="Print size" className="space-y-2">
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
        <dt className="text-muted">Pixels</dt>
        <dd className="tabular-nums">
          {data.width} × {data.height}
        </dd>
        <dt className="text-muted">Resolution</dt>
        <dd className="tabular-nums">
          {fileDpi ? `${fileDpi} dpi` : <span className="text-muted">Not recorded in the file</span>}
        </dd>
      </dl>
      <div className="rounded-lg border border-border p-3">
        <label className="flex items-center justify-between gap-2">
          <span>Printed at</span>
          <select
            value={dpi}
            onChange={(e) => setDpi(Number(e.target.value))}
            className="rounded-md border border-border bg-surface px-2 py-1 text-sm tabular-nums"
          >
            {choices.map((d) => (
              <option key={d} value={d}>
                {d} dpi{d === fileDpi ? " (file)" : ""}
              </option>
            ))}
          </select>
        </label>
        <p className="mt-1.5 text-base font-medium tabular-nums">
          {mm(data.width, dpi).toFixed(1)} × {mm(data.height, dpi).toFixed(1)} mm
        </p>
        <p className="text-xs text-muted">The largest size this image prints at {dpi} dpi without upscaling.</p>
      </div>
    </section>
  );
}
