import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { api } from "../api/client";
import { type FileItem, type FileList, useFile } from "../api/library";
import { Spinner } from "../components/ui";
import type { FileNavigation } from "./chrome";
import FileViewer from "./FileViewer";
import GcodeViewer from "./GcodeViewer";
import ModelViewer from "./ModelViewer";

/** Files that open in one of the viewers rather than only downloading. */
export function previewable(f: Pick<FileItem, "viewable" | "inline" | "format">) {
  return f.viewable || f.inline || f.format === "gcode";
}

/**
 * Opens a file in the viewer that fits it (3D model, sliced G-code, or companion file)
 * and lets the visitor step through the other previewable files in the same folder.
 * Loaded lazily, as the 3D viewers pull in three.js.
 */
export default function FilePreview({
  fileId,
  onOpen,
  onClose,
}: {
  fileId: number;
  onOpen: (id: number) => void;
  onClose: () => void;
}) {
  const { data: file, error } = useFile(fileId);

  // The folder's files, in name order; dir filters include subfolders, so keep exact matches.
  const siblings = useQuery({
    queryKey: ["folder-files", file?.source_id, file?.dir],
    queryFn: () => {
      const p = new URLSearchParams({ source: String(file!.source_id), sort: "name", limit: "500" });
      if (file!.dir) p.set("dir", file!.dir);
      for (const k of ["model", "image", "document", "video", "gcode", "laser", "archive"]) p.append("kind", k);
      return api<FileList>(`/files?${p}`);
    },
    enabled: !!file,
    staleTime: 60_000,
  });

  const nav = useMemo<FileNavigation | undefined>(() => {
    if (!file || !siblings.data) return undefined;
    const list = siblings.data.items.filter((f) => f.dir === file.dir && previewable(f));
    const index = list.findIndex((f) => f.id === file.id);
    if (index < 0) return undefined;
    return {
      index,
      count: list.length,
      prev: index > 0 ? () => onOpen(list[index - 1].id) : undefined,
      next: index < list.length - 1 ? () => onOpen(list[index + 1].id) : undefined,
    };
  }, [file, siblings.data, onOpen]);

  if (error) {
    return (
      <div className="fixed inset-0 z-50 flex flex-col items-center justify-center gap-3 bg-bg p-6 text-center">
        <p>This file is no longer in the library.</p>
        <button onClick={onClose} className="text-sm text-accent underline-offset-2 hover:underline">
          Back to the library
        </button>
      </div>
    );
  }
  if (!file) {
    return (
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg">
        <Spinner />
      </div>
    );
  }
  if (file.viewable) return <ModelViewer key={file.id} fileId={file.id} nav={nav} onClose={onClose} />;
  if (file.format === "gcode") return <GcodeViewer key={file.id} file={file} nav={nav} onClose={onClose} />;
  return <FileViewer key={file.id} file={file} nav={nav} onClose={onClose} />;
}
