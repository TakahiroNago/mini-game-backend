# データベース設計

MySQL 8.4 / InnoDB / utf8mb4 / 時刻は UTC で保存（`reward_date` のみ JST 基準の DATE）。

## ER 図

```
users 1 ──── 1 players 1 ──── * player_items * ──── 1 items
                   │
                   ├──── * reward_claims
                   │
                   └──── * item_transactions * ──── 1 items

users 1 ──── * audit_logs            (admin の操作履歴)
users 1 ──── * revoked_tokens        (ログアウト済み JWT, Day 2 で追加)
```

## テーブル定義

### users

| 列 | 型 | 制約 |
|---|---|---|
| id | BIGINT UNSIGNED | PK, AUTO_INCREMENT |
| email | VARCHAR(255) | NOT NULL, UNIQUE |
| password_hash | VARCHAR(255) | NOT NULL（bcrypt） |
| role | ENUM('player','admin') | NOT NULL DEFAULT 'player' |
| created_at / updated_at | DATETIME(3) | NOT NULL |

### players

| 列 | 型 | 制約 |
|---|---|---|
| id | BIGINT UNSIGNED | PK |
| user_id | BIGINT UNSIGNED | NOT NULL, UNIQUE, FK users(id) ON DELETE CASCADE |
| player_name | VARCHAR(32) | NOT NULL, UNIQUE |
| level | INT | NOT NULL DEFAULT 1, CHECK >= 1 |
| experience | BIGINT | NOT NULL DEFAULT 0, CHECK >= 0 |
| coins | BIGINT | NOT NULL DEFAULT 0, CHECK >= 0 |
| created_at / updated_at | DATETIME(3) | |

インデックス: `idx_players_ranking (level DESC, experience DESC, id ASC)` — ランキング ORDER BY と完全一致させ filesort を避ける。

**レベルアップ規則**: `experience` は「現在レベル内の経験値」。必要経験値 = `level * 100`。経験値獲得時に `experience >= level*100` の間 `experience -= level*100; level++` を繰り返す（`player` パッケージの純粋関数として単体テスト）。

### items

| 列 | 型 | 制約 |
|---|---|---|
| id | BIGINT UNSIGNED | PK |
| name | VARCHAR(64) | NOT NULL |
| description | VARCHAR(255) | NOT NULL DEFAULT '' |
| item_type | ENUM('coin','experience') | NOT NULL — 使用時にどのステータスへ効果を与えるか |
| effect_value | INT | NOT NULL, CHECK > 0 |
| starter_quantity | INT | NOT NULL DEFAULT 0, CHECK >= 0 — 登録時に新規プレイヤーへ付与する個数（0 = 初期アイテムではない）。000003 で追加 |
| deleted_at | DATETIME(3) | NULL — **論理削除**。履歴 (`item_transactions`) から参照されるため物理削除しない |
| created_at / updated_at | DATETIME(3) | |

### player_items（所持数）

| 列 | 型 | 制約 |
|---|---|---|
| id | BIGINT UNSIGNED | PK |
| player_id | BIGINT UNSIGNED | FK players(id) ON DELETE CASCADE |
| item_id | BIGINT UNSIGNED | FK items(id) |
| quantity | INT | NOT NULL DEFAULT 0, **CHECK (quantity >= 0)** |
| created_at / updated_at | DATETIME(3) | |

UNIQUE `(player_id, item_id)` — 1 プレイヤー 1 アイテム 1 行。増減は `INSERT ... ON DUPLICATE KEY UPDATE quantity = quantity + ?` と条件付き `UPDATE ... WHERE quantity >= ?`。

### reward_claims（デイリー報酬）

| 列 | 型 | 制約 |
|---|---|---|
| id | BIGINT UNSIGNED | PK |
| player_id | BIGINT UNSIGNED | FK players(id) ON DELETE CASCADE |
| reward_date | DATE | NOT NULL（JST 日付） |
| coins | INT | NOT NULL |
| experience | INT | NOT NULL |
| created_at | DATETIME(3) | |

**UNIQUE `(player_id, reward_date)`** — 二重取得防止の本体。100 並行でも INSERT が成功するのは 1 件だけ。

### item_transactions（取得・使用履歴）

| 列 | 型 | 制約 |
|---|---|---|
| id | BIGINT UNSIGNED | PK |
| player_id | BIGINT UNSIGNED | FK players(id) ON DELETE CASCADE |
| item_id | BIGINT UNSIGNED | FK items(id) |
| quantity | INT | NOT NULL, CHECK > 0 |
| transaction_type | ENUM('acquire','use') | NOT NULL |
| idempotency_key | VARCHAR(64) | NULL |
| created_at | DATETIME(3) | |

