import { Archive, Box, FileText, Film, Image as ImageIcon, Zap } from "lucide-react";
import { useEffect, useState } from "react";
import type { FileItem } from "../api/library";

const kindIcons: Record<string, typeof Box> = {
  model: Box,
  archive: Archive,
  image: ImageIcon,
  document: FileText,
  video: Film,
  laser: Zap,
};

export function KindIcon({ kind, className = "" }: { kind: string; className?: string }) {
  const Icon = kindIcons[kind] ?? FileText;
  return <Icon className={className} aria-hidden />;
}

/**
 * Shows a file's thumbnail. Ready thumbnails load as plain cacheable images. Pending
 * ones are requested from the API (which queues rendering at high priority) and polled
 * until the worker has produced them.
 */
export function Thumbnail({ file, className = "" }: { file: FileItem; className?: string }) {
  if (file.thumb === "ready" && file.thumb_url) {
    return (
      <img
        src={file.thumb_url}
        alt=""
        loading="lazy"
        decoding="async"
        draggable={false}
        className={`h-full w-full object-contain ${className}`}
      />
    );
  }
  if (file.thumb === "pending" && file.thumb_url) {
    return <PendingThumbnail url={file.thumb_url} kind={file.kind} className={className} />;
  }
  return <Placeholder kind={file.kind} />;
}

function Placeholder({ kind, busy }: { kind: string; busy?: boolean }) {
  return (
    <div className="flex h-full w-full items-center justify-center">
      <KindIcon kind={kind} className={`size-1/4 text-muted/50 ${busy ? "animate-pulse" : ""}`} />
    </div>
  );
}

function PendingThumbnail({ url, kind, className }: { url: string; kind: string; className: string }) {
  const [src, setSrc] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const ctrl = new AbortController();
    let objectUrl: string | null = null;
    let delay = 600;

    async function poll() {
      for (let attempt = 0; attempt < 40 && !ctrl.signal.aborted; attempt++) {
        try {
          const res = await fetch(url, { credentials: "same-origin", signal: ctrl.signal });
          if (res.status === 200) {
            objectUrl = URL.createObjectURL(await res.blob());
            setSrc(objectUrl);
            return;
          }
          if (res.status !== 202) {
            setFailed(true);
            return;
          }
        } catch {
          if (ctrl.signal.aborted) return;
        }
        await new Promise((r) => setTimeout(r, delay));
        delay = Math.min(delay * 1.5, 4000);
      }
      setFailed(true);
    }
    poll();
    return () => {
      ctrl.abort();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [url]);

  if (src) {
    return <img src={src} alt="" draggable={false} className={`h-full w-full object-contain ${className}`} />;
  }
  return <Placeholder kind={kind} busy={!failed} />;
}
