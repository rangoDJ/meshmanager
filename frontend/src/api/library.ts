import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "./client";
import type { Job } from "./types";

export type ThumbnailMode = "lazy" | "eager";

export type Source = {
  id: number;
  name: string;
  path: string;
  thumbnail_mode: ThumbnailMode;
  enabled: boolean;
  last_scan_finished_at: string | null;
  created_at: string;
  file_count: number;
  model_count: number;
  total_size: number;
  last_scan: Job | null;
};

export type FileItem = {
  id: number;
  source_id: number;
  source_name: string;
  rel_path: string;
  dir: string;
  name: string;
  kind: string;
  format: string;
  size: number;
  mtime: string;
  content_hash: string | null;
  /** Set for files inside an archive. */
  archive_id: number | null;
  thumb: "ready" | "pending" | "failed" | "none";
  thumb_url: string | null;
  thumb_error?: string;
  triangles: number | null;
  dimensions: [number, number, number] | null;
  volume_mm3: number | null;
  area_mm2: number | null;
  watertight: boolean | null;
  viewable: boolean;
};

export type FileList = { items: FileItem[]; next_cursor: string | null; total?: number };
export type Facet = { kind: string; format: string; count: number };
export type FolderNode = { name: string; path: string; count: number; children: FolderNode[] };
export type DirListing = {
  root: string;
  path: string;
  parent: string | null;
  dirs: { name: string; path: string }[];
};

export type FileFilters = {
  source?: number;
  dir?: string;
  kinds: string[];
  formats: string[];
  q: string;
  sort: string;
};

function filterParams(f: FileFilters) {
  const p = new URLSearchParams();
  if (f.source) p.set("source", String(f.source));
  if (f.source && f.dir) p.set("dir", f.dir);
  f.kinds.forEach((k) => p.append("kind", k));
  f.formats.forEach((k) => p.append("format", k));
  if (f.q.trim()) p.set("q", f.q.trim());
  if (f.sort) p.set("sort", f.sort);
  return p;
}

const scanActive = (s: Source) =>
  s.last_scan?.state === "queued" || s.last_scan?.state === "running";

export function useSources() {
  return useQuery({
    queryKey: ["sources"],
    queryFn: () => api<Source[]>("/sources"),
    refetchInterval: (q) => (q.state.data?.some(scanActive) ? 1000 : 15_000),
  });
}

export function useCreateSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { path: string; name?: string; thumbnail_mode: ThumbnailMode }) =>
      api<Source>("/sources", { method: "POST", body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["sources"] }),
  });
}

export function useUpdateSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: number; name?: string; thumbnail_mode?: ThumbnailMode; enabled?: boolean }) =>
      api<Source>(`/sources/${id}`, { method: "PATCH", body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["sources"] }),
  });
}

export function useDeleteSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/sources/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["sources"] });
      qc.invalidateQueries({ queryKey: ["files"] });
    },
  });
}

export function useScanSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<Job>(`/sources/${id}/scan`, { method: "POST" }),
    onSettled: () => qc.invalidateQueries({ queryKey: ["sources"] }),
  });
}

export function useDirs(path: string | null) {
  return useQuery({
    queryKey: ["dirs", path],
    queryFn: () => api<DirListing>(`/fs/dirs?path=${encodeURIComponent(path ?? "")}`),
    placeholderData: keepPreviousData,
  });
}

export function useTree(sourceId: number | undefined, kinds: string[]) {
  return useQuery({
    queryKey: ["tree", sourceId, kinds],
    queryFn: () => {
      const p = new URLSearchParams();
      kinds.forEach((k) => p.append("kind", k));
      return api<FolderNode>(`/sources/${sourceId}/tree?${p}`);
    },
    enabled: !!sourceId,
  });
}

export function useFiles(f: FileFilters) {
  return useInfiniteQuery({
    queryKey: ["files", f],
    queryFn: ({ pageParam }) => {
      const p = filterParams(f);
      p.set("limit", "120");
      if (pageParam) p.set("cursor", pageParam);
      return api<FileList>(`/files?${p}`);
    },
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    placeholderData: keepPreviousData,
  });
}

export function useFacets(f: FileFilters) {
  return useQuery({
    queryKey: ["facets", { ...f, kinds: [], formats: [], sort: "" }],
    queryFn: () => api<Facet[]>(`/files/facets?${filterParams({ ...f, kinds: [], formats: [], sort: "" })}`),
    placeholderData: keepPreviousData,
  });
}

export function useFile(id: number | null) {
  return useQuery({
    queryKey: ["file", id],
    queryFn: () => api<FileItem>(`/files/${id}`),
    enabled: id !== null,
  });
}
