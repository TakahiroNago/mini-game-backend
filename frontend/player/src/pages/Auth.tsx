import { useState, type FormEvent } from "react";
import { savedGuest, useAuth } from "../lib/auth";
import { errorMessage } from "../lib/api";
import { Button, ErrorBanner, Input } from "../components/ui";

type Mode = "login" | "register";

// Set only by the public demo build (deploy/vps/demo.env).
const demoMode = import.meta.env.VITE_DEMO_MODE === "true";
const adminUrl: string | undefined = import.meta.env.VITE_ADMIN_URL;

export default function AuthPage() {
  const { login, register, startAsGuest, resumeGuest } = useAuth();
  const [mode, setMode] = useState<Mode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [playerName, setPlayerName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const guest = savedGuest();

  // On success the session changes and App swaps this page out, so busy only
  // needs resetting on failure.
  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await action();
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    void run(() => (mode === "login" ? login(email, password) : register(email, password, playerName)));
  };

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-sm space-y-5">
        <div className="text-center">
          <h1 className="text-3xl font-black tracking-tight text-amber-300">Mini Game</h1>
          <p className="mt-2 text-sm text-slate-400">報酬を受け取り、アイテムを使ってランキングを上げよう</p>
        </div>

        <div className="space-y-3 rounded-2xl border border-slate-800 bg-slate-900 p-6">
          <Button className="w-full py-3 text-base" disabled={busy} onClick={() => void run(startAsGuest)}>
            ゲストですぐに遊ぶ
          </Button>
          {guest && (
            <Button variant="secondary" className="w-full" disabled={busy} onClick={() => void run(resumeGuest)}>
              前回のゲスト（{guest.playerName}）で続ける
            </Button>
          )}
          <p className="text-center text-xs text-slate-500">登録不要。ランダムなアカウントが自動で作られます</p>
        </div>

        {demoMode && (
          <div className="rounded-2xl border border-amber-400/40 bg-amber-950/40 px-5 py-4 text-sm text-amber-100">
            <p className="font-semibold">公開デモ環境です</p>
            <ul className="mt-1 list-disc space-y-1 pl-5 text-xs text-amber-200/90">
              <li>データは毎日 4:00（日本時間）にリセットされます。</li>
              <li>登録したメールアドレスは管理画面で誰でも見られます。実在のメールアドレスや普段使うパスワードは入力しないでください。</li>
            </ul>
            {adminUrl && (
              <a href={adminUrl} className="mt-2 inline-block text-xs font-semibold text-amber-300 underline">
                運営用の管理画面を開く
              </a>
            )}
          </div>
        )}

        <form onSubmit={submit} className="space-y-4 rounded-2xl border border-slate-800 bg-slate-900 p-6">
          <div className="grid grid-cols-2 gap-1 rounded-lg bg-slate-950 p-1 text-sm font-semibold" role="tablist">
            {(["login", "register"] as const).map((m) => (
              <button
                key={m}
                type="button"
                role="tab"
                aria-selected={mode === m}
                onClick={() => {
                  setMode(m);
                  setError(null);
                }}
                className={`rounded-md py-1.5 transition ${mode === m ? "bg-slate-700 text-white" : "text-slate-400 hover:text-slate-200"}`}
              >
                {m === "login" ? "ログイン" : "新規登録"}
              </button>
            ))}
          </div>
          <ErrorBanner message={error} />
          <Input label="メールアドレス" type="email" autoComplete="username" required maxLength={255} value={email} onChange={(e) => setEmail(e.target.value)} />
          <Input
            label="パスワード"
            type="password"
            autoComplete={mode === "login" ? "current-password" : "new-password"}
            required
            minLength={8}
            maxLength={72}
            hint={mode === "register" ? "8〜72 文字" : undefined}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          {mode === "register" && (
            <Input
              label="プレイヤー名"
              required
              minLength={2}
              maxLength={32}
              hint="2〜32 文字、英数字のみ"
              value={playerName}
              onChange={(e) => setPlayerName(e.target.value)}
            />
          )}
          <Button type="submit" variant="secondary" className="w-full" disabled={busy}>
            {busy ? "処理中…" : mode === "login" ? "ログイン" : "登録して始める"}
          </Button>
        </form>
      </div>
    </div>
  );
}
