import { useState, type FormEvent } from "react";
import { useSetup } from "../api/hooks";
import { AuthCard, Button, ErrorText, Field } from "../components/ui";

export function SetupPage() {
  const setup = useSetup();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [localError, setLocalError] = useState("");

  function submit(e: FormEvent) {
    e.preventDefault();
    if (password !== confirm) {
      setLocalError("Passwords don't match.");
      return;
    }
    setLocalError("");
    setup.mutate({ username, password });
  }

  return (
    <AuthCard title="Welcome to MeshManager" subtitle="Create the admin account to get started.">
      <form onSubmit={submit} className="space-y-4">
        <Field
          label="Username"
          name="username"
          autoComplete="username"
          required
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <Field
          label="Password"
          name="password"
          type="password"
          autoComplete="new-password"
          required
          minLength={8}
          hint="At least 8 characters."
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <Field
          label="Confirm password"
          name="confirm"
          type="password"
          autoComplete="new-password"
          required
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
        />
        <ErrorText>{localError || setup.error?.message}</ErrorText>
        <Button type="submit" className="w-full" loading={setup.isPending}>
          Create account
        </Button>
      </form>
    </AuthCard>
  );
}
