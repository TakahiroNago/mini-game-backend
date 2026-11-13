import { useState, type FormEvent } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { api, errorMessage } from "../lib/api";
import { Badge, Button, Card, Empty, ErrorBanner, Input, Loading, Pagination, Table, formatDate, formatNumber } from "../components/ui";

const LIMIT = 20;

export default function UsersPage() {
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const offset = Number(params.get("offset") ?? 0) || 0;
  const [draft, setDraft] = useState(q);

  const users = useQuery({
    queryKey: ["users", q, offset],
    queryFn: () => api.users({ q, limit: LIMIT, offset }),
    placeholderData: keepPreviousData,
  });

  const search = (e: FormEvent) => {
    e.preventDefault();
    setParams(draft ? { q: draft } : {});
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-xl font-bold text-slate-900">ユーザー</h1>
          <p className="text-sm text-slate-500">登録アカウントとプレイヤー情報</p>
        </div>
        <form onSubmit={search} className="flex items-end gap-2">
          <Input placeholder="メール / プレイヤー名で検索" value={draft} onChange={(e) => setDraft(e.target.value)} className="w-64" />
          <Button type="submit" variant="secondary">
            検索
          </Button>
        </form>
      </div>
      <Card>
        {users.isPending ? (
          <Loading />
        ) : users.isError ? (
          <ErrorBanner message={errorMessage(users.error)} />
        ) : users.data.users.length === 0 ? (
          <Empty label="該当するユーザーがいません" />
        ) : (
          <>
            <Table
              head={
                <>
                  <th>ID</th>
                  <th>メール</th>
                  <th>ロール</th>
                  <th>プレイヤー</th>
                  <th className="text-right">Lv</th>
                  <th className="text-right">経験値</th>
                  <th className="text-right">コイン</th>
                  <th>登録日時</th>
                </>
              }
            >
              {users.data.users.map((u) => (
                <tr key={u.id} className="hover:bg-slate-50">
                  <td className="tabular-nums">{u.id}</td>
                  <td>
                    <Link to={`/users/${u.id}`} className="text-indigo-700 hover:underline">
                      {u.email}
                    </Link>
                  </td>
                  <td>
                    <Badge tone={u.role === "admin" ? "indigo" : "slate"}>{u.role}</Badge>
                  </td>
                  <td>{u.player?.player_name ?? <span className="text-slate-400">—</span>}</td>
                  <td className="text-right tabular-nums">{u.player?.level ?? "—"}</td>
                  <td className="text-right tabular-nums">{u.player ? formatNumber(u.player.experience) : "—"}</td>
                  <td className="text-right tabular-nums">{u.player ? formatNumber(u.player.coins) : "—"}</td>
                  <td className="whitespace-nowrap text-slate-500">{formatDate(u.created_at)}</td>
                </tr>
              ))}
            </Table>
            <Pagination
              limit={LIMIT}
              offset={offset}
              total={users.data.total}
              onChange={(o) => setParams(q ? { q, offset: String(o) } : { offset: String(o) })}
            />
          </>
        )}
      </Card>
    </div>
  );
}
