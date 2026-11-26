import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, ApiError, session, UNAUTHORIZED_EVENT } from "./api";
import type { AuthUser } from "./types";

// Throwaway guest credentials, kept so a guest can come back after the JWT
// expires. They are random, never reused elsewhere and only open a demo account.
const GUEST_KEY = "minigame.player.guest";

interface GuestCredentials {
  email: string;
  password: string;
  playerName: string;
}

export function savedGuest(): GuestCredentials | null {
  try {
    const raw = localStorage.getItem(GUEST_KEY);
    return raw ? (JSON.parse(raw) as GuestCredentials) : null;
  } catch {
    return null;
  }
}

function randomString(length: number, alphabet: string): string {
  const bytes = crypto.getRandomValues(new Uint8Array(length));
  return Array.from(bytes, (b) => alphabet[b % alphabet.length]).join("");
}

interface AuthContextValue {
  user: AuthUser | null;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, playerName: string) => Promise<void>;
  /** Creates a random account and logs in: one click for a visitor. */
  startAsGuest: () => Promise<void>;
  resumeGuest: () => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(() => session.user());
  const queryClient = useQueryClient();

  const clear = useCallback(() => {
    session.clear();
    setUser(null);
    queryClient.clear();
  }, [queryClient]);

  // A 401 from any call (expired or revoked token) drops the session.
  useEffect(() => {
    window.addEventListener(UNAUTHORIZED_EVENT, clear);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, clear);
  }, [clear]);

  const login = useCallback(async (email: string, password: string) => {
    const res = await api.login(email, password);
    if (res.user.role !== "player") {
      // Admin accounts have no player row; revoke the token right away.
      session.save(res.access_token, res.user);
      await api.logout().catch(() => undefined);
      session.clear();
      throw new ApiError(403, "FORBIDDEN", "管理者アカウントは管理画面からログインしてください");
    }
    session.save(res.access_token, res.user);
    setUser(res.user);
  }, []);

  const register = useCallback(
    async (email: string, password: string, playerName: string) => {
      await api.register(email, password, playerName);
      await login(email, password);
    },
    [login],
  );

  const startAsGuest = useCallback(async () => {
    const guest: GuestCredentials = {
      email: `guest-${randomString(10, "abcdefghijklmnopqrstuvwxyz0123456789")}@example.com`,
      password: randomString(20, "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"),
      playerName: `Guest${randomString(6, "ABCDEFGHJKLMNPQRSTUVWXYZ23456789")}`,
    };
    await api.register(guest.email, guest.password, guest.playerName);
    localStorage.setItem(GUEST_KEY, JSON.stringify(guest));
    await login(guest.email, guest.password);
  }, [login]);

  const resumeGuest = useCallback(async () => {
    const guest = savedGuest();
    if (!guest) throw new Error("保存されたゲストアカウントがありません");
    try {
      await login(guest.email, guest.password);
    } catch (err) {
      // The demo database is reset periodically; a stale guest is gone for good.
      if (err instanceof ApiError && err.code === "INVALID_CREDENTIALS") {
        localStorage.removeItem(GUEST_KEY);
        throw new Error("前回のゲストアカウントはリセットされました。新しく始めてください");
      }
      throw err;
    }
  }, [login]);

  const logout = useCallback(async () => {
    await api.logout().catch(() => undefined);
    clear();
  }, [clear]);

  const value = useMemo(
    () => ({ user, login, register, startAsGuest, resumeGuest, logout }),
    [user, login, register, startAsGuest, resumeGuest, logout],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}
