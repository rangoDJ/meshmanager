import { useState, type FormEvent } from "react";
import { useLogin } from "../api/hooks";
import { AuthCard, Button, ErrorText, Field } from "../components/ui";

export function LoginPage() {
  const login = useLogin();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  function submit(e: FormEvent) {
    e.preventDefault();
    login.mutate({ username, password });
  }

  return (
    <AuthCard title="Sign in" subtitle="to your MeshManager library">
      <form onSubmit={submit} className="space-y-4">
        <Field
          label="Username"
          name="username"
          autoComplete="username"
          autoFocus
          required
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <Field
          label="Password"
          name="password"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <ErrorText>{login.error?.message}</ErrorText>
        <Button type="submit" className="w-full" loading={login.isPending}>
          Sign in
        </Button>
      </form>
    </AuthCard>
  );
}