UNIQUE `(player_id, idempotency_key)` — MySQL では NULL は UNIQUE の対象外なので、キー無しの取得履歴（報酬・初期付与）は複数行入る。
INDEX `(player_id, created_at)` — 履歴一覧用。

### audit_logs（管理操作の監査）

| 列 | 型 |
|---|---|
| id | BIGINT UNSIGNED PK |
| actor_user_id | BIGINT UNSIGNED FK users(id) |
| action | VARCHAR(64)（例: `item.create`, `item.update`, `item.delete`） |
| target_type | VARCHAR(32) |
| target_id | BIGINT UNSIGNED NULL |
| detail | JSON NULL（変更前後の値） |
| created_at | DATETIME(3) |

INDEX `(actor_user_id, created_at)`, `(created_at)`。

## トランザクション設計

### ロック順序の原則（最重要）

**プレイヤーを変更するトランザクションは、必ず最初に `SELECT ... FROM players WHERE id = ? FOR UPDATE` で players 行を X ロックする。**
アイテム使用・デイリー報酬・（将来の）管理者による付与はすべてこの順序に従い、1 プレイヤーに対する変更はこのロックで直列化される。

当初は「UNIQUE 制約のある INSERT を先に行い、重複なら players 行をロックする前に失敗させる」設計だったが、
結合テスト（50 並行のアイテム使用）で **InnoDB のデッドロック (1213) が多発**した。原因は外部キー制約:
`item_transactions.player_id` / `reward_claims.player_id` への INSERT は親行 `players` に **共有 (S) ロック**を取る。
2 つのトランザクションが共に S ロックを持ったまま後段の `FOR UPDATE` で X ロックを要求し合うと循環待ちになる。
players 行を最初に X ロックすれば、後続の FK チェックは自分の X ロックと両立するため、この循環は起きない。

保険として `database.WithTxRetry` が 1213 を最大 3 回まで再実行するが、上記の順序を守る限り実際には発火しない。

### デイリー報酬 `POST /rewards/claim`

```
BEGIN
  SELECT level, experience, coins FROM players WHERE id = ? FOR UPDATE   -- 1. 直列化
  INSERT INTO reward_claims (player_id, reward_date, coins, experience) VALUES (?, ?, 100, 50)
      -- 2. 1062 Duplicate entry → ROLLBACK → 409 REWARD_ALREADY_CLAIMED
  (アプリでレベルアップ計算)
  UPDATE players SET level=?, experience=?, coins=coins+100 WHERE id = ?
COMMIT
```

`reward_date` は `Asia/Tokyo` 固定オフセット (+09:00) で切り出した日付文字列。アプリ側では「取得済みか」を事前判定せず、UNIQUE 違反だけを正とする。

### アイテム使用 `POST /items/use`

```
BEGIN
  SELECT level, experience, coins FROM players WHERE id = ? FOR UPDATE   -- 1. 直列化
  SELECT ... FROM items WHERE id = ? AND deleted_at IS NULL              -- 2. 使用可能か → 無ければ 404
  INSERT INTO item_transactions (player_id, item_id, quantity, 'use', idempotency_key)
      -- 3. 1062 → ROLLBACK → 冪等再送: 保存済みトランザクションを返す（200, replayed=true）
      --    同じキーで item_id / quantity が異なれば 422 IDEMPOTENCY_KEY_MISMATCH
  UPDATE player_items SET quantity = quantity - ?
      WHERE player_id = ? AND item_id = ? AND quantity >= ?               -- 4. 条件付き UPDATE
      -- RowsAffected = 0 → ROLLBACK（台帳行も消える）→ 409 INSUFFICIENT_ITEM
  UPDATE players SET ...                                                  -- 5. 効果反映
COMMIT
```

### 初期アイテム付与（登録時）

`POST /auth/register` のトランザクション内で、`items.starter_quantity > 0` かつ未削除のアイテムを
`player_items` に INSERT（`ON DUPLICATE KEY UPDATE`）し、`item_transactions` に `acquire`（キー NULL）で記録する。
users / players / 初期インベントリは全て揃うか、全て無いかのどちらかになる。

## マイグレーション

`migrations/NNNNNN_name.up.sql` / `.down.sql`。golang-migrate をライブラリとして埋め込み、

| # | 内容 |
|---|---|
| 000001 | 初期スキーマ（users, players, items, player_items, reward_claims, item_transactions, audit_logs） |
| 000002 | revoked_tokens（ログアウト） |
| 000003 | `items.starter_quantity` 追加 + 初期マスタ 4 件（Small/Large Coin Pouch, Experience Potion, Elixir of Wisdom） |


- `go run ./cmd/migrate -cmd up`
- API 起動時 `AUTO_MIGRATE=true` で自動適用（開発用）

で実行する。
