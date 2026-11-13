import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, errorMessage } from "../lib/api";
import { Badge, Card, Empty, ErrorBanner, Loading, Pagination, Table, formatDate } from "../components/ui";

const LIMIT = 20;

function actionTone(action: string): "green" | "amber" | "rose" | "indigo" | "slate" {
  if (action.endsWith(".create")) return "green";
  if (action.endsWith(".update")) return "amber";
  if (action.endsWith(".delete")) return "rose";
  if (action.includes("grant")) return "indigo";
  return "slate";
}

export default function AuditLogsPage() {
  const [offset, setOffset] = useState(0);
  const [open, setOpen] = useState<number | null>(null);
  const logs = useQuery({
    queryKey: ["audit-logs", offset],
    queryFn: () => api.auditLogs({ limit: LIMIT, offset }),
    placeholderData: keepPreviousData,
  });

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900">監査ログ</h1>
        <p className="text-sm text-slate-500">管理 API による変更の記録。変更と同じトランザクションで書き込まれます。</p>
      </div>
      <Card>
        {logs.isPending ? (
          <Loading />
        ) : logs.isError ? (
          <ErrorBanner message={errorMessage(logs.error)} />
        ) : logs.data.logs.length === 0 ? (
          <Empty label="記録はまだありません" />
        ) : (
          <>
            <Table
              head={
                <>
                  <th>ID</th>
                  <th>日時</th>
                  <th>操作者</th>
                  <th>アクション</th>
                  <th>対象</th>
                  <th>詳細</th>
                </>
              }
            >
              {logs.data.logs.map((l) => (
                <tr key={l.id} className="align-top">
                  <td className="tabular-nums">{l.id}</td>
                  <td className="whitespace-nowrap text-slate-500">{formatDate(l.created_at)}</td>
                  <td>{l.actor_email}</td>
                  <td>
                    <Badge tone={actionTone(l.action)}>{l.action}</Badge>
                  </td>
                  <td className="whitespace-nowrap">
                    {l.target_type} #{l.target_id ?? "—"}
                  </td>
                  <td>
                    <button type="button" className="text-xs text-indigo-700 hover:underline" onClick={() => setOpen(open === l.id ? null : l.id)}>
                      {open === l.id ? "閉じる" : "表示"}
                    </button>
                    {open === l.id && (
                      <pre className="mt-2 max-w-xl overflow-x-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-100">{JSON.stringify(l.detail, null, 2)}</pre>
                    )}
                  </td>
                </tr>
              ))}
            </Table>
            <Pagination limit={LIMIT} offset={offset} total={logs.data.total} onChange={setOffset} />
          </>
        )}
      </Card>
    </div>
  );
}
