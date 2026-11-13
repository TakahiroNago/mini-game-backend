# アーキテクチャ設計

## 1. 全体構成

```
                  ┌──────────────────────────────┐
                  │   React Admin (Vite, :5173)   │
                  │   TanStack Query / Tailwind   │
                  └──────────────┬───────────────┘
                                 │ HTTPS/JSON  (Bearer JWT, role=admin)
  Game Client (curl / 簡易画面)   │
          │                      │
          ▼                      ▼
┌──────────────────────────────────────────────────────────┐
│                     Go API (chi, :8080)                   │
│                                                          │
│  middleware: RequestID → RealIP → Logger(slog) →         │
│              Recoverer → Timeout → BodyLimit → CORS →    │
│              RateLimit → Metrics → Auth(JWT) → Admin     │
│                                                          │
│  /api/v1/auth      /api/v1/player   /api/v1/items        │
│  /api/v1/rewards   /api/v1/ranking  /api/v1/admin/*      │
│  /health  /ready  /metrics                               │
└───────────────┬───────────────────────────┬──────────────┘
                │ database/sql (tx, FOR UPDATE, UNIQUE)     │ /metrics scrape
                ▼                                           ▼
       ┌────────────────┐                         ┌────────────────┐
       │  MySQL 8.4     │                         │  Prometheus    │
       │  (InnoDB)      │                         │  (+Grafana)    │
       └────────────────┘                         └────────────────┘
```

すべて Docker Compose で起動する（`api` / `mysql` / `admin` / `prometheus`）。

## 2. 技術選定と理由

| 領域 | 選定 | 理由 |
|---|---|---|
| HTTP ルーター | chi v5 | `net/http` 互換・軽量・ミドルウェア合成が明快。フレームワーク固有の抽象を持ち込まない |
| DB アクセス | `database/sql` + `go-sql-driver/mysql` | SQL・トランザクション・ロックを**自分の手で書いて説明できる**ことが本プロジェクトの目的。sqlc は未導入（Windows ツールチェーン簡素化のため）。必要になれば後から導入可 |
| マイグレーション | golang-migrate（ライブラリとして埋め込み） | CLI を別途インストールせず `go run ./cmd/migrate` と API 起動時の `AUTO_MIGRATE` で実行できる。SQL ファイルがそのまま DB 設計書になる |
| 認証 | JWT (HS256, `golang-jwt/jwt/v5`) + bcrypt (cost 12) | ステートレスでスケールしやすい。ログアウトは `revoked_tokens(jti)` への登録で実現し、ミドルウェアが毎回参照する（有効期限 1h なので表は小さく保てる） |
| バリデーション | `go-playground/validator` | 構造体タグで宣言的に書ける標準的な選択 |
| ログ | `log/slog` JSON | 標準ライブラリ。request_id / status / latency を構造化出力 |
| メトリクス | `prometheus/client_golang` | `/metrics` 公開。ラベルは method / route pattern / status のみ（高カーディナリティ回避） |
| レート制限 | `go-chi/httprate` | chi 公式。IP ベースのインメモリ制限で MVP には十分 |

## 3. パッケージ構成（レイヤリング）

```
cmd/api/main.go          起動・DI・graceful shutdown
cmd/migrate/main.go      マイグレーション CLI (up / down / version / force)
internal/config          環境変数 → Config 構造体
internal/database        *sql.DB 生成、トランザクションヘルパー、migrate 実行
internal/httpx           JSON レスポンス / 統一エラー / リクエストデコード / バリデーション
internal/identity        認証済み主体（Claims）と context ヘルパー。依存ゼロで循環 import を防ぐ
internal/middleware      ログ・リカバリ・認証・認可・レート制限・メトリクス
internal/health          /health, /ready
internal/server          ルーター組み立て（全ハンドラの登録）
internal/auth            登録・ログイン・JWT 発行/検証
internal/player          プレイヤー情報・所持アイテム
internal/item            アイテム一覧・使用（トランザクション）
internal/reward          デイリー報酬（UNIQUE 制約）
internal/ranking         ランキング
internal/admin           管理 API + 監査ログ
internal/observability   Prometheus メトリクス定義
migrations/              *.sql（embed して配布）
tests/                   結合・並行テスト（MySQL が必要、build tag: integration）
```

