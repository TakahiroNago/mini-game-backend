import { useState, type FormEvent } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../lib/auth";
import { errorMessage } from "../lib/api";
import { Button, ErrorBanner, Input } from "../components/ui";

// Set only by the public demo build (deploy/vps/demo.env). The demo admin
// account is public on purpose; a normal build shows none of this.
const demoEmail: string | undefined = import.meta.env.VITE_DEMO_ADMIN_EMAIL;
const demoPassword: string | undefined = import.meta.env.VITE_DEMO_ADMIN_PASSWORD;
const playerUrl: string | undefined = import.meta.env.VITE_PLAYER_URL;

export default function LoginPage() {
  const { user, login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (user) return <Navigate to="/" replace />;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await login(email, password);
      const from = (location.state as { from?: string } | null)?.from ?? "/";
      navigate(from, { replace: true });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <form onSubmit={submit} className="w-full max-w-sm space-y-4 rounded-xl border border-slate-200 bg-white p-8 shadow-sm">
        <div>
          <h1 className="text-xl font-bold text-slate-900">Mini Game 管理画面</h1>
          <p className="mt-1 text-sm text-slate-500">管理者アカウントでログインしてください</p>
        </div>
        {demoEmail && demoPassword && (
          <div className="space-y-2 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
            <p>公開デモ環境です。データは毎日 4:00（日本時間）にリセットされます。</p>
            <Button
              variant="secondary"
              className="w-full"
              onClick={() => {
                setEmail(demoEmail);
                setPassword(demoPassword);
              }}
            >
              デモ用の管理者アカウントを入力
            </Button>
            {playerUrl && (
              <a href={playerUrl} className="block text-center text-xs font-medium text-indigo-700 underline">
                プレイヤー画面はこちら
              </a>
            )}
          </div>
        )}
        <ErrorBanner message={error} />
        <Input label="メールアドレス" type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
        <Input label="パスワード" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        <Button type="submit" className="w-full" disabled={busy}>
          {busy ? "ログイン中…" : "ログイン"}
        </Button>
      </form>
    </div>
  );
}
