import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../lib/auth";
import { Button } from "./ui";

const nav = [
  { to: "/", label: "ダッシュボード", end: true },
  { to: "/users", label: "ユーザー" },
  { to: "/items", label: "アイテム" },
  { to: "/audit-logs", label: "監査ログ" },
];

export default function Layout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-56 shrink-0 flex-col border-r border-slate-200 bg-white md:flex">
        <div className="px-5 py-5">
          <p className="text-lg font-bold text-slate-900">Mini Game</p>
          <p className="text-xs text-slate-500">運営管理画面</p>
        </div>
        <nav className="flex flex-1 flex-col gap-1 px-3">
          {nav.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.end}
              className={({ isActive }) =>
                `rounded-lg px-3 py-2 text-sm font-medium ${isActive ? "bg-indigo-50 text-indigo-700" : "text-slate-600 hover:bg-slate-50"}`
              }
            >
              {n.label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between gap-4 border-b border-slate-200 bg-white px-4 py-3 md:px-8">
          <nav className="flex gap-3 text-sm md:hidden">
            {nav.map((n) => (
              <NavLink key={n.to} to={n.to} end={n.end} className={({ isActive }) => (isActive ? "font-semibold text-indigo-700" : "text-slate-600")}>
                {n.label}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-3 text-sm text-slate-600">
            <span className="hidden sm:inline">{user?.email}</span>
            <Button
              variant="secondary"
              onClick={async () => {
                await logout();
                navigate("/login", { replace: true });
              }}
            >
              ログアウト
            </Button>
          </div>
        </header>
        <main className="flex-1 px-4 py-6 md:px-8">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