各ドメインパッケージは以下の 3 層に分ける。

- `handler.go` : HTTP ⇄ JSON の変換、バリデーション、ステータスコード決定
- `service.go` : ビジネスルール。トランザクション境界をここで握る
- `repository.go` : SQL。`*sql.DB` と `*sql.Tx` の両方を受けられるよう `database.Executor` インターフェースを使う

過度なインターフェース化は避け、テストで差し替えが必要になった箇所だけ interface にする。

## 4. 横断的な設計方針

### エラーレスポンス（統一フォーマット）

```json
{ "error": { "code": "REWARD_ALREADY_CLAIMED", "message": "today's reward has already been claimed" } }
```

- `code` は機械判定用の定数、`message` は人間向け。内部エラー（SQL 文、スタック）は絶対に返さない。
- 500 系はログに request_id 付きで原因を出力し、クライアントには固定文言のみ返す。

### 時刻の扱い

- DB には **UTC** で保存（DSN に `loc=UTC`、セッション `time_zone='+00:00'`）。
- デイリー報酬の「日付」だけは **JST (Asia/Tokyo) で切り出して DATE 型**で保存する。日付判定ロジックは `reward` パッケージに閉じ込め、`time.Now` を差し替え可能にして単体テストする。

### ID

- `BIGINT UNSIGNED AUTO_INCREMENT`。InnoDB のクラスタインデックスと相性が良く、ランキングの tie-break にも使える。外部公開 ID の推測リスクは JWT による所有者チェックで担保する。

### データ整合性（最重要）

| ケース | 手段 |
|---|---|
| 報酬の二重取得 | `reward_claims (player_id, reward_date)` の UNIQUE 制約。INSERT → 1062 エラーなら 409。アプリ側の事前チェックには依存しない |
| アイテムの二重使用 | `UPDATE player_items SET quantity = quantity - ? WHERE ... AND quantity >= ?` の**条件付き UPDATE**。`RowsAffected == 0` なら 409。さらに `CHECK (quantity >= 0)` を最終防衛線に |
| プレイヤー更新の競合 | プレイヤーを変更する全トランザクションが**最初に** `SELECT ... FOR UPDATE` で players 行をロックする（ロック順序を統一）。INSERT を先に行うと FK チェックの共有ロックとの間でデッドロックが起きる（docs/database.md 参照） |
| 冪等性 | `Idempotency-Key` ヘッダ（アイテム使用で必須）。`item_transactions (player_id, idempotency_key)` UNIQUE。重複時は元のトランザクションを返し `replayed: true` を付ける。同じキーで内容が違えば 422（スコープ: プレイヤー単位、保持期間: 無期限＝履歴テーブルそのもの） |
| デッドロック | `database.WithTxRetry` が 1213 を最大 3 回再実行する保険。ロック順序が守られていれば発火しない |

### 認証フローの要点

- 登録は users と players を **同一トランザクション**で作成し、player_name 重複時は users も巻き戻す。
- ログイン失敗は「メール不存在」「パスワード不一致」を区別せず `INVALID_CREDENTIALS` を返し、不存在時もダミーハッシュと bcrypt 比較して応答時間を揃える。
- JWT は `alg` を HS256 に固定（`alg:none` / 鍵混同攻撃対策）、`iss` と `exp` を必須にする。
- 管理者は API から作れない。`ADMIN_EMAIL` / `ADMIN_PASSWORD` から起動時に冪等に作成する。

### セキュリティ

- bcrypt (cost 12)、JWT 有効期限 1h、`Authorization: Bearer`。
- すべての SQL はプレースホルダ。文字列連結禁止。
- `http.MaxBytesReader` で 1MB 制限。`ReadHeaderTimeout` / `ReadTimeout` / `WriteTimeout` / `IdleTimeout` を設定。
- ログ・エラーメッセージにパスワード／トークン／メールアドレスを出さない。

## 5. 想定しない範囲（MVP）

- リフレッシュトークン、OAuth、メール認証
- マルチリージョン・キャッシュ層（Redis）
- ゲームクライアントのリッチ UI
