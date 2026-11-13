import type {
  AuditPage,
  AuthUser,
  Dashboard,
  GrantResult,
  Item,
  ItemInput,
  LoginResponse,
  UserDetail,
  UsersPage,
} from "./types";

const TOKEN_KEY = "minigame.admin.token";
const USER_KEY = "minigame.admin.user";

/** Error envelope returned by every API endpoint. */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly details?: Record<string, string>,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export const session = {
  token: (): string | null => localStorage.getItem(TOKEN_KEY),
  user: (): AuthUser | null => {
    const raw = localStorage.getItem(USER_KEY);
    return raw ? (JSON.parse(raw) as AuthUser) : null;
  },
  save(token: string, user: AuthUser) {
    localStorage.setItem(TOKEN_KEY, token);
    localStorage.setItem(USER_KEY, JSON.stringify(user));
  },
  clear() {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
  },
};

/** Fired when the API answers 401 so the auth provider can log out. */
export const UNAUTHORIZED_EVENT = "minigame:unauthorized";

async function request<T>(method: string, path: string, body?: unknown, extraHeaders?: Record<string, string>): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json", ...extraHeaders };
  const token = session.token();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";

  const res = await fetch(`/api/v1${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const json = text ? (JSON.parse(text) as unknown) : null;
  if (!res.ok) {
    const err = (json as { error?: { code: string; message: string; details?: Record<string, string> } } | null)?.error;
    if (res.status === 401 && token) {
      window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
    }
    throw new ApiError(res.status, err?.code ?? "HTTP_ERROR", err?.message ?? res.statusText, err?.details);
  }
  return json as T;
}

export const api = {
  login: (email: string, password: string) => request<LoginResponse>("POST", "/auth/login", { email, password }),
  logout: () => request<void>("POST", "/auth/logout"),
  me: () => request<AuthUser>("GET", "/auth/me"),

  dashboard: () => request<Dashboard>("GET", "/admin/dashboard"),
  users: (params: { q?: string; limit: number; offset: number }) => {
    const qs = new URLSearchParams({ limit: String(params.limit), offset: String(params.offset) });
    if (params.q) qs.set("q", params.q);
    return request<UsersPage>("GET", `/admin/users?${qs}`);
  },
  user: (id: number) => request<UserDetail>("GET", `/admin/users/${id}`),
  items: (includeDeleted: boolean) => request<{ items: Item[] }>("GET", `/admin/items?include_deleted=${includeDeleted}`),
  createItem: (input: ItemInput) => request<Item>("POST", "/admin/items", input),
  updateItem: (id: number, input: ItemInput) => request<Item>("PUT", `/admin/items/${id}`, input),
  deleteItem: (id: number) => request<void>("DELETE", `/admin/items/${id}`),
  grantItem: (playerId: number, itemId: number, quantity: number) =>
    request<GrantResult>("POST", `/admin/players/${playerId}/items`, { item_id: itemId, quantity }),
  auditLogs: (params: { limit: number; offset: number }) =>
    request<AuditPage>("GET", `/admin/audit-logs?limit=${params.limit}&offset=${params.offset}`),
};

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    const details = err.details ? Object.entries(err.details).map(([k, v]) => `${k}: ${v}`).join(", ") : "";
    return details ? `${err.message} (${details})` : `${err.message} [${err.code}]`;
  }
  if (err instanceof Error) return err.message;
  return String(err);
}
