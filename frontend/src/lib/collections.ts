import type { Collection } from "../api/organize";

export type CollectionRow = { collection: Collection; depth: number };

/** Orders collections as a tree, each followed by the ones nested in it. */
export function treeOrder(collections: Collection[]): CollectionRow[] {
  const byParent = new Map<number | null, Collection[]>();
  const ids = new Set(collections.map((c) => c.id));
  for (const c of collections) {
    // A parent that is missing from the list counts as the top level.
    const key = c.parent_id !== null && ids.has(c.parent_id) ? c.parent_id : null;
    byParent.set(key, [...(byParent.get(key) ?? []), c]);
  }
  const out: CollectionRow[] = [];
  const walk = (parent: number | null, depth: number) => {
    for (const c of byParent.get(parent) ?? []) {
      out.push({ collection: c, depth });
      walk(c.id, depth + 1);
    }
  };
  walk(null, 0);
  return out;
}

/** The collection and the ids of everything nested in it. */
export function descendantIds(collections: Collection[], id: number): Set<number> {
  const out = new Set<number>([id]);
  let grew = true;
  while (grew) {
    grew = false;
    for (const c of collections) {
      if (c.parent_id !== null && out.has(c.parent_id) && !out.has(c.id)) {
        out.add(c.id);
        grew = true;
      }
    }
  }
  return out;
}
