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
│              Recoverer → Timeout → NoCache → SecHeaders  │
│              → BodyLimit → CORS → RateLimit(api) →       │
│              RateLimit(auth) → Auth(JWT) → RequireRole   │
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

すべて Docker Compose で起動する（`api` / `mysql` / `admin` / `prometheus` / `grafana`）。

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
| レート制限 | `go-chi/httprate` | chi 公式。IP ベースのインメモリ制限で MVP には十分。1 インスタンス前提で、複数台にする時は `LimitCounter` を Redis 実装に差し替える |
| CORS | `go-chi/cors` | 許可オリジンのホワイトリストのみ。空リストは全拒否にラップし直している（ライブラリの既定値は `*`） |

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

#### ミドルウェアの順序とその理由（Day 5）

| 順序 | ミドルウェア | 理由 |
|---|---|---|
| 1 | `RequestID` → `RealIP` | 以降の全ログと IP キーが同じ値を見る。`RealIP` は nginx が付ける `X-Forwarded-For` / `X-Real-IP` を信用する前提（API を直接インターネットに晒す構成では外す） |
| 2 | `Logger` → `Recoverer` → `Timeout` | panic もタイムアウトも JSON の 500 / 504 になり、必ず 1 行ログが残る |
| 3 | `NoCache` → `SecurityHeaders` | 全レスポンスに `no-store` と nosniff / DENY / no-referrer / CSP `default-src 'none'`。JSON API なので HTML 用ヘッダは最小限 |
| 4 | `BodyLimit` | `Content-Length` 超過は読まずに 413。chunked は `MaxBytesReader` が読み取り時に打ち切り、接続を閉じる |
| 5 | `CORS` | 429 や 401 にも CORS ヘッダが付くよう、レート制限と認証より**前**。許可オリジンの preflight はここで 204 を返し、認証まで到達しない |
| 6 | `RateLimit(api)` | `/api/v1` 配下にだけ掛け、`/health` / `/ready` はオーケストレーターが 429 を見ないよう対象外 |
| 7 | `RateLimit(auth)` | `register` / `login` は未認証で叩ける唯一の面なので、別のずっと小さい予算を重ねる（既定 10/min/IP）。ログイン成功も失敗も同じ予算を消費するので、総当たりは 1 分に 10 回で止まる |
| 8 | `Authenticate` → `RequireRole` | JWT 検証（署名・alg・iss・exp・失効）→ ロール判定 |

- レート制限はプロセス内メモリの固定ウィンドウ 2 本によるスライディング近似。水平スケール時は `httprate.WithLimitCounter` で Redis 実装に差し替える。
- CORS の credentials は常に `false`。認証はヘッダの Bearer トークンだけで Cookie を使わないため、CSRF の成立条件がない。
- `go-chi/cors` は空の許可リストを `*` と解釈するため、`middleware.CORS` が空リストを「全拒否」に変換している。`production` では `*` を設定エラーにする。

### 管理画面と管理 API

- `frontend/admin` は Vite + React + TypeScript + Tailwind v4 + TanStack Query + React Router。API 型は `src/lib/types.ts` で Go 側の JSON と 1:1 に対応させる。
- **同一オリジン構成**: 開発時は Vite の proxy、コンテナでは nginx が `/api/` を `api` サービスへ中継する。ブラウザから見ると UI と API は同一オリジンなので CORS もプリフライトも発生しない。CORS ミドルウェアは別オリジンのゲームクライアント向けで、`CORS_ALLOWED_ORIGINS` に列挙したオリジンにだけ応答する。
- 認可は API 側で完結する。`/api/v1/admin/*` は `Authenticate` → `RequireRole(admin)` を通り、player ロールは 403。UI のログイン画面は player トークンを受け取った場合、即 `/auth/logout` で失効させて破棄する。
- 監査ログ: `audit_logs` は item.create / item.update / item.delete / player.grant_item を、対象の変更と**同一トランザクション**で INSERT する。`detail` には before / after を JSON で保持し、UI で展開表示できる。
- アイテム付与 (`POST /admin/players/{id}/items`) もプレイヤー変更の一種なので、他の経路と同じく players 行の `FOR UPDATE` から始める。

### Observability

- **メトリクスは「ルートパターン」単位**。`internal/observability` のミドルウェアはルーターの最外側に置き、ハンドラが返った後に `chi.RouteContext(ctx).RoutePattern()` を読む。パスパラメータは `{id}` のまま、ルートに一致しない 404 / 405 は `unmatched` に畳む。これで「スキャナが叩いた無数の URL」がシリーズを増やすことはない。
- **レジストリはインスタンスごと**（`prometheus.NewRegistry`）。グローバル既定レジストリを使うとテストで複数ルーターを作った瞬間に二重登録で panic するため。`/metrics` ハンドラは `ContinueOnError` で、1 つのコレクタの失敗がスクレイプ全体を落とさないようにする。
- **業務指標は HTTP 指標から導出する**。`route="/api/v1/rewards/claim", status="409"` が「二重取得の拒否」、`items/use` の 409 が「所持不足」、422 が「冪等キー不一致」。専用カウンタを増やすより、ステータスコードを意味のある形に設計しておく方がドリフトしない。
- **DB プール**は `collectors.NewDBStatsCollector` で `go_sql_*` として公開。`wait_count` の増加がプール枯渇のシグナル。
- `/metrics` は `/api/v1` の外に置き、レート制限の対象外。認証は掛けず、ネットワーク層で守る（compose では Prometheus コンテナとホストポートからのみ到達）。
- ログは 1 リクエスト 1 行の JSON（`internal/middleware.Logger`）。メトリクスで「何が」起きたかを見て、request_id でログの「どのリクエストで」に辿り着く、という分担。

### CI（GitHub Actions）

`go`（gofmt / vet / unit / race / MySQL サービスコンテナで結合テスト）→ `frontend`（tsc + vite build）→ `compose`（イメージビルド、スタック起動、healthcheck 待ち、`/health` `/ready` `/metrics`、nginx 経由の管理者ログイン、Prometheus のターゲット up を確認）。`-race` は Linux ランナーでしか動かないため、ローカル（Windows, gcc なし）では CI に任せる。

## 5. 想定しない範囲（MVP）

- リフレッシュトークン、OAuth、メール認証
- マルチリージョン・キャッシュ層（Redis）
- ゲームクライアントのリッチ UI
