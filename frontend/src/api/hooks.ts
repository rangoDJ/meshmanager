import { type QueryClient, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./client";
import type { AuthStatus, Job, SystemInfo, User } from "./types";

export const authStatusKey = ["auth", "status"] as const;

export function useAuthStatus() {
  return useQuery({
    queryKey: authStatusKey,
    queryFn: () => api<AuthStatus>("/auth/status"),
    staleTime: Infinity,
  });
}

type Credentials = { username: string; password: string };

/**
 * Switches the signed-in user: drops every cached query belonging to the previous
 * session, then updates auth status in place. (queryClient.clear() would also remove
 * the auth query that the AuthGate observes, so the gate would never see the change.)
 */
function setSignedInUser(qc: QueryClient, user: User | null) {
  qc.removeQueries({ predicate: (q) => q.queryKey[0] !== authStatusKey[0] });
  qc.setQueryData<AuthStatus>(authStatusKey, { setup_required: false, user });
}

function useSignIn(path: "/auth/login" | "/auth/setup") {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (c: Credentials) => api<User>(path, { method: "POST", body: c }),
    onSuccess: (user) => setSignedInUser(qc, user),
  });
}

export const useLogin = () => useSignIn("/auth/login");
export const useSetup = () => useSignIn("/auth/setup");

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api<void>("/auth/logout", { method: "POST" }),
    onSettled: () => setSignedInUser(qc, null),
  });
}

export function useChangePassword() {
  return useMutation({
    mutationFn: (body: { current_password: string; new_password: string }) =>
      api<void>("/auth/password", { method: "POST", body }),
  });
}

export function useSystem() {
  return useQuery({
    queryKey: ["system"],
    queryFn: () => api<SystemInfo>("/system"),
    refetchInterval: 10_000,
  });
}

export function useJobs() {
  return useQuery({
    queryKey: ["jobs"],
    queryFn: () => api<Job[]>("/jobs?limit=10"),
    // Poll quickly while something is in flight, slowly otherwise.
    refetchInterval: (q) =>
      q.state.data?.some((j) => j.state === "queued" || j.state === "running") ? 500 : 10_000,
  });
}

export function usePingJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api<Job>("/jobs/ping", { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["jobs"] }),
  });
}
