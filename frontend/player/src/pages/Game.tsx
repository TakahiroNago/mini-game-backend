import { useState, useSyncExternalStore } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiLog, errorMessage } from "../lib/api";
import { useAuth } from "../lib/auth";
import type { InventoryItem, Player, Progress, UseItemResult } from "../lib/types";
import { Button, ErrorBanner, Loading, Panel, formatNumber } from "../components/ui";

const RANKING_LIMIT = 20;
// Set only by the public demo build (deploy/vps/demo.env).
const adminUrl: string | undefined = import.meta.env.VITE_ADMIN_URL;

/** What the last item use did, kept so the same request can be sent again. */
interface LastUse {
  item: InventoryItem;
  key: string;
  result: UseItemResult;
}

export default function GamePage() {
  const { logout } = useAuth();
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [lastUse, setLastUse] = useState<LastUse | null>(null);

  const player = useQuery({ queryKey: ["player"], queryFn: api.player });
  const inventory = useQuery({ queryKey: ["inventory"], queryFn: api.inventory });
  const reward = useQuery({ queryKey: ["reward"], queryFn: api.rewardStatus });
  const ranking = useQuery({ queryKey: ["ranking"], queryFn: () => api.ranking(RANKING_LIMIT) });
  const myRank = useQuery({ queryKey: ["myRank"], queryFn: api.myRank });

  // Every mutation changes the player row, so the server stays the source of
  // truth: refetch instead of patching the cache by hand.
  const applied = (before: Player | undefined, after: Progress, message: string) => {
    setError(null);
    setNotice(before && after.level > before.level ? `${message} レベル ${after.level} に上がりました！` : message);
    void queryClient.invalidateQueries();
  };

  const claim = useMutation({
    mutationFn: api.claimReward,
    onSuccess: (res) => applied(player.data, res.player, `デイリー報酬を受け取りました（コイン +${res.coins}、経験値 +${res.experience}）。`),
    onError: (err) => {
      setNotice(null);
      setError(errorMessage(err));
      void queryClient.invalidateQueries({ queryKey: ["reward"] });
    },
  });

  const useItem = useMutation({
    mutationFn: ({ item, key }: { item: InventoryItem; key: string }) => api.useItem(item.item_id, 1, key),
    onSuccess: (result, { item, key }) => {
      setLastUse({ item, key, result });
      applied(
        player.data,
        result.player,
        result.replayed ? `再送を検知しました。${item.name} は消費されていません。` : `${item.name} を使いました（${effectLabel(item)}）。`,
      );
    },
    onError: (err) => {
      setNotice(null);
      setError(errorMessage(err));
      void queryClient.invalidateQueries({ queryKey: ["inventory"] });
    },
  });

  const busy = claim.isPending || useItem.isPending;
  const items = inventory.data?.items ?? [];

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      <header className="mb-6 flex items-center justify-between gap-4">
        <h1 className="text-2xl font-black tracking-tight text-amber-300">Mini Game</h1>
        <div className="flex items-center gap-3 text-sm text-slate-300">
          {adminUrl && (
            <a href={adminUrl} className="text-xs text-amber-300 underline">
              管理画面
            </a>
          )}
          <span className="font-semibold">{player.data?.player_name}</span>
          <Button variant="secondary" onClick={() => void logout()}>
            ログアウト
          </Button>
        </div>
      </header>

      <div className="mb-4 space-y-3" aria-live="polite">
        <ErrorBanner message={error} />
        {notice && <div className="rounded-lg border border-emerald-500/40 bg-emerald-950/60 px-4 py-3 text-sm text-emerald-200">{notice}</div>}
      </div>

      <div className="grid gap-4 lg:grid-cols-5">
        <div className="space-y-4 lg:col-span-3">
          <Panel title="ステータス">
            {player.data ? <Status player={player.data} rank={myRank.data?.rank} total={myRank.data?.total} /> : <Loading />}
          </Panel>

          <Panel title="デイリー報酬" actions={reward.data && <span>{reward.data.reward_date}（日本時間）</span>}>
            {reward.data ? (
              <div className="flex flex-wrap items-center justify-between gap-4">
                <p className="text-sm text-slate-300">
                  コイン +{formatNumber(reward.data.coins)}、経験値 +{formatNumber(reward.data.experience)}。1 日 1 回受け取れます。
                </p>
                <Button disabled={busy || reward.data.claimed} onClick={() => claim.mutate()}>
                  {reward.data.claimed ? "受け取り済み" : "受け取る"}
                </Button>
              </div>
            ) : (
              <Loading />
            )}
          </Panel>

          <Panel title="所持アイテム">
            {!inventory.data ? (
              <Loading />
            ) : items.length === 0 ? (
              <p className="py-4 text-center text-sm text-slate-500">アイテムを使い切りました。運営（管理画面）からの付与を待ちましょう。</p>
            ) : (
              <ul className="divide-y divide-slate-800">
                {items.map((item) => (
                  <li key={item.item_id} className="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0">
                    <div className="min-w-0">
                      <p className="font-semibold text-slate-100">
                        {item.name} <span className="ml-1 tabular-nums text-slate-400">× {item.quantity}</span>
                      </p>
                      <p className="text-xs text-slate-400">{effectLabel(item)}</p>
                    </div>
                    <Button disabled={busy} onClick={() => useItem.mutate({ item, key: crypto.randomUUID() })}>
                      使う
                    </Button>
                  </li>
                ))}
              </ul>
            )}
            {lastUse && (
              <div className="mt-4 rounded-lg border border-slate-700 bg-slate-950 p-4 text-sm">
                <p className="text-slate-300">
                  直前の使用: {lastUse.item.name}（取引 ID <span className="tabular-nums">{lastUse.result.transaction_id}</span>、
                  <code className="text-xs">replayed: {String(lastUse.result.replayed)}</code>）
                </p>
                <p className="mt-1 break-all font-mono text-xs text-slate-500">Idempotency-Key: {lastUse.key}</p>
                <div className="mt-3 flex flex-wrap items-center gap-3">
                  <Button variant="secondary" disabled={busy} onClick={() => useItem.mutate({ item: lastUse.item, key: lastUse.key })}>
                    同じキーでもう一度送る
                  </Button>
                  <span className="text-xs text-slate-500">通信の再送を再現します。同じ取引 ID が返り、アイテムは減りません。</span>
                </div>
              </div>
            )}
          </Panel>
        </div>

        <div className="space-y-4 lg:col-span-2">
          <Panel title="ランキング" actions={ranking.data && <span>{formatNumber(ranking.data.total)} 人中 上位 {RANKING_LIMIT}</span>}>
            {ranking.data ? (
              <ol className="space-y-1 text-sm">
                {ranking.data.rankings.map((r) => {
                  const me = r.player_id === player.data?.id;
                  return (
                    <li key={r.player_id} className={`flex items-center gap-3 rounded-lg px-3 py-1.5 ${me ? "bg-amber-400/15 text-amber-200" : "text-slate-300"}`}>
                      <span className="w-6 text-right tabular-nums text-slate-500">{r.rank}</span>
                      <span className="min-w-0 flex-1 truncate font-medium">
                        {r.player_name}
                        {me && <span className="ml-2 text-xs">（あなた）</span>}
                      </span>
                      <span className="tabular-nums text-slate-400">
                        Lv {r.level} / {formatNumber(r.experience)} EXP
                      </span>
                    </li>
                  );
                })}
              </ol>
            ) : (
              <Loading />
            )}
          </Panel>

          <ApiLogPanel />
        </div>
      </div>
    </div>
  );
}

