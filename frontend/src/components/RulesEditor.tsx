import { useMemo } from "react";
import { useFiles } from "../api/library";
import { type Rules, useTags } from "../api/organize";
import { formatNumber } from "../lib/format";
import { TagChip, TagCombobox } from "./tags";

const KINDS: [string, string][] = [
  ["model", "Models"],
  ["archive", "Archives"],
  ["image", "Images"],
  ["document", "Documents"],
  ["laser", "Laser & CNC"],
  ["gcode", "G-code"],
  ["video", "Videos"],
];

/** True when the rules say something, which a smart collection needs. */
export function rulesAreSet(r: Rules) {
  return !!(r.tags?.length || r.not_tags?.length || r.q?.trim() || r.kinds?.length || r.formats?.length);
}

/**
 * The rules of a smart collection: files file themselves into it when they match.
 * The count underneath is live, so the effect of each change is visible straight away.
 */
export function RulesEditor({ value, onChange }: { value: Rules; onChange: (rules: Rules) => void }) {
  const { data: tags = [] } = useTags();
  const has = value.tags ?? [];
  const hasNot = value.not_tags ?? [];
  const kinds = value.kinds ?? [];

  const byId = (id: number) => tags.find((t) => t.id === id);
  const idOf = (name: string) => tags.find((t) => t.name.toLowerCase() === name.toLowerCase())?.id;

  const preview = useFiles({
    kinds,
    formats: [],
    q: value.q ?? "",
    sort: "name",
    tags: has,
    notTags: hasNot,
    tagAny: !!value.tag_any,
  });
  const matches = useMemo(() => preview.data?.pages[0]?.total, [preview.data]);
  const set = rulesAreSet(value);

  return (
    <div className="space-y-5">
      <TagRules
        label="Has these tags"
        ids={has}
        byId={byId}
        otherIds={hasNot}
        onAdd={(name) => {
          const id = idOf(name);
          if (id && !has.includes(id)) onChange({ ...value, tags: [...has, id] });
        }}
        onRemove={(id) => onChange({ ...value, tags: has.filter((t) => t !== id) })}
      />
      {has.length > 1 && (
        <div role="radiogroup" aria-label="Match" className="flex w-fit overflow-hidden rounded-lg border border-border text-sm">
          {([false, true] as const).map((any) => (
            <button
              key={String(any)}
              type="button"
              role="radio"
              aria-checked={!!value.tag_any === any}
              onClick={() => onChange({ ...value, tag_any: any })}
              className={`h-9 px-3 ${!!value.tag_any === any ? "bg-accent text-accent-fg" : "text-muted hover:text-fg"}`}
            >
              {any ? "Any of them" : "All of them"}
            </button>
          ))}
        </div>
      )}
      <TagRules
        label="Doesn't have these tags"
        ids={hasNot}
        byId={byId}
        otherIds={has}
        onAdd={(name) => {
          const id = idOf(name);
          if (id && !hasNot.includes(id)) onChange({ ...value, not_tags: [...hasNot, id] });
        }}
        onRemove={(id) => onChange({ ...value, not_tags: hasNot.filter((t) => t !== id) })}
      />

      <label className="block">
        <span className="mb-1.5 block text-sm font-medium">Name or folder contains</span>
        <input
          value={value.q ?? ""}
          onChange={(e) => onChange({ ...value, q: e.target.value })}
          placeholder="For example dragon"
          className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
        />
      </label>

      <fieldset>
        <legend className="mb-1.5 text-sm font-medium">Only these kinds of file</legend>
        <div className="flex flex-wrap gap-1.5">
          {KINDS.map(([k, label]) => {
            const on = kinds.includes(k);
            return (
              <button
                key={k}
                type="button"
                aria-pressed={on}
                onClick={() => onChange({ ...value, kinds: on ? kinds.filter((x) => x !== k) : [...kinds, k] })}
                className={`h-9 rounded-full border px-3 text-sm transition ${
                  on ? "border-accent bg-accent text-accent-fg" : "border-border bg-surface text-muted hover:text-fg"
                }`}
              >
                {label}
              </button>
            );
          })}
        </div>
        <p className="mt-1 text-xs text-muted">Leave all off to include every kind.</p>
      </fieldset>

      <p className="text-sm text-muted tabular-nums" role="status">
        {!set
          ? "Add at least one rule."
          : matches === undefined
            ? "Counting…"
            : `${formatNumber(matches)} ${matches === 1 ? "file matches" : "files match"} right now.`}
      </p>
    </div>
  );
}

function TagRules({
  label,
  ids,
  otherIds,
  byId,
  onAdd,
  onRemove,
}: {
  label: string;
  ids: number[];
  otherIds: number[];
  byId: (id: number) => { name: string; color: import("../api/organize").TagColor } | undefined;
  onAdd: (name: string) => void;
  onRemove: (id: number) => void;
}) {
  return (
    <div className="space-y-2">
      <span className="block text-sm font-medium">{label}</span>
      {ids.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {ids.map((id) => {
            const t = byId(id);
            return t ? (
              <li key={id}>
                <TagChip tag={t} onRemove={() => onRemove(id)} />
              </li>
            ) : null;
          })}
        </ul>
      )}
      <TagCombobox allowCreate={false} skip={[...ids, ...otherIds]} onAdd={onAdd} placeholder="Pick a tag…" />
    </div>
  );
}
