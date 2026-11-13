import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, ApiError, session, UNAUTHORIZED_EVENT } from "./api";
import type { AuthUser } from "./types";

interface AuthContextValue {
  user: AuthUser | null;
  login: (email: string, password: string) => Promise<void>;
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
    if (res.user.role !== "admin") {
      // The token is valid but useless here; revoke it right away.
      session.save(res.access_token, res.user);
      await api.logout().catch(() => undefined);
      session.clear();
      throw new ApiError(403, "FORBIDDEN", "管理者アカウントでログインしてください");
    }
    session.save(res.access_token, res.user);
    setUser(res.user);
  }, []);

  const logout = useCallback(async () => {
    await api.logout().catch(() => undefined);
    clear();
  }, [clear]);

  const value = useMemo(() => ({ user, login, logout }), [user, login, logout]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}
