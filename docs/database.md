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

### デイリー報酬 `POST /rewards/claim`

```
BEGIN
  INSERT INTO reward_claims (player_id, reward_date, coins, experience) VALUES (?, ?, 100, 50)
      -- 1062 Duplicate entry → ROLLBACK → 409 REWARD_ALREADY_CLAIMED
  SELECT level, experience, coins FROM players WHERE id = ? FOR UPDATE
  (アプリでレベルアップ計算)
  UPDATE players SET level=?, experience=?, coins=coins+100 WHERE id = ?
COMMIT
```

INSERT を先に行うのがポイント。UNIQUE 違反で即座に失敗し、players 行ロックを取る前に終わるため競合コストが最小になる。

### アイテム使用 `POST /items/use`

```
BEGIN
  (Idempotency-Key が既存なら → 既存トランザクションを返して終了)
  SELECT ... FROM items WHERE id = ? AND deleted_at IS NULL           -- 使用可能か
  UPDATE player_items SET quantity = quantity - ?
      WHERE player_id = ? AND item_id = ? AND quantity >= ?            -- 条件付き UPDATE
      -- RowsAffected = 0 → ROLLBACK → 409 INSUFFICIENT_ITEM
  SELECT level, experience, coins FROM players WHERE id = ? FOR UPDATE
  UPDATE players SET ...（効果反映）
  INSERT INTO item_transactions (..., 'use', idempotency_key)
      -- 1062 → ROLLBACK → 冪等再送として既存結果を返す
COMMIT
```

## マイグレーション

`migrations/NNNNNN_name.up.sql` / `.down.sql`。golang-migrate をライブラリとして埋め込み、

- `go run ./cmd/migrate -cmd up`
- API 起動時 `AUTO_MIGRATE=true` で自動適用（開発用）

で実行する。
