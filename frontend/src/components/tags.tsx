import { Plus, X } from "lucide-react";
import { useId, useMemo, useState } from "react";
import { type TagColor, TAG_COLORS, type TagRef, useTags } from "../api/organize";

/** Tag colours are the user's own labels, so they appear only as small dots. */
export function TagDot({ color, className = "" }: { color: TagColor; className?: string }) {
  return (
    <span
      aria-hidden
      className={`inline-block size-2.5 shrink-0 rounded-full ${className}`}
      style={{ background: `var(--tag-${color})` }}
    />
  );
}

/** A tag as a neutral chip with its colour dot; with onRemove it can be taken off. */
export function TagChip({
  tag,
  onRemove,
  removeLabel,
}: {
  tag: Pick<TagRef, "name" | "color">;
  onRemove?: () => void;
  removeLabel?: string;
}) {
  return (
    <span className="inline-flex max-w-full items-center gap-1.5 rounded-full border border-border bg-surface-2 py-0.5 pr-1 pl-2.5 text-xs">
      <TagDot color={tag.color} />
      <span className="truncate">{tag.name}</span>
      {onRemove ? (
        <button
          type="button"
          onClick={onRemove}
          aria-label={removeLabel ?? `Remove tag ${tag.name}`}
          className="grid size-6 place-items-center rounded-full text-muted hover:bg-border hover:text-fg focus-visible:outline-2 focus-visible:outline-accent pointer-coarse:size-9"
        >
          <X className="size-3.5" aria-hidden />
        </button>
      ) : (
        <span className="pr-1.5" />
      )}
    </span>
  );
}

/**
 * A text field that suggests existing tags as you type and offers to create a new one.
 * Suggestions sit in the page flow below the field, so they are never clipped by a
 * scrolling panel.
 */
export function TagCombobox({
  onAdd,
  skip = [],
  placeholder = "Add a tag…",
  autoFocus,
  allowCreate = true,
}: {
  /** Called with the tag's name; the server creates it if it doesn't exist. */
  onAdd: (name: string) => void;
  /** Tag ids not to suggest, such as the ones a file already has. */
  skip?: number[];
  placeholder?: string;
  autoFocus?: boolean;
  /** Offer to create a tag that doesn't exist yet. Off where only existing tags make sense. */
  allowCreate?: boolean;
}) {
  const { data: tags = [] } = useTags();
  const [text, setText] = useState("");
  const [active, setActive] = useState(0);
  const [focused, setFocused] = useState(false);
  const listId = useId();

  const query = text.trim().toLowerCase();
  const matches = useMemo(
    () => tags.filter((t) => !skip.includes(t.id) && (!query || t.name.toLowerCase().includes(query))).slice(0, 6),
    [tags, skip, query],
  );
  const exact = tags.some((t) => t.name.toLowerCase() === query);
  const options = [...matches.map((t) => ({ key: `t${t.id}`, name: t.name, color: t.color as TagColor | null, create: false }))];
  if (allowCreate && query && !exact) options.push({ key: "new", name: text.trim(), color: null, create: true });

  function pick(i: number) {
    const o = options[i];
    if (!o) return;
    onAdd(o.name);
    setText("");
    setActive(0);
  }

  return (
    <div>
      <input
        value={text}
        autoFocus={autoFocus}
        onChange={(e) => {
          setText(e.target.value);
          setActive(0);
        }}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown" && options.length) {
            e.preventDefault();
            setActive((active + 1) % options.length);
          } else if (e.key === "ArrowUp" && options.length) {
            e.preventDefault();
            setActive((active - 1 + options.length) % options.length);
          } else if (e.key === "Enter" && text.trim()) {
            e.preventDefault();
            pick(Math.min(active, options.length - 1));
          } else if (e.key === "Escape" && text) {
            e.stopPropagation();
            setText("");
          }
        }}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        role="combobox"
        aria-expanded={focused && options.length > 0}
        aria-controls={listId}
        aria-activedescendant={options[active] ? `${listId}-${options[active].key}` : undefined}
        aria-autocomplete="list"
        aria-label={placeholder}
        placeholder={placeholder}
        maxLength={60}
        className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none transition focus:border-accent focus:ring-2 focus:ring-accent/25"
      />
      {focused && options.length > 0 && (
        <ul id={listId} role="listbox" className="mt-1 overflow-hidden rounded-lg border border-border bg-surface">
          {options.map((o, i) => (
            <li
              key={o.key}
              id={`${listId}-${o.key}`}
              role="option"
              aria-selected={i === active}
              onMouseDown={(e) => {
                e.preventDefault(); // keep focus in the field
                pick(i);
              }}
              onMouseEnter={() => setActive(i)}
              className={`flex min-h-10 cursor-pointer items-center gap-2 px-3 py-1.5 text-sm ${i === active ? "bg-surface-2" : ""}`}
            >
              {o.create ? (
                <>
                  <Plus className="size-3.5 text-muted" aria-hidden />
                  <span className="truncate">
                    Create tag <span className="font-medium">{o.name}</span>
                  </span>
                </>
              ) : (
                <>
                  <TagDot color={o.color as TagColor} />
                  <span className="truncate">{o.name}</span>
                </>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** A row of colour swatches for choosing a tag's colour. */
export function ColorPicker({ value, onChange, label = "Tag colour" }: { value: TagColor; onChange: (c: TagColor) => void; label?: string }) {
  return (
    <div role="radiogroup" aria-label={label} className="flex flex-wrap gap-1">
      {TAG_COLORS.map((c) => (
        <button
          key={c}
          type="button"
          role="radio"
          aria-checked={value === c}
          aria-label={c}
          title={c}
          onClick={() => onChange(c)}
          className={`grid size-9 place-items-center rounded-lg border transition focus-visible:outline-2 focus-visible:outline-accent ${
            value === c ? "border-accent bg-accent/10" : "border-border hover:bg-surface-2"
          }`}
        >
          <TagDot color={c} className="size-4" />
        </button>
      ))}
    </div>
  );
}
