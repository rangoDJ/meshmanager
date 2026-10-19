export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

type Options = {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
};

/** Calls the JSON API. Throws ApiError with the server's message on non-2xx responses. */
export async function api<T>(path: string, { method = "GET", body }: Options = {}): Promise<T> {
  const res = await fetch(`/api${path}`, {
    method,
    credentials: "same-origin",
    headers: {
      // The server rejects state-changing requests without this header (CSRF guard).
      "X-Requested-With": "MeshManager",
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  if (res.status === 204) return undefined as T;

  const isJSON = res.headers.get("Content-Type")?.startsWith("application/json");
  const data = isJSON ? await res.json() : undefined;
  if (!res.ok) {
    throw new ApiError(res.status, data?.error ?? `Request failed (${res.status})`);
  }
  return data as T;
}
