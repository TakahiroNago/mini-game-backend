import type {
  AuthUser,
  ClaimResult,
  InventoryItem,
  LoginResponse,
  MyRank,
  Player,
  RankingPage,
  RewardStatus,
  UseItemResult,
} from "./types";

const TOKEN_KEY = "minigame.player.token";
const USER_KEY = "minigame.player.user";

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

/**
 * One entry of the on-screen API log. Request and response bodies are never
 * recorded, so credentials and tokens cannot end up in it.
 */
export interface ApiCall {
  id: number;
  method: string;
  path: string;
  /** 0 when the request never reached the server. */
  status: number;
  code?: string;
  ms: number;
  requestId: string | null;
  rateRemaining: string | null;
  idempotencyKey?: string;
  replayed?: boolean;
}

const MAX_LOG = 30;
let calls: ApiCall[] = [];
let nextCallId = 1;
const listeners = new Set<() => void>();

function record(call: Omit<ApiCall, "id">) {
  calls = [{ id: nextCallId++, ...call }, ...calls].slice(0, MAX_LOG);
  listeners.forEach((l) => l());
}

/** External store read with useSyncExternalStore. */
export const apiLog = {
  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },
  snapshot: () => calls,
  clear() {
    calls = [];
    listeners.forEach((l) => l());
  },
};

async function request<T>(method: string, path: string, body?: unknown, extraHeaders?: Record<string, string>): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json", ...extraHeaders };
  const token = session.token();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const idempotencyKey = extraHeaders?.["Idempotency-Key"];

  const started = performance.now();
  let res: Response;
  try {
    res = await fetch(`/api/v1${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (err) {
    record({ method, path, status: 0, code: "NETWORK_ERROR", ms: Math.round(performance.now() - started), requestId: null, rateRemaining: null, idempotencyKey });
    throw err;
  }

  const text = res.status === 204 ? "" : await res.text();
  let json: unknown = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    // A proxy error page (502 while the API restarts) is not JSON.
  }
  const err = (json as { error?: { code: string; message: string; details?: Record<string, string> } } | null)?.error;
  record({
    method,
    path,
    status: res.status,
    code: res.ok ? undefined : (err?.code ?? "HTTP_ERROR"),
    ms: Math.round(performance.now() - started),
    requestId: res.headers.get("X-Request-Id"),
    rateRemaining: res.headers.get("X-RateLimit-Remaining"),
    idempotencyKey,
    replayed: (json as { replayed?: boolean } | null)?.replayed,
  });

  if (!res.ok) {
    if (res.status === 401 && token) {
      window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
    }
    throw new ApiError(res.status, err?.code ?? "HTTP_ERROR", err?.message ?? res.statusText, err?.details);
  }
  return json as T;
}

export const api = {
  register: (email: string, password: string, playerName: string) =>
    request<{ user_id: number; player_id: number }>("POST", "/auth/register", { email, password, player_name: playerName }),
  login: (email: string, password: string) => request<LoginResponse>("POST", "/auth/login", { email, password }),
  logout: () => request<void>("POST", "/auth/logout"),

  player: () => request<Player>("GET", "/player/me"),
  inventory: () => request<{ items: InventoryItem[] }>("GET", "/player/me/items"),
  /** The caller owns the key: sending the same one again must replay, not consume twice. */
  useItem: (itemId: number, quantity: number, idempotencyKey: string) =>
    request<UseItemResult>("POST", "/items/use", { item_id: itemId, quantity }, { "Idempotency-Key": idempotencyKey }),

  rewardStatus: () => request<RewardStatus>("GET", "/rewards/status"),
  claimReward: () => request<ClaimResult>("POST", "/rewards/claim"),

  ranking: (limit: number) => request<RankingPage>("GET", `/ranking?limit=${limit}&offset=0`),
  myRank: () => request<MyRank>("GET", "/ranking/me"),
};

const friendly: Record<string, string> = {
  INVALID_CREDENTIALS: "メールアドレスまたはパスワードが違います",
  EMAIL_ALREADY_EXISTS: "このメールアドレスは登録済みです",
  PLAYER_NAME_ALREADY_EXISTS: "このプレイヤー名は使われています",
  INSUFFICIENT_ITEM: "アイテムの所持数が足りません",
  REWARD_ALREADY_CLAIMED: "本日のデイリー報酬は受け取り済みです",
  ITEM_NOT_FOUND: "このアイテムは現在使用できません",
  RATE_LIMITED: "リクエストが多すぎます。少し待ってからもう一度お試しください",
  NETWORK_ERROR: "サーバーに接続できません",
};

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.details) {
      return Object.entries(err.details)
        .map(([k, v]) => `${k}: ${v}`)
        .join(", ");
    }
    return `${friendly[err.code] ?? err.message} [${err.code}]`;
  }
  if (err instanceof TypeError) return friendly.NETWORK_ERROR;
  if (err instanceof Error) return err.message;
  return String(err);
}
