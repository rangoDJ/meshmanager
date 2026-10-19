import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { ApiError } from "./api/client";
import { authStatusKey } from "./api/hooks";
import type { AuthStatus } from "./api/types";
import App from "./App";
import "./index.css";

// Any 401 means the session ended (expired, signed out elsewhere): drop the user
// so the auth gate sends us to the login page.
function onError(error: Error) {
  if (error instanceof ApiError && error.status === 401) {
    queryClient.setQueryData<AuthStatus>(authStatusKey, (old) =>
      old ? { ...old, user: null } : old,
    );
  }
}

const queryClient = new QueryClient({
  queryCache: new QueryCache({ onError }),
  mutationCache: new MutationCache({ onError }),
  defaultOptions: {
    queries: {
      retry: (count, error) => !(error instanceof ApiError && error.status < 500) && count < 2,
      refetchOnWindowFocus: false,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
