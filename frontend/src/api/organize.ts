import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "./client";
import type { FileItem } from "./library";

// --- tags --------------------------------------------------------------------

export const TAG_COLORS = ["slate", "red", "orange", "amber", "green", "teal", "blue", "indigo", "violet", "pink"] as const;
export type TagColor = (typeof TAG_COLORS)[number];

/** A tag as shown on a file. */
export type TagRef = { id: number; name: string; color: TagColor; pinned: boolean };
export type Tag = TagRef & { files: number; created_at: string };

// --- file details ------------------------------------------------------------

export type FileMeta = { notes: string; source_url: string; designer: string; license: string };
export type CollectionRef = { id: number; name: string; smart: boolean };
export type QueueRef = { item_id: number; list_id: number; list_name: string; status: PrintStatus; quantity: number };
export type Photo = {
  id: number;
  file_id: number;
  content_type: string;
  width: number;
  height: number;
  size: number;
  is_cover: boolean;
  created_at: string;
  url: string;
  thumb_url: string;
};

// --- collections -------------------------------------------------------------

/** A smart collection's saved filter. */
export type Rules = {
  tags?: number[];
  tag_any?: boolean;
  not_tags?: number[];
  q?: string;
  kinds?: string[];
  formats?: string[];
  source_id?: number;
  dir?: string;
};
export type Collection = {
  id: number;
  parent_id: number | null;
  name: string;
  /** Set for a smart collection, whose files are whatever matches. */
  rules: Rules | null;
  files: number;
  created_at: string;
};

// --- print queue -------------------------------------------------------------

export type PrintStatus = "todo" | "printing" | "done";
export type PrintList = {
  id: number;
  name: string;
  printer: string;
  created_at: string;
  todo: number;
  printing: number;
  done: number;
};
export type PrintItem = {
  id: number;
  list_id: number;
  quantity: number;
  status: PrintStatus;
  note: string;
  position: number;
  added_at: string;
  finished_at: string | null;
  file: FileItem;
};

// --- folders -----------------------------------------------------------------

export type FolderCard = {
  name: string;
  path: string;
  files: number;
  cover_id: number | null;
  /** True when the cover was chosen, not picked automatically. */
  custom: boolean;
  cover: FileItem | null;
};

// --- selections --------------------------------------------------------------

/** Everything the library filter can say, for acting on all matching files at once. */
export type SelectionFilter = {
  source?: number;
  dir?: string;
  kinds?: string[];
  formats?: string[];
  q?: string;
  tags?: number[];
  tag_any?: boolean;
  not_tags?: number[];
  collection?: number;
  unfiled?: boolean;
};
export type Selection = { ids: number[] } | { filter: SelectionFilter };

// --- cache refresh -----------------------------------------------------------

type Key = "files" | "file" | "facets" | "tags" | "collections" | "print-lists" | "print-items" | "folders";

function useRefresh() {
  const qc = useQueryClient();
  return (...keys: Key[]) => Promise.all(keys.map((k) => qc.invalidateQueries({ queryKey: [k] })));
}

// --- tags --------------------------------------------------------------------

export function useTags() {
  return useQuery({ queryKey: ["tags"], queryFn: () => api<Tag[]>("/tags"), staleTime: 15_000 });
}

export function useCreateTag() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: { name: string; color?: TagColor; pinned?: boolean }) => api<Tag>("/tags", { method: "POST", body }),
    onSuccess: () => refresh("tags"),
  });
}

export function useUpdateTag() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: number; name?: string; color?: TagColor; pinned?: boolean }) =>
      api<Tag>(`/tags/${id}`, { method: "PATCH", body }),
    onSuccess: () => refresh("tags", "files", "file", "collections"),
  });
}

export function useDeleteTag() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/tags/${id}`, { method: "DELETE" }),
    onSuccess: () => refresh("tags", "files", "file", "facets", "collections"),
  });
}

export function useMergeTag() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ id, into }: { id: number; into: number }) => api<void>(`/tags/${id}/merge`, { method: "POST", body: { into } }),
    onSuccess: () => refresh("tags", "files", "file", "facets", "collections"),
  });
}

/** Adds and removes tags on a selection of files. Names that don't exist yet are created. */
export function useBulkTags() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: { selection: Selection; add_ids?: number[]; add_names?: string[]; remove_ids?: number[] }) => {
      const { selection, ...rest } = body;
      return api<{ files: number; added: number }>("/bulk/tags", { method: "POST", body: { ...selection, ...rest } });
    },
    onSuccess: () => refresh("tags", "files", "file", "facets", "collections"),
  });
}

export function useUpdateMeta() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: number } & Partial<FileMeta>) =>
      api<FileMeta>(`/files/${id}/meta`, { method: "PATCH", body }),
    onSuccess: () => refresh("file", "files"),
  });
}

// --- photos ------------------------------------------------------------------

export function useUploadPhoto() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: async ({ fileId, photo }: { fileId: number; photo: File }) => {
      const form = new FormData();
      form.append("photo", photo);
      const res = await fetch(`/api/files/${fileId}/photos`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-Requested-With": "MeshManager" },
        body: form,
      });
      const data = res.headers.get("Content-Type")?.startsWith("application/json") ? await res.json() : undefined;
      if (!res.ok) throw new ApiError(res.status, data?.error ?? `Upload failed (${res.status})`);
      return data as Photo;
    },
    onSuccess: () => refresh("file", "files", "folders"),
  });
}

export function useUpdatePhoto() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ id, cover }: { id: number; cover: boolean }) => api<Photo>(`/photos/${id}`, { method: "PATCH", body: { cover } }),
    onSuccess: () => refresh("file", "files", "folders", "print-items"),
  });
}

export function useDeletePhoto() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/photos/${id}`, { method: "DELETE" }),
    onSuccess: () => refresh("file", "files", "folders", "print-items"),
  });
}

