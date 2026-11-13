import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, errorMessage } from "../lib/api";
import type { Item, ItemInput, ItemType } from "../lib/types";
import { Badge, Button, Card, Empty, ErrorBanner, Input, Loading, Modal, Select, Table, formatDate, formatNumber } from "../components/ui";

const emptyInput: ItemInput = { name: "", description: "", item_type: "coin", effect_value: 100, starter_quantity: 0 };

export default function ItemsPage() {
  const queryClient = useQueryClient();
  const [includeDeleted, setIncludeDeleted] = useState(false);
  const [editing, setEditing] = useState<{ id: number | null; input: ItemInput } | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<Item | null>(null);

  const items = useQuery({ queryKey: ["items", includeDeleted], queryFn: () => api.items(includeDeleted) });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["items"] });
    void queryClient.invalidateQueries({ queryKey: ["audit-logs"] });
    void queryClient.invalidateQueries({ queryKey: ["dashboard"] });
  };

  const save = useMutation({
    mutationFn: ({ id, input }: { id: number | null; input: ItemInput }) => (id === null ? api.createItem(input) : api.updateItem(id, input)),
    onSuccess: () => {
      setEditing(null);
      invalidate();
    },
  });

  const remove = useMutation({
    mutationFn: (id: number) => api.deleteItem(id),
    onSuccess: () => {
      setConfirmDelete(null);
      invalidate();
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!editing) return;
    save.mutate(editing);
  };

  const field = <K extends keyof ItemInput>(key: K, value: ItemInput[K]) =>
    setEditing((cur) => (cur ? { ...cur, input: { ...cur.input, [key]: value } } : cur));

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-xl font-bold text-slate-900">アイテム</h1>
          <p className="text-sm text-slate-500">マスタデータ。削除は論理削除で、履歴からの参照は維持されます。</p>
        </div>
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-sm text-slate-600">
            <input type="checkbox" checked={includeDeleted} onChange={(e) => setIncludeDeleted(e.target.checked)} />
            削除済みも表示
          </label>
          <Button onClick={() => setEditing({ id: null, input: emptyInput })}>新規作成</Button>
        </div>
      </div>

      <Card>
        {items.isPending ? (
          <Loading />
        ) : items.isError ? (
          <ErrorBanner message={errorMessage(items.error)} />
        ) : items.data.items.length === 0 ? (
          <Empty label="アイテムがありません" />
        ) : (
          <Table
            head={
              <>
                <th>ID</th>
                <th>名前</th>
                <th>種類</th>
                <th className="text-right">効果</th>
                <th className="text-right">初期付与</th>
                <th>状態</th>
                <th>更新日時</th>
                <th></th>
              </>
            }
          >
            {items.data.items.map((it) => (
              <tr key={it.id} className={it.deleted_at ? "text-slate-400" : ""}>
                <td className="tabular-nums">{it.id}</td>
                <td>
                  <div className="font-medium">{it.name}</div>
                  <div className="text-xs text-slate-500">{it.description}</div>
                </td>
                <td>
                  <Badge tone={it.item_type === "coin" ? "amber" : "green"}>{it.item_type}</Badge>
                </td>
                <td className="text-right tabular-nums">+{formatNumber(it.effect_value)}</td>
                <td className="text-right tabular-nums">{it.starter_quantity}</td>
                <td>{it.deleted_at ? <Badge tone="rose">削除済み</Badge> : <Badge tone="green">有効</Badge>}</td>
                <td className="whitespace-nowrap text-slate-500">{formatDate(it.updated_at)}</td>
                <td className="whitespace-nowrap text-right">
                  {!it.deleted_at && (
                    <div className="flex justify-end gap-2">
                      <Button
                        variant="secondary"
                        onClick={() =>
                          setEditing({
                            id: it.id,
                            input: {
                              name: it.name,
                              description: it.description,
                              item_type: it.item_type,
                              effect_value: it.effect_value,
                              starter_quantity: it.starter_quantity,
                            },
                          })
                        }
                      >
                        編集
                      </Button>
                      <Button variant="danger" onClick={() => setConfirmDelete(it)}>
                        削除
                      </Button>
                    </div>
                  )}
                </td>
              </tr>
            ))}
          </Table>
        )}
      </Card>

      {editing && (
        <Modal title={editing.id === null ? "アイテムを作成" : `アイテム #${editing.id} を編集`} onClose={() => setEditing(null)}>
          <form onSubmit={submit} className="space-y-3">
            <Input label="名前" required maxLength={64} value={editing.input.name} onChange={(e) => field("name", e.target.value)} />
            <Input label="説明" maxLength={255} value={editing.input.description} onChange={(e) => field("description", e.target.value)} />
            <div className="grid grid-cols-3 gap-3">
              <Select label="種類" value={editing.input.item_type} onChange={(e) => field("item_type", e.target.value as ItemType)}>
                <option value="coin">coin</option>
                <option value="experience">experience</option>
              </Select>
              <Input label="効果値" type="number" min={1} max={1_000_000} required value={editing.input.effect_value} onChange={(e) => field("effect_value", Number(e.target.value))} />
              <Input label="初期付与数" type="number" min={0} max={99} value={editing.input.starter_quantity} onChange={(e) => field("starter_quantity", Number(e.target.value))} />
            </div>
            <ErrorBanner message={save.isError ? errorMessage(save.error) : null} />
            <div className="flex justify-end gap-2 pt-2">
              <Button variant="secondary" onClick={() => setEditing(null)}>
                キャンセル
              </Button>
              <Button type="submit" disabled={save.isPending}>
                {save.isPending ? "保存中…" : "保存"}
              </Button>
            </div>
          </form>
        </Modal>
      )}

      {confirmDelete && (
        <Modal title={`アイテム #${confirmDelete.id} を削除`} onClose={() => setConfirmDelete(null)}>
          <p className="text-sm text-slate-600">
            「{confirmDelete.name}」をプレイヤーから見えなくします。所持数や取引履歴はそのまま残ります。
          </p>
          <ErrorBanner message={remove.isError ? errorMessage(remove.error) : null} />
          <div className="flex justify-end gap-2 pt-4">
            <Button variant="secondary" onClick={() => setConfirmDelete(null)}>
              キャンセル
            </Button>
            <Button variant="danger" disabled={remove.isPending} onClick={() => remove.mutate(confirmDelete.id)}>
              削除する
            </Button>
          </div>
        </Modal>
      )}
    </div>
  );
}
