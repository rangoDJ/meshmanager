import {
  Copy,
  FolderHeart,
  HardDrive,
  LayoutDashboard,
  Library,
  ListTodo,
  LogOut,
  Menu,
  Settings,
  Tags,
  X,
} from "lucide-react";
import { useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router";
import { useAuthStatus, useLogout } from "../api/hooks";

const nav = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/library", label: "Library", icon: Library },
  { to: "/sources", label: "Sources", icon: HardDrive },
  { to: "/duplicates", label: "Duplicates", icon: Copy },
  { to: "/collections", label: "Collections", icon: FolderHeart },
  { to: "/tags", label: "Tags", icon: Tags },
  { to: "/queue", label: "Print queue", icon: ListTodo },
  { to: "/settings", label: "Settings", icon: Settings },
];

export function AppShell() {
  const [open, setOpen] = useState(false);
  const { data } = useAuthStatus();
  const logout = useLogout();
  // Pages built around a grid or a board use the full width; the rest read better narrower.
  const path = useLocation().pathname;
  const wide = ["/library", "/collections", "/queue"].some((p) => path.startsWith(p));

  return (
    <div className="flex min-h-full">
      {open && (
        <div className="fixed inset-0 z-30 bg-black/40 md:hidden" onClick={() => setOpen(false)} />
      )}
      <aside
        className={`fixed inset-y-0 left-0 z-40 flex w-60 flex-col border-r border-border bg-surface transition-transform print:hidden md:static md:translate-x-0 ${
          open ? "translate-x-0" : "-translate-x-full"
        }`}
      >
        <div className="flex h-14 items-center gap-2.5 px-4">
          <img src="/favicon.svg" alt="" className="size-6" />
          <span className="font-semibold">MeshManager</span>
          <button
            className="ml-auto rounded p-1 text-muted md:hidden"
            onClick={() => setOpen(false)}
            aria-label="Close menu"
          >
            <X className="size-5" />
          </button>
        </div>
        <nav className="flex-1 space-y-0.5 px-2 py-2">
          {nav.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              onClick={() => setOpen(false)}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition ${
                  isActive
                    ? "bg-accent/10 font-medium text-accent"
                    : "text-muted hover:bg-surface-2 hover:text-fg"
                }`
              }
            >
              <Icon className="size-4" aria-hidden />
              {label}
            </NavLink>
          ))}
        </nav>
        <div className="flex items-center gap-2 border-t border-border p-3">
          <div className="flex size-8 items-center justify-center rounded-full bg-accent/15 text-sm font-semibold text-accent">
            {data?.user?.username.slice(0, 1).toUpperCase()}
          </div>
          <span className="min-w-0 flex-1 truncate text-sm">{data?.user?.username}</span>
          <button
            onClick={() => logout.mutate()}
            className="rounded-lg p-2 text-muted hover:bg-surface-2 hover:text-fg"
            title="Sign out"
            aria-label="Sign out"
          >
            <LogOut className="size-4" />
          </button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center gap-3 border-b border-border bg-surface px-4 md:hidden print:hidden">
          <button onClick={() => setOpen(true)} className="rounded p-1 text-muted" aria-label="Open menu">
            <Menu className="size-5" />
          </button>
          <span className="font-semibold">MeshManager</span>
        </header>
        <main className={`mx-auto w-full flex-1 p-4 md:p-6 print:p-0 ${wide ? "" : "max-w-5xl md:p-8"}`}>
          <Outlet />
        </main>
      </div>
    </div>
  );
}
