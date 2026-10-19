import { Archive, ChevronRight, Folder, HardDrive, Layers } from "lucide-react";
import { useState } from "react";
import { type FolderNode, type Source, useTree } from "../api/library";
import { formatNumber } from "../lib/format";

type Props = {
  sources: Source[];
  sourceId?: number;
  dir: string;
  kinds: string[];
  onSelect: (sourceId: number | undefined, dir: string) => void;
};

/** Sidebar listing every source; the selected one expands into its folder tree. */
export function FolderPanel({ sources, sourceId, dir, kinds, onSelect }: Props) {
  return (
    <nav className="space-y-0.5 text-sm" aria-label="Folders">
      <Row active={!sourceId} onClick={() => onSelect(undefined, "")} icon={<Layers className="size-4" />}>
        All sources
      </Row>
      {sources.map((s) => (
        <div key={s.id}>
          <Row
            active={sourceId === s.id && dir === ""}
            onClick={() => onSelect(s.id, "")}
            icon={<HardDrive className="size-4" />}
          >
            {s.name}
          </Row>
          {sourceId === s.id && <SourceTree sourceId={s.id} dir={dir} kinds={kinds} onSelect={onSelect} />}
        </div>
      ))}
    </nav>
  );
}

function SourceTree({
  sourceId,
  dir,
  kinds,
  onSelect,
}: {
  sourceId: number;
  dir: string;
  kinds: string[];
  onSelect: Props["onSelect"];
}) {
  const { data } = useTree(sourceId, kinds);
  if (!data) return null;
  return (
    <ul className="ml-3 border-l border-border pl-1">
      {data.children.map((n) => (
        <TreeNode key={n.path} node={n} selected={dir} onSelect={(p) => onSelect(sourceId, p)} />
      ))}
    </ul>
  );
}

function TreeNode({ node, selected, onSelect }: { node: FolderNode; selected: string; onSelect: (p: string) => void }) {
  const onPath = selected === node.path || selected.startsWith(node.path + "/");
  const [open, setOpen] = useState(onPath);
  const hasChildren = node.children.length > 0;

  return (
    <li>
      <div
        className={`flex items-center rounded-md ${
          selected === node.path ? "bg-accent/10 text-accent" : "text-muted hover:bg-surface-2 hover:text-fg"
        }`}
      >
        <button
          className={`p-1 ${hasChildren ? "" : "invisible"}`}
          onClick={() => setOpen(!open)}
          aria-label={open ? "Collapse" : "Expand"}
          tabIndex={hasChildren ? 0 : -1}
        >
          <ChevronRight className={`size-3.5 transition-transform ${open ? "rotate-90" : ""}`} />
        </button>
        <button className="flex min-w-0 flex-1 items-center gap-1.5 py-1 pr-2 text-left" onClick={() => onSelect(node.path)}>
          {/\.zip$/i.test(node.name) ? (
            <Archive className="size-3.5 shrink-0" aria-hidden />
          ) : (
            <Folder className="size-3.5 shrink-0" aria-hidden />
          )}
          <span className="truncate">{node.name}</span>
          <span className="ml-auto pl-2 text-xs tabular-nums opacity-70">{formatNumber(node.count)}</span>
        </button>
      </div>
      {open && hasChildren && (
        <ul className="ml-3 border-l border-border pl-1">
          {node.children.map((c) => (
            <TreeNode key={c.path} node={c} selected={selected} onSelect={onSelect} />
          ))}
        </ul>
      )}
    </li>
  );
}

function Row({
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
      className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left ${
        active ? "bg-accent/10 font-medium text-accent" : "text-muted hover:bg-surface-2 hover:text-fg"
      }`}
    >
      {icon}
      <span className="truncate">{children}</span>
    </button>
  );
}
