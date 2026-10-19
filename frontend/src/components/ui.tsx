import { LoaderCircle } from "lucide-react";
import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from "react";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost";
  loading?: boolean;
};

export function Button({
  variant = "primary",
  loading,
  disabled,
  className = "",
  children,
  ...props
}: ButtonProps) {
  const styles = {
    primary: "bg-accent text-accent-fg hover:opacity-90",
    secondary: "border border-border bg-surface hover:bg-surface-2",
    ghost: "hover:bg-surface-2 text-muted hover:text-fg",
  }[variant];
  return (
    <button
      {...props}
      disabled={disabled || loading}
      className={`inline-flex items-center justify-center gap-2 rounded-lg px-3.5 py-2 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50 ${styles} ${className}`}
    >
      {loading && <LoaderCircle className="size-4 animate-spin" aria-hidden />}
      {children}
    </button>
  );
}

type FieldProps = InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: string };

export function Field({ label, hint, id, ...props }: FieldProps) {
  const inputId = id ?? props.name;
  return (
    <div className="space-y-1.5">
      <label htmlFor={inputId} className="block text-sm font-medium">
        {label}
      </label>
      <input
        id={inputId}
        {...props}
        className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none transition focus:border-accent focus:ring-2 focus:ring-accent/25"
      />
      {hint && <p className="text-xs text-muted">{hint}</p>}
    </div>
  );
}

export function Card({
  title,
  action,
  children,
  className = "",
}: {
  title?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`rounded-xl border border-border bg-surface p-5 ${className}`}>
      {(title || action) && (
        <header className="mb-4 flex items-center justify-between gap-3">
          <h2 className="font-semibold">{title}</h2>
          {action}
        </header>
      )}
      {children}
    </section>
  );
}

export function ErrorText({ children }: { children: ReactNode }) {
  if (!children) return null;
  return (
    <p role="alert" className="rounded-lg bg-danger/10 px-3 py-2 text-sm text-danger">
      {children}
    </p>
  );
}

export function Spinner() {
  return <LoaderCircle className="size-6 animate-spin text-muted" aria-label="Loading" />;
}

export function FullPageMessage({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-full flex-col items-center justify-center p-6 text-center">
      {children}
    </div>
  );
}

export function AuthCard({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-full items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 text-center">
          <img src="/favicon.svg" alt="" className="mx-auto mb-3 size-10" />
          <h1 className="text-xl font-semibold">{title}</h1>
          <p className="mt-1 text-sm text-muted">{subtitle}</p>
        </div>
        <div className="rounded-xl border border-border bg-surface p-6 shadow-sm">{children}</div>
      </div>
    </div>
  );
}