function effectLabel(item: InventoryItem): string {
  return item.item_type === "coin" ? `コイン +${formatNumber(item.effect_value)}` : `経験値 +${formatNumber(item.effect_value)}`;
}

function Status({ player, rank, total }: { player: Player; rank?: number; total?: number }) {
  // Experience is progress inside the current level; level L needs L × 100.
  const needed = player.level * 100;
  const percent = Math.min(100, Math.round((player.experience / needed) * 100));
  return (
    <div className="space-y-4">
      <dl className="grid grid-cols-3 gap-4 text-center">
        {[
          ["レベル", formatNumber(player.level)],
          ["コイン", formatNumber(player.coins)],
          ["順位", rank ? `${formatNumber(rank)} 位` : "–"],
        ].map(([label, value]) => (
          <div key={label} className="rounded-xl bg-slate-950 px-2 py-3">
            <dt className="text-xs text-slate-400">{label}</dt>
            <dd className="mt-1 text-2xl font-bold tabular-nums text-slate-50">{value}</dd>
          </div>
        ))}
      </dl>
      <div>
        <div className="mb-1 flex justify-between text-xs text-slate-400">
          <span>次のレベルまで</span>
          <span className="tabular-nums">
            {formatNumber(player.experience)} / {formatNumber(needed)} EXP
          </span>
        </div>
        <div
          className="h-3 overflow-hidden rounded-full bg-slate-800"
          role="progressbar"
          aria-label="経験値"
          aria-valuemin={0}
          aria-valuemax={needed}
          aria-valuenow={player.experience}
        >
          <div className="h-full rounded-full bg-amber-400 transition-[width] duration-500" style={{ width: `${percent}%` }} />
        </div>
        {total !== undefined && <p className="mt-2 text-xs text-slate-500">全 {formatNumber(total)} 人</p>}
      </div>
    </div>
  );
}

/** Shows what the browser actually sent, so the backend's behaviour is visible. */
function ApiLogPanel() {
  const calls = useSyncExternalStore(apiLog.subscribe, apiLog.snapshot);
  const remaining = calls.find((c) => c.rateRemaining !== null)?.rateRemaining;
  return (
    <Panel
      title="API ログ"
      actions={
        <>
          {remaining && <span>レート残り {remaining}</span>}
          <Button variant="ghost" className="px-2 py-1 text-xs" onClick={apiLog.clear}>
            消去
          </Button>
        </>
      }
    >
      {calls.length === 0 ? (
        <p className="py-2 text-center text-sm text-slate-500">まだリクエストがありません</p>
      ) : (
        <ul className="max-h-80 space-y-2 overflow-y-auto font-mono text-xs">
          {calls.map((c) => (
            <li key={c.id}>
              <div className="flex items-baseline gap-2">
                <span className={`font-bold tabular-nums ${c.status >= 200 && c.status < 300 ? "text-emerald-400" : "text-rose-400"}`}>{c.status || "ERR"}</span>
                <span className="text-slate-400">{c.method}</span>
                <span className="min-w-0 flex-1 truncate text-slate-200">{c.path}</span>
                <span className="tabular-nums text-slate-500">{c.ms} ms</span>
              </div>
              {(c.code || c.replayed !== undefined || c.requestId) && (
                <div className="truncate pl-10 text-slate-500">
                  {c.code && <span className="mr-2 text-rose-300">{c.code}</span>}
                  {c.replayed !== undefined && <span className={`mr-2 ${c.replayed ? "text-amber-300" : ""}`}>replayed={String(c.replayed)}</span>}
                  {c.requestId && <span>req {c.requestId}</span>}
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
