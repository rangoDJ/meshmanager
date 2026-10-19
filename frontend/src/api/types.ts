export type User = {
  id: number;
  username: string;
  is_admin: boolean;
  created_at: string;
  last_login_at: string | null;
};

export type AuthStatus = {
  setup_required: boolean;
  user: User | null;
};

export type JobState = "queued" | "running" | "done" | "failed";

export type Job = {
  id: number;
  kind: string;
  state: JobState;
  payload: Record<string, unknown>;
  progress: number;
  message: string | null;
  error: string | null;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
};

export type WorkerInfo = {
  id: string;
  hostname: string;
  version: string;
  started_at: string;
  last_seen_at: string;
};

export type SystemInfo = {
  version: string;
  go_version: string;
  platform: string;
  workers: WorkerInfo[];
};
