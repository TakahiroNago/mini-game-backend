import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api, errorMessage } from "../lib/api";
import { Badge, Button, Card, Empty, ErrorBanner, Input, Loading, Select, Table, formatDate, formatNumber } from "../components/ui";

export default function UserDetailPage() {
  const id = Number(useParams().id);
  const queryClient = useQueryClient();
  const detail = useQuery({ queryKey: ["user", id], queryFn: () => api.user(id), enabled: Number.isFinite(id) && id > 0 });
  const items = useQuery({ queryKey: ["items", false], queryFn: () => api.items(false) });

  const [itemId, setItemId] = useState<number>(0);
  const [quantity, setQuantity] = useState(1);
  const [notice, setNotice] = useState<string | null>(null);

  const grant = useMutation({
    mutationFn: (playerId: number) => api.grantItem(playerId, itemId, quantity),
    onSuccess: (res) => {
      setNotice(`アイテム #${res.item_id} を ${res.quantity} 個付与しました（取引 #${res.transaction_id}）`);
      void queryClient.invalidateQueries({ queryKey: ["user", id] });
      void queryClient.invalidateQueries({ queryKey: ["audit-logs"] });
    },
  });

  if (!Number.isFinite(id) || id <= 0) return <ErrorBanner message="不正なユーザー ID です" />;
  if (detail.isPending) return <Loading />;
  if (detail.isError) return <ErrorBanner message={errorMessage(detail.error)} />;

  const { user, inventory, recent_transactions, reward_claims } = detail.data;
  const player = user.player;

  const submitGrant = (e: FormEvent) => {
    e.preventDefault();
    if (!player || !itemId) return;
    setNotice(null);
    grant.mutate(player.id);
  };

  return (
    <div className="space-y-6">
      <div>
        <Link to="/users" className="text-sm text-indigo-700 hover:underline">
          ← ユーザー一覧
        </Link>
        <h1 className="mt-1 text-xl font-bold text-slate-900">{user.email}</h1>
        <p className="text-sm text-slate-500">
          ユーザー #{user.id} · <Badge tone={user.role === "admin" ? "indigo" : "slate"}>{user.role}</Badge> · 登録 {formatDate(user.created_at)}
        </p>
      </div>

      {!player ? (
        <Card title="プレイヤー">
          <Empty label="このアカウントにはプレイヤーがいません（管理者）" />
        </Card>
      ) : (
        <>
          <div className="grid gap-4 md:grid-cols-2">
            <Card title="プレイヤー">
              <dl className="grid grid-cols-2 gap-y-2 text-sm">
                <dt className="text-slate-500">プレイヤー名</dt>
                <dd className="font-medium">{player.player_name}</dd>
                <dt className="text-slate-500">プレイヤー ID</dt>
                <dd className="tabular-nums">{player.id}</dd>
                <dt className="text-slate-500">レベル</dt>
                <dd className="tabular-nums">{player.level}</dd>
                <dt className="text-slate-500">経験値（レベル内）</dt>
                <dd className="tabular-nums">
                  {formatNumber(player.experience)} / {formatNumber(player.level * 100)}
                </dd>
                <dt className="text-slate-500">コイン</dt>
                <dd className="tabular-nums">{formatNumber(player.coins)}</dd>
                <dt className="text-slate-500">デイリー報酬受取回数</dt>
                <dd className="tabular-nums">{reward_claims}</dd>
              </dl>
            </Card>
            <Card title="アイテム付与">
              <form onSubmit={submitGrant} className="space-y-3">
                <Select label="アイテム" value={itemId} onChange={(e) => setItemId(Number(e.target.value))} required>
                  <option value={0}>選択してください</option>
                  {items.data?.items.map((it) => (
                    <option key={it.id} value={it.id}>
                      #{it.id} {it.name}（{it.item_type} +{it.effect_value}）
                    </option>
                  ))}
                </Select>
                <Input label="個数" type="number" min={1} max={999} value={quantity} onChange={(e) => setQuantity(Number(e.target.value))} />
                <ErrorBanner message={grant.isError ? errorMessage(grant.error) : null} />
                {notice && <p className="rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{notice}</p>}
                <Button type="submit" disabled={grant.isPending || !itemId}>
                  {grant.isPending ? "付与中…" : "付与する"}
                </Button>
                <p className="text-xs text-slate-500">付与は item_transactions（acquire）と監査ログに記録されます。</p>
              </form>
            </Card>
          </div>

          <Card title="所持アイテム">
            {inventory.length === 0 ? (
              <Empty label="所持アイテムはありません" />
            ) : (
              <Table
                head={
                  <>
                    <th>ID</th>
                    <th>名前</th>
                    <th>種類</th>
                    <th className="text-right">効果</th>
                    <th className="text-right">所持数</th>
                  </>
                }
              >
                {inventory.map((it) => (
                  <tr key={it.item_id}>
                    <td className="tabular-nums">{it.item_id}</td>
                    <td>{it.name}</td>
                    <td>
                      <Badge tone={it.item_type === "coin" ? "amber" : "green"}>{it.item_type}</Badge>
                    </td>
                    <td className="text-right tabular-nums">+{formatNumber(it.effect_value)}</td>
                    <td className="text-right tabular-nums">{it.quantity}</td>
                  </tr>
                ))}
              </Table>
            )}
          </Card>

          <Card title="最近の取引（最新 20 件）">
            {recent_transactions.length === 0 ? (
              <Empty label="取引はありません" />
            ) : (
              <Table
                head={
                  <>
                    <th>取引 ID</th>
                    <th>種別</th>
                    <th>アイテム</th>
                    <th className="text-right">数量</th>
                    <th>Idempotency-Key</th>
                    <th>日時</th>
                  </>
                }
              >
                {recent_transactions.map((t) => (
                  <tr key={t.id}>
                    <td className="tabular-nums">{t.id}</td>
                    <td>
                      <Badge tone={t.transaction_type === "use" ? "rose" : "green"}>{t.transaction_type}</Badge>
                    </td>
                    <td>
                      #{t.item_id} {t.item_name}
                    </td>
                    <td className="text-right tabular-nums">{t.quantity}</td>
                    <td className="font-mono text-xs text-slate-500">{t.idempotency_key ?? "—"}</td>
                    <td className="whitespace-nowrap text-slate-500">{formatDate(t.created_at)}</td>
                  </tr>
                ))}
              </Table>
            )}
          </Card>
        </>
      )}
    </div>
  );
}