// --- folders -----------------------------------------------------------------

export function useFolders(sourceId: number | undefined, dir: string, kinds: string[]) {
  return useQuery({
    queryKey: ["folders", sourceId, dir, kinds],
    queryFn: () => {
      const p = new URLSearchParams({ dir });
      kinds.forEach((k) => p.append("kind", k));
      return api<FolderCard[]>(`/sources/${sourceId}/folders?${p}`);
    },
    enabled: !!sourceId,
  });
}

export function useSetFolderCover() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ sourceId, dir, fileId }: { sourceId: number; dir: string; fileId: number }) =>
      api<void>(`/sources/${sourceId}/folder-cover`, { method: "PUT", body: { dir, file_id: fileId } }),
    onSuccess: () => refresh("folders"),
  });
}

export function useClearFolderCover() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ sourceId, dir }: { sourceId: number; dir: string }) =>
      api<void>(`/sources/${sourceId}/folder-cover?dir=${encodeURIComponent(dir)}`, { method: "DELETE" }),
    onSuccess: () => refresh("folders"),
  });
}

// --- collections -------------------------------------------------------------

export function useCollections() {
  return useQuery({ queryKey: ["collections"], queryFn: () => api<Collection[]>("/collections"), staleTime: 15_000 });
}

export function useCreateCollection() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: { name: string; parent_id?: number | null; rules?: Rules }) =>
      api<Collection>("/collections", { method: "POST", body }),
    onSuccess: () => refresh("collections"),
  });
}

export function useUpdateCollection() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: number; name?: string; parent_id?: number | null; rules?: Rules }) =>
      api<Collection>(`/collections/${id}`, { method: "PATCH", body }),
    onSuccess: () => refresh("collections", "files", "file"),
  });
}

export function useDeleteCollection() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/collections/${id}`, { method: "DELETE" }),
    onSuccess: () => refresh("collections", "files", "file"),
  });
}

export function useAddToCollection() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ id, selection }: { id: number; selection: Selection }) =>
      api<{ files: number; added: number }>(`/collections/${id}/files`, { method: "POST", body: selection }),
    onSuccess: () => refresh("collections", "files", "file"),
  });
}

export function useRemoveFromCollection() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ id, selection }: { id: number; selection: Selection }) =>
      api<{ files: number }>(`/collections/${id}/files/remove`, { method: "POST", body: selection }),
    onSuccess: () => refresh("collections", "files", "file"),
  });
}

// --- print queue -------------------------------------------------------------

export function usePrintLists() {
  return useQuery({ queryKey: ["print-lists"], queryFn: () => api<PrintList[]>("/print-lists"), staleTime: 10_000 });
}

export function useCreatePrintList() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: { name: string; printer?: string }) => api<PrintList>("/print-lists", { method: "POST", body }),
    onSuccess: () => refresh("print-lists"),
  });
}

export function useUpdatePrintList() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: number; name?: string; printer?: string }) =>
      api<PrintList>(`/print-lists/${id}`, { method: "PATCH", body }),
    onSuccess: () => refresh("print-lists", "print-items"),
  });
}

export function useDeletePrintList() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/print-lists/${id}`, { method: "DELETE" }),
    onSuccess: () => refresh("print-lists", "print-items", "file"),
  });
}

export function usePrintItems(listId: number | null) {
  return useQuery({
    queryKey: ["print-items", listId],
    queryFn: () => api<{ list: PrintList; items: PrintItem[] }>(`/print-lists/${listId}/items`),
    enabled: listId !== null,
  });
}

export function useAddPrintItems() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ listId, selection, quantity }: { listId: number; selection: Selection; quantity?: number }) =>
      api<{ files: number; added: number }>(`/print-lists/${listId}/items`, {
        method: "POST",
        body: { ...selection, quantity },
      }),
    onSuccess: () => refresh("print-lists", "print-items", "file"),
  });
}

export function useUpdatePrintItem() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({
      id,
      ...body
    }: {
      id: number;
      status?: PrintStatus;
      quantity?: number;
      note?: string;
      position?: number;
      list_id?: number;
    }) => api<void>(`/print-items/${id}`, { method: "PATCH", body }),
    onSuccess: () => refresh("print-lists", "print-items", "file"),
  });
}

export function useDeletePrintItem() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/print-items/${id}`, { method: "DELETE" }),
    onSuccess: () => refresh("print-lists", "print-items", "file"),
  });
}

export function useClearDone() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (listId: number) => api<{ removed: number }>(`/print-lists/${listId}/clear-done`, { method: "POST" }),
    onSuccess: () => refresh("print-lists", "print-items", "file"),
  });
}
