import { useState, type FormEvent } from "react";
import { useAuthStatus, useChangePassword } from "../api/hooks";
import { Button, Card, ErrorText, Field } from "../components/ui";

export function SettingsPage() {
  const { data } = useAuthStatus();
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Settings</h1>
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
