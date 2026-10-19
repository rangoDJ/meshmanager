import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { useAuthStatus, useChangePassword } from "../api/hooks";
import { type AppSettings, useAppSettings, useSaveAppSettings } from "../api/settings";
import { Button, Card, ErrorText, Field, Spinner } from "../components/ui";

export function SettingsPage() {
  const { data } = useAuthStatus();
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Settings</h1>
      <LibrarySettingsCard />
      <Card title="Account">
        <p className="mb-5 text-sm text-muted">
          Signed in as <span className="font-medium text-fg">{data?.user?.username}</span>
        </p>
        <ChangePasswordForm />
      </Card>
    </div>
  );
}

function ChangePasswordForm() {
  const change = useChangePassword();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [localError, setLocalError] = useState("");

  function submit(e: FormEvent) {
    e.preventDefault();
    if (next !== confirm) {
      setLocalError("New passwords don't match.");
      return;
    }
    setLocalError("");
    change.mutate(
      { current_password: current, new_password: next },
      {
        onSuccess: () => {
          setCurrent("");
          setNext("");
          setConfirm("");
        },
      },
    );
  }

  return (
    <form onSubmit={submit} className="max-w-sm space-y-4">
      <h3 className="font-medium">Change password</h3>
      <Field
        label="Current password"
        name="current_password"
        type="password"
        autoComplete="current-password"
        required
        value={current}
        onChange={(e) => setCurrent(e.target.value)}
      />
      <Field
        label="New password"
        name="new_password"
        type="password"
        autoComplete="new-password"
        required
        minLength={8}
        hint="At least 8 characters. Other devices will be signed out."
        value={next}
        onChange={(e) => setNext(e.target.value)}
      />
      <Field
        label="Confirm new password"
        name="confirm_password"
        type="password"
        autoComplete="new-password"
        required
        value={confirm}
        onChange={(e) => setConfirm(e.target.value)}
      />
      <ErrorText>{localError || change.error?.message}</ErrorText>
      {change.isSuccess && <p className="text-sm text-success">Password updated.</p>}
      <Button type="submit" loading={change.isPending}>
        Update password
      </Button>
    </form>
  );
}

const thumbSizes = [256, 384, 512, 768];
const scanIntervals: [number, string][] = [
  [0, "Off: only when I click Scan now"],
  [1, "Every hour"],
  [6, "Every 6 hours"],
  [12, "Every 12 hours"],
  [24, "Daily"],
  [168, "Weekly"],
];

/** App settings, stored in DATA_DIR/config.json and applied without a restart. */
function LibrarySettingsCard() {
  const { data, isPending, error } = useAppSettings();
  const save = useSaveAppSettings();
  const [form, setForm] = useState<AppSettings | null>(null);

  useEffect(() => {
    if (data && !form) setForm(data.settings);
  }, [data, form]);

  if (isPending || !form) {
    return (
      <Card title="Library">
        {error ? <ErrorText>{error.message}</ErrorText> : <Spinner />}
      </Card>
    );
  }

  const dirty = JSON.stringify(form) !== JSON.stringify(data?.settings);
  const isDefault = JSON.stringify(form) === JSON.stringify(data?.defaults);
  const set = <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => {
    save.reset();
    setForm({ ...form, [key]: value });
  };
  const withCurrent = <T,>(options: [number, T][], value: number, label: (v: number) => T) =>
    options.some(([v]) => v === value) ? options : [...options, [value, label(value)] as [number, T]];

  function submit(e: FormEvent) {
    e.preventDefault();
    if (form) save.mutate(form, { onSuccess: (res) => setForm(res.settings) });
  }

  return (
    <Card title="Library">
      <form onSubmit={submit} className="space-y-5">
        <Setting label="Thumbnail size" hint="Applies to thumbnails rendered from now on.">
          <select
            value={form.thumb_size}
            onChange={(e) => set("thumb_size", Number(e.target.value))}
            className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm"
          >
            {withCurrent(
              thumbSizes.map((s) => [s, `${s} × ${s} px`] as [number, string]),
              form.thumb_size,
              (v) => `${v} × ${v} px`,
            ).map(([v, label]) => (
              <option key={v} value={v}>
                {label}
              </option>
            ))}
          </select>
        </Setting>

        <Setting label="Rescan sources" hint="Picks up new, changed and deleted files automatically.">
          <select
            value={form.scan_interval_hours}
            onChange={(e) => set("scan_interval_hours", Number(e.target.value))}
            className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm"
          >
            {withCurrent(scanIntervals, form.scan_interval_hours, (v) => `Every ${v} hours`).map(([v, label]) => (
              <option key={v} value={v}>
                {label}
              </option>
            ))}
          </select>
        </Setting>

        <Setting
          label="Files hashed in parallel"
          hint="Used to find duplicates. Lower it for spinning disks or a slow NAS."
        >
          <input
            type="number"
            min={1}
            max={32}
            required
            value={form.hash_concurrency}
            onChange={(e) => set("hash_concurrency", Number(e.target.value))}
            className="w-24 rounded-lg border border-border bg-surface px-3 py-2 text-sm"
          />
        </Setting>

        <label className="flex cursor-pointer items-start gap-3">
          <input
            type="checkbox"
            checked={form.index_videos}
            onChange={(e) => set("index_videos", e.target.checked)}
            className="mt-0.5 size-4 accent-accent"
          />
          <span>
            <span className="block text-sm font-medium">Index videos</span>
            <span className="block text-xs text-muted">Include timelapses and other videos on the next scan.</span>
          </span>
        </label>

        <ErrorText>{save.error?.message}</ErrorText>
        <div className="flex flex-wrap items-center gap-2">
          <Button type="submit" disabled={!dirty} loading={save.isPending}>
            Save settings
          </Button>
          <Button
            type="button"
            variant="ghost"
            disabled={isDefault}
            onClick={() => data && setForm(data.defaults)}
          >
            Reset to defaults
          </Button>
          {save.isSuccess && !dirty && <span className="text-sm text-success">Saved.</span>}
        </div>
        <p className="text-xs text-muted">
          Stored in <span className="font-mono">{data?.file}</span>
        </p>
      </form>
    </Card>
  );
}

function Setting({ label, hint, children }: { label: string; hint: string; children: ReactNode }) {
  return (
    <div className="grid gap-1.5 sm:grid-cols-[14rem_1fr] sm:items-start sm:gap-4">
      <div>
        <p className="text-sm font-medium">{label}</p>
        <p className="text-xs text-muted">{hint}</p>
      </div>
      <div className="max-w-xs">{children}</div>
    </div>
  );
}
