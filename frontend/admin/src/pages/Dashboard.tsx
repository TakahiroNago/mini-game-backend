import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api, errorMessage } from "../lib/api";
import { Card, Empty, ErrorBanner, Loading, StatTile, Table, formatNumber } from "../components/ui";

export default function DashboardPage() {
  const q = useQuery({ queryKey: ["dashboard"], queryFn: api.dashboard, refetchInterval: 30_000 });

  if (q.isPending) return <Loading />;
  if (q.isError) return <ErrorBanner message={errorMessage(q.error)} />;
  const d = q.data;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900">ダッシュボード</h1>
        <p className="text-sm text-slate-500">本日（JST {d.reward_date}）の状況。30 秒ごとに更新。</p>
      </div>
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        <StatTile label="ユーザー" value={formatNumber(d.users)} hint={`プレイヤー ${formatNumber(d.players)}`} />
        <StatTile label="アイテム（有効）" value={formatNumber(d.items)} />
        <StatTile label="本日の登録" value={formatNumber(d.registrations_today)} />
        <StatTile label="本日の報酬受取" value={formatNumber(d.reward_claims_today)} />
        <StatTile label="本日のアイテム使用" value={formatNumber(d.item_uses_today)} />
      </div>
      <Card title="ランキング上位">
        {d.top_players.length === 0 ? (
          <Empty label="プレイヤーがいません" />
        ) : (
          <Table
            head={
              <>
                <th>順位</th>
                <th>プレイヤー</th>
                <th className="text-right">Lv</th>
                <th className="text-right">経験値</th>
              </>
            }
          >
            {d.top_players.map((p) => (
              <tr key={p.player_id}>
                <td className="tabular-nums">{p.rank}</td>
                <td>
                  <Link to={`/users?q=${encodeURIComponent(p.player_name)}`} className="text-indigo-700 hover:underline">
                    {p.player_name}
                  </Link>
                </td>
                <td className="text-right tabular-nums">{p.level}</td>
                <td className="text-right tabular-nums">{formatNumber(p.experience)}</td>
              </tr>
            ))}
          </Table>
        )}
      </Card>
    </div>
  );
}
