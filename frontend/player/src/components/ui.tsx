import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from "react";

export function Panel({ title, actions, children, className = "" }: { title?: ReactNode; actions?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`rounded-2xl border border-slate-800 bg-slate-900 ${className}`}>
      {(title || actions) && (
        <header className="flex items-center justify-between gap-4 border-b border-slate-800 px-5 py-3">
          <h2 className="text-sm font-semibold text-slate-200">{title}</h2>
          <div className="flex items-center gap-2 text-xs text-slate-400">{actions}</div>
        </header>
      )}
      <div className="p-5">{children}</div>
    </section>
  );
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "ghost" };

export function Button({ variant = "primary", className = "", ...props }: ButtonProps) {
  const styles = {
    primary: "bg-amber-400 text-slate-950 hover:bg-amber-300 disabled:bg-slate-700 disabled:text-slate-400",
    secondary: "border border-slate-600 bg-slate-800 text-slate-100 hover:bg-slate-700 disabled:text-slate-500",
    ghost: "text-slate-300 hover:bg-slate-800 disabled:text-slate-600",
  }[variant];
  return (
    <button
      type="button"
      className={`inline-flex items-center justify-center rounded-lg px-3 py-2 text-sm font-semibold transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-amber-300 disabled:cursor-not-allowed ${styles} ${className}`}
      {...props}
    />
  );
}

export function Input({ label, hint, className = "", ...props }: InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: string }) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block font-medium text-slate-300">{label}</span>
      <input
        className={`w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-slate-100 outline-none focus:border-amber-400 focus:ring-1 focus:ring-amber-400 ${className}`}
        {...props}
      />
      {hint && <span className="mt-1 block text-xs text-slate-500">{hint}</span>}
    </label>
  );
}

export function ErrorBanner({ message }: { message: string | null | undefined }) {
  if (!message) return null;
  return (
    <div role="alert" className="rounded-lg border border-rose-500/40 bg-rose-950/60 px-4 py-3 text-sm text-rose-200">
      {message}
    </div>
  );
}

export function Loading({ label = "読み込み中…" }: { label?: string }) {
  return <p className="py-4 text-center text-sm text-slate-500">{label}</p>;
}

export function formatNumber(n: number): string {
  return n.toLocaleString("ja-JP");
}
