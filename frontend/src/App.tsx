import { Navigate, Outlet, Route, Routes, useLocation } from "react-router";
import { useAuthStatus } from "./api/hooks";
import { AppShell } from "./components/AppShell";
import { FullPageMessage, Spinner } from "./components/ui";
import { CollectionsPage } from "./pages/CollectionsPage";
import { DashboardPage } from "./pages/DashboardPage";
import { LibraryPage } from "./pages/LibraryPage";
import { LoginPage } from "./pages/LoginPage";
import { PlaceholderPage } from "./pages/PlaceholderPage";
import { QueuePage } from "./pages/QueuePage";
import { SettingsPage } from "./pages/SettingsPage";
import { SetupPage } from "./pages/SetupPage";
import { SourcesPage } from "./pages/SourcesPage";
import { TagsPage } from "./pages/TagsPage";

export default function App() {
  return (
    <Routes>
      <Route element={<AuthGate mode="public" />}>
        <Route path="/setup" element={<SetupPage />} />
        <Route path="/login" element={<LoginPage />} />
      </Route>
      <Route element={<AuthGate mode="private" />}>
        <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="library" element={<LibraryPage />} />
          <Route path="sources" element={<SourcesPage />} />
          <Route
            path="duplicates"
            element={
              <PlaceholderPage
                title="Duplicates"
                milestone={4}
                text="Exact copies, identical geometry and likely duplicates, with a suggested keeper."
              />
            }
          />
          <Route path="collections" element={<CollectionsPage />} />
          <Route path="tags" element={<TagsPage />} />
          <Route path="queue" element={<QueuePage />} />
          <Route path="settings" element={<SettingsPage />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

/**
 * Routes by auth state: first run → /setup, signed out → /login,
 * signed in → the app (and away from /login and /setup).
 */
function AuthGate({ mode }: { mode: "public" | "private" }) {
  const { data, isPending, isError, error, refetch } = useAuthStatus();
  const location = useLocation();

  if (isPending) {
    return (
      <FullPageMessage>
        <Spinner />
      </FullPageMessage>
    );
  }
  if (isError) {
    return (
      <FullPageMessage>
        <p className="text-danger">Can't reach the server: {error.message}</p>
        <button className="mt-3 text-sm text-accent underline" onClick={() => refetch()}>
          Try again
        </button>
      </FullPageMessage>
    );
  }

  if (data.setup_required) {
    return location.pathname === "/setup" ? <Outlet /> : <Navigate to="/setup" replace />;
  }
  if (mode === "private" && !data.user) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }
  if (mode === "public" && (data.user || location.pathname === "/setup")) {
    return <Navigate to="/" replace />;
  }
  return <Outlet />;
}
