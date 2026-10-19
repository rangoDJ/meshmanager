import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./client";

export type AppSettings = {
  hash_concurrency: number;
  scan_interval_hours: number;
  thumb_size: number;
  index_videos: boolean;
};

export type SettingsResponse = {
  settings: AppSettings;
  defaults: AppSettings;
  file: string;
};

export function useAppSettings() {
  return useQuery({
    queryKey: ["settings"],
    queryFn: () => api<SettingsResponse>("/settings"),
  });
}

export function useSaveAppSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AppSettings) => api<SettingsResponse>("/settings", { method: "PUT", body }),
    onSuccess: (data) => qc.setQueryData(["settings"], data),
  });
}
