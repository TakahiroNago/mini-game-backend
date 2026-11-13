# API 仕様

ベース URL: `http://localhost:8080`（Compose では `API_HOST_PORT`、管理画面経由なら `http://localhost:5173/api/...`）。
すべて JSON（`Content-Type: application/json; charset=utf-8`）。時刻は RFC 3339 の UTC、`reward_date` だけ JST の `YYYY-MM-DD`。

## 共通事項

### 認証

`Authorization: Bearer <access_token>`。トークンは `POST /api/v1/auth/login` が発行する JWT（HS256、有効期限は `JWT_EXPIRY`、既定 1 時間）。
`role` は `player` か `admin`。`/api/v1/admin/*` は `admin` のみ。

### エラー形式

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "request validation failed",
    "details": { "email": "must be a valid email address" }
  }
}
```

`code` は機械判定用の定数、`message` は人間向け、`details` はフィールド単位のバリデーション結果（ある場合のみ）。
内部エラーは常に `500 INTERNAL_ERROR` の固定文言で、原因はサーバーログ（`request_id` 付き）にだけ残る。

| HTTP | code | 発生条件 |
|---|---|---|
| 400 | `BAD_REQUEST` | ボディが空 / JSON として不正 / 未知のフィールド / 型違い / 末尾に余分なデータ |
| 400 | `VALIDATION_ERROR` | 構造体タグのバリデーション失敗（`details` あり）、パス ID が正の整数でない、`limit` / `offset` / `q` が範囲外 |
| 400 | `IDEMPOTENCY_KEY_REQUIRED` | `POST /items/use` に `Idempotency-Key` がない、または形式不正 |
| 401 | `UNAUTHORIZED` | `Authorization` ヘッダなし / 形式不正 / 署名不正 / `alg` 不一致 |
| 401 | `TOKEN_EXPIRED` | `exp` 超過 |
| 401 | `TOKEN_REVOKED` | ログアウト済みの `jti` |
| 401 | `INVALID_CREDENTIALS` | ログイン失敗（メール不存在とパスワード不一致を区別しない） |
| 403 | `FORBIDDEN` | ロール不足（player が admin ルートへ） |
| 404 | `NOT_FOUND` | ルートが存在しない |
| 404 | `PLAYER_NOT_FOUND` / `USER_NOT_FOUND` / `ITEM_NOT_FOUND` | 対象が存在しない（アイテムは論理削除済みも含む） |
| 405 | `METHOD_NOT_ALLOWED` | メソッド不一致 |
| 409 | `EMAIL_ALREADY_EXISTS` / `PLAYER_NAME_ALREADY_EXISTS` | 登録時の重複 |
| 409 | `INSUFFICIENT_ITEM` | 所持数不足 |
| 409 | `REWARD_ALREADY_CLAIMED` | 本日分は取得済み |
| 413 | `PAYLOAD_TOO_LARGE` | ボディが `MAX_BODY_BYTES`（既定 1 MiB）超 |
| 422 | `IDEMPOTENCY_KEY_MISMATCH` | 同じ `Idempotency-Key` を別の内容で再利用 |
| 429 | `RATE_LIMITED` | IP ごとの予算超過。`Retry-After` と `X-RateLimit-*` を返す |
| 500 | `INTERNAL_ERROR` | 予期しない失敗 |
| 503 | – | `/ready` が MySQL に到達できない |

### 共通レスポンスヘッダ

`X-Request-Id`（ログ照合用）、`X-RateLimit-Limit` / `X-RateLimit-Remaining` / `X-RateLimit-Reset`（`/api/v1/*`）、
`Cache-Control: no-store`、`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Content-Security-Policy: default-src 'none'`。

### ページネーション

`?limit=`（1〜100、既定 20）と `?offset=`（0 以上、既定 0）。レスポンスは `limit` / `offset` / `total` を同梱する。

---

## 運用エンドポイント

### `GET /health`

プロセス稼働確認。依存先は見ない。

```json
{ "status": "ok", "version": "dev" }
```

### `GET /ready`

MySQL への `Ping` を含む準備確認。失敗時は 503。

```json
{ "status": "ready", "checks": { "mysql": "ok" } }
```

### `GET /metrics`

Prometheus テキスト形式。認証なし・レート制限なし（ネットワーク層で保護する前提）。指標の一覧は README の Observability 節。

---

## 認証 `/api/v1/auth`

### `POST /register`

ユーザーとプレイヤーを同一トランザクションで作成し、`items.starter_quantity > 0` のアイテムを初期付与する。

| フィールド | 型 | 制約 |
|---|---|---|
| `email` | string | 必須、メール形式、255 文字以下。小文字に正規化して保存 |
| `password` | string | 必須、8〜72 文字（bcrypt の上限） |
| `player_name` | string | 必須、2〜32 文字、英数字（Unicode 含む）のみ、ユニーク |

```http
POST /api/v1/auth/register
{"email":"hero@example.com","password":"password123","player_name":"hero"}

201 Created
{"user_id":1,"player_id":1}
```

エラー: `VALIDATION_ERROR`、`EMAIL_ALREADY_EXISTS`、`PLAYER_NAME_ALREADY_EXISTS`（どちらの 409 でも行は残らない）。レート制限は `AUTH_RATE_LIMIT_PER_MINUTE`。

### `POST /login`

```http
POST /api/v1/auth/login
{"email":"hero@example.com","password":"password123"}

200 OK
{
  "access_token": "<jwt>",
  "token_type": "Bearer",
  "expires_at": "2026-11-13T10:00:00Z",
  "user": { "id": 1, "email": "hero@example.com", "role": "player", "created_at": "2026-11-13T09:00:00Z" }
}
```

エラー: `INVALID_CREDENTIALS`（メール不存在時もダミーハッシュと比較して応答時間を揃える）。レート制限は `AUTH_RATE_LIMIT_PER_MINUTE`。

### `POST /logout` （Bearer）

トークンの `jti` を失効表へ登録する。以後そのトークンは `TOKEN_REVOKED`。`204 No Content`。

### `GET /me` （Bearer）

```json
{ "id": 1, "email": "hero@example.com", "role": "player", "created_at": "2026-11-13T09:00:00Z" }
```

---

## プレイヤー `/api/v1/player` （Bearer）

### `GET /me`

```json
{
  "id": 1, "user_id": 1, "player_name": "hero",
  "level": 1, "experience": 0, "coins": 0,
  "created_at": "2026-11-13T09:00:00Z", "updated_at": "2026-11-13T09:00:00Z"
}
```

`experience` は「現在レベル内の進捗」。レベル `L` から `L+1` に必要な経験値は `L × 100`（`level` の上限は 999）。
admin アカウントにはプレイヤー行がないため `404 PLAYER_NOT_FOUND`。

### `GET /me/items`

```json
{
  "items": [
    { "item_id": 1, "name": "Small Coin Pouch", "description": "...", "item_type": "coin", "effect_value": 100, "quantity": 3 },
    { "item_id": 3, "name": "Experience Potion", "description": "...", "item_type": "experience", "effect_value": 50, "quantity": 3 }
  ]
}
```

所持数 0 のアイテムは含まない。

---

## アイテム `/api/v1/items` （Bearer）

### `GET /`

利用可能なカタログ（論理削除済みを除く）。

```json
{
  "items": [
    { "id": 1, "name": "Small Coin Pouch", "description": "...", "item_type": "coin", "effect_value": 100,
      "starter_quantity": 3, "created_at": "...", "updated_at": "..." }
  ]
}
```

`item_type` は `coin`（`coins += effect_value × quantity`）か `experience`（`experience += effect_value × quantity`、レベルアップ処理込み）。

### `POST /use`

**`Idempotency-Key` ヘッダ必須**（1〜64 文字、`A-Z a-z 0-9 _ . : -`。UUID 推奨）。

| フィールド | 型 | 制約 |
|---|---|---|
| `item_id` | int | 必須、正の整数 |
| `quantity` | int | 1〜99、省略時 1 |

```http
POST /api/v1/items/use
Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000
{"item_id":3,"quantity":2}

200 OK
{
  "transaction_id": 12, "item_id": 3, "quantity": 2, "replayed": false,
  "player": { "level": 2, "experience": 0, "coins": 0 }
}
```

- 処理は 1 トランザクション: players 行を `FOR UPDATE` → アイテム確認 → 台帳 INSERT（`(player_id, idempotency_key)` UNIQUE）→ `UPDATE player_items ... WHERE quantity >= ?` → 効果適用 → players 更新。
- **再送**（同じキー・同じ `item_id` / `quantity`）: `200` で保存済みの `transaction_id` を返し `replayed: true`。消費は起きない。`player` は**現在の**進捗。
- 同じキーで内容が違う: `422 IDEMPOTENCY_KEY_MISMATCH`。
- 失敗した試行（`409 INSUFFICIENT_ITEM`、`404 ITEM_NOT_FOUND`）は台帳行ごとロールバックされるので、同じキーをやり直しに使える。
- キーのスコープはプレイヤー単位。他人のキーと衝突しない。

---

## デイリー報酬 `/api/v1/rewards` （Bearer）

### `POST /claim`

JST の暦日ごとに 1 回。`reward_claims (player_id, reward_date)` の UNIQUE 制約で保証し、2 回目は `409 REWARD_ALREADY_CLAIMED`。
報酬は固定（コイン 100、経験値 50）。

```json
{
  "claim_id": 7, "reward_date": "2026-11-13", "coins": 100, "experience": 50,
  "player": { "level": 1, "experience": 50, "coins": 100 }
}
```

### `GET /status`

```json
{ "reward_date": "2026-11-13", "claimed": false, "coins": 100, "experience": 50 }
```

---

## ランキング `/api/v1/ranking` （Bearer）

### `GET /`

`level DESC, experience DESC, id ASC` の安定ソート（`idx_players_ranking` をそのまま使う）。`?limit=` / `?offset=`。

```json
{
  "rankings": [
    { "rank": 1, "player_id": 5, "player_name": "alice", "level": 3, "experience": 20 },
    { "rank": 2, "player_id": 1, "player_name": "hero",  "level": 2, "experience": 0 }
  ],
  "limit": 20, "offset": 0, "total": 2
}
```

### `GET /me`

```json
{ "rank": 2, "player_id": 1, "player_name": "hero", "level": 2, "experience": 0, "total": 2 }
```

---

## 管理 `/api/v1/admin` （Bearer, role=admin）

player ロールは全ルートで `403 FORBIDDEN`。更新系は監査ログ（`audit_logs`）を**同一トランザクション**で記録する。

### `GET /dashboard`

「本日」は JST。

```json
{
  "users": 12, "players": 11, "items": 4,
  "registrations_today": 2, "reward_claims_today": 5, "item_uses_today": 9,
  "reward_date": "2026-11-13",
  "top_players": [ { "rank": 1, "player_id": 5, "player_name": "alice", "level": 3, "experience": 20 } ]
}
```

### `GET /users`

`?q=`（255 文字以下）でメールまたはプレイヤー名の部分一致、`?limit=` / `?offset=`。新しい順。

```json
{
  "users": [
    { "id": 2, "email": "hero@example.com", "role": "player", "created_at": "...",
      "player": { "id": 1, "player_name": "hero", "level": 2, "experience": 0, "coins": 100 } },
    { "id": 1, "email": "admin@example.com", "role": "admin", "created_at": "...", "player": null }
  ],
  "limit": 20, "offset": 0, "total": 2
}
```

### `GET /users/{id}`

```json
{
  "user": { "id": 2, "email": "hero@example.com", "role": "player", "created_at": "...", "player": { "...": "..." } },
  "inventory": [ { "item_id": 1, "name": "Small Coin Pouch", "item_type": "coin", "effect_value": 100, "quantity": 3, "description": "" } ],
  "recent_transactions": [
    { "id": 12, "item_id": 3, "item_name": "Experience Potion", "quantity": 2, "transaction_type": "use",
      "idempotency_key": "550e8400-...", "created_at": "..." },
    { "id": 1, "item_id": 1, "item_name": "Small Coin Pouch", "quantity": 3, "transaction_type": "acquire",
      "idempotency_key": null, "created_at": "..." }
  ],
  "reward_claims": 1
}
```

エラー: `404 USER_NOT_FOUND`。

### `GET /items`

`?include_deleted=true` で論理削除済み（`deleted_at` あり）も含める。レスポンスは `{"items":[...]}`。

### `POST /items` → 201 / `PUT /items/{id}` → 200

| フィールド | 型 | 制約 |
|---|---|---|
| `name` | string | 必須、1〜64 文字（前後の空白は除去） |
| `description` | string | 255 文字以下 |
| `item_type` | string | `coin` / `experience` |
| `effect_value` | int | 1〜1,000,000 |
| `starter_quantity` | int | 0〜99（登録時に自動付与する個数） |

レスポンスはアイテム 1 件（`GET /items` と同じ形）。`PUT` は削除済みアイテムに対して `404 ITEM_NOT_FOUND`。
監査ログ: `item.create`（`after`）、`item.update`（`before` / `after`）。

### `DELETE /items/{id}` → 204

論理削除。既存の所持・台帳は残り、カタログと `POST /items/use` から消える。監査ログ `item.delete`（`before`）。

### `POST /players/{id}/items`

| フィールド | 型 | 制約 |
|---|---|---|
| `item_id` | int | 必須 |
| `quantity` | int | 1〜999 |

```json
{ "player_id": 1, "item_id": 2, "quantity": 5, "transaction_id": 13 }
```

players 行の `FOR UPDATE` → `acquire` 取引の INSERT（`idempotency_key` は null）→ `player_items` の UPSERT → 監査ログ `player.grant_item`。
エラー: `404 PLAYER_NOT_FOUND`、`404 ITEM_NOT_FOUND`。

### `GET /audit-logs`

新しい順。`?limit=` / `?offset=`。

```json
{
  "logs": [
    { "id": 3, "actor_user_id": 1, "actor_email": "admin@example.com",
      "action": "player.grant_item", "target_type": "player", "target_id": 1,
      "detail": { "item_id": 2, "quantity": 5, "transaction_id": 13 },
      "created_at": "..." }
  ],
  "limit": 20, "offset": 0, "total": 3
}
```

`action` は `item.create` / `item.update` / `item.delete` / `player.grant_item`。`detail` は操作ごとの JSON（before / after など）。
