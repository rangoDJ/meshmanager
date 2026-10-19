import { CircleAlert, CircleCheck, Play, Server } from "lucide-react";
import { useJobs, usePingJob, useSystem } from "../api/hooks";
import type { Job } from "../api/types";
import { Button, Card, ErrorText, Spinner } from "../components/ui";

export function DashboardPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Dashboard</h1>
        <p className="mt-1 text-sm text-muted">
          Milestone 0: the API, database, and background worker are wired up. Library features
          arrive in the next milestones.
        </p>
      </div>
      <div className="grid gap-6 lg:grid-cols-[1fr_1.4fr]">
        <SystemCard />
        <JobsCard />
      </div>
    </div>
  );
}

function SystemCard() {
  const { data, isPending, error } = useSystem();
  return (
    <Card title="System">
      {isPending ? (
        <Spinner />
      ) : error ? (
        <ErrorText>{error.message}</ErrorText>
      ) : (
        <div className="space-y-4 text-sm">
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
            <dt className="text-muted">Version</dt>
            <dd className="font-mono">{data.version}</dd>
            <dt className="text-muted">Runtime</dt>
            <dd className="font-mono">
              {data.go_version} · {data.platform}
            </dd>
          </dl>
          <div>
            <h3 className="mb-2 font-medium">Workers</h3>
            {data.workers.length === 0 ? (
              <p className="flex items-center gap-2 text-warning">
                <CircleAlert className="size-4" /> No worker online. Background jobs will wait.
              </p>
            ) : (
              <ul className="space-y-1.5">
                {data.workers.map((w) => (
                  <li key={w.id} className="flex items-center gap-2">
                    <Server className="size-4 text-success" aria-hidden />
                    <span className="font-mono">{w.hostname}</span>
                    <span className="text-muted">up since {formatTime(w.started_at)}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      )}
    </Card>
  );
}

function JobsCard() {
  const jobs = useJobs();
  const ping = usePingJob();
  return (
    <Card
      title="Background jobs"
      action={
        <Button variant="secondary" onClick={() => ping.mutate()} loading={ping.isPending}>
          <Play className="size-4" aria-hidden /> Run test job
        </Button>
      }
    >
      <ErrorText>{ping.error?.message ?? jobs.error?.message}</ErrorText>
      {jobs.isPending ? (
        <Spinner />
      ) : jobs.data?.length === 0 ? (
        <p className="text-sm text-muted">
          No jobs yet. Run a test job to check that the worker picks up work.
        </p>
      ) : (
        <ul className="divide-y divide-border">
          {jobs.data?.map((job) => <JobRow key={job.id} job={job} />)}
        </ul>
      )}
    </Card>
  );
}

function JobRow({ job }: { job: Job }) {
  const pct = Math.round(job.progress * 100);
  return (
    <li className="py-3 first:pt-0 last:pb-0">
      <div className="flex items-center gap-2 text-sm">
        <StateBadge state={job.state} />
        <span className="font-medium">{job.kind}</span>
        <span className="text-muted">#{job.id}</span>
        <span className="ml-auto text-xs text-muted">{formatTime(job.created_at)}</span>
      </div>
      {(job.state === "running" || job.state === "queued") && (
        <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-surface-2">
          <div className="h-full rounded-full bg-accent transition-all" style={{ width: `${pct}%` }} />
        </div>
      )}
      {(job.error || job.message) && (
        <p className={`mt-1 text-xs ${job.error ? "text-danger" : "text-muted"}`}>
          {job.error ?? job.message}
        </p>
      )}
    </li>
  );
}

function StateBadge({ state }: { state: Job["state"] }) {
  if (state === "done") return <CircleCheck className="size-4 text-success" aria-label="done" />;
  if (state === "failed") return <CircleAlert className="size-4 text-danger" aria-label="failed" />;
  return (
    <span
      className={`size-2.5 rounded-full ${state === "running" ? "animate-pulse bg-accent" : "bg-muted"}`}
      aria-label={state}
    />
  );
}

function formatTime(iso: string) {
  return new Date(iso).toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" });
}
