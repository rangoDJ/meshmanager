import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import type { FileItem } from "../api/library";
import { copyText } from "../lib/clipboard";

/**
 * Where a file lives: its real path on the host or network when the source has one
 * set, with a copy button; otherwise its path in the library and how to set one.
 */
export function CopyLocation({ file }: { file: FileItem }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");

  async function copy() {
    if (!file.host_path) return;
    setState((await copyText(file.host_path)) ? "copied" : "failed");
    setTimeout(() => setState("idle"), 2000);
  }

  if (!file.host_path) {
    return (
      <div className="min-w-0">
        <p className="break-all">
          {file.source_name} / {file.rel_path}
        </p>
        <p className="mt-0.5 text-xs text-muted">
          To copy where this is on your computer,{" "}
          <Link to={`/sources?edit=${file.source_id}`} className="text-accent underline-offset-2 hover:underline">
            set this source's network location
          </Link>
          .
        </p>
      </div>
    );
  }

  return (
    <div className="min-w-0">
      <p className="font-mono text-[13px] leading-snug break-all">{file.host_path}</p>
      <button
        onClick={copy}
        className="mt-1.5 inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-1 text-xs text-muted transition-colors hover:bg-surface-2 hover:text-fg focus-visible:outline-2 focus-visible:outline-accent"
      >
        {state === "copied" ? (
          <Check className="size-3.5 text-success" aria-hidden />
        ) : (
          <Copy className="size-3.5" aria-hidden />
        )}
        {state === "copied" ? "Copied" : state === "failed" ? "Couldn't copy; select the path above" : "Copy location"}
      </button>
    </div>
  );
}
