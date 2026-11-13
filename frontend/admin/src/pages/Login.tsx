import { useState, type FormEvent } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../lib/auth";
import { errorMessage } from "../lib/api";
import { Button, ErrorBanner, Input } from "../components/ui";

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
