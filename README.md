# Mini Game Backend

Go + MySQL で実装するゲームバックエンド API と、React 製の運営管理画面。
**トランザクションと並行処理によるデータ整合性**、**認証・認可**、**Observability** を重点的に実装する。

[![CI](https://github.com/TakahiroNago/mini-game-backend/actions/workflows/ci.yml/badge.svg)](https://github.com/TakahiroNago/mini-game-backend/actions/workflows/ci.yml)

> 7 日間の計画（[docs/plan.md](docs/plan.md)）はすべて完了。

## ドキュメント

| ドキュメント | 内容 |
|---|---|
| [docs/architecture.md](docs/architecture.md) | 全体構成、技術選定の理由、レイヤリング、ミドルウェア順序、Observability、CI |
| [docs/database.md](docs/database.md) | ER 図、テーブル定義、ロック順序、トランザクション設計、マイグレーション |
| [docs/api-spec.md](docs/api-spec.md) | 全エンドポイントの要求 / 応答 / エラーコード |
| [docs/security.md](docs/security.md) | 脅威 × 対策 × テストの対応表、やっていないこと |

## このプロジェクトで見せたいこと

1. **整合性を DB の制約で保証する**: 報酬の二重取得は UNIQUE 制約、アイテムの二重消費は条件付き UPDATE、冪等性は `(player_id, idempotency_key)` の UNIQUE。アプリ側の事前チェックには依存しない。
2. **ロック順序の統一**: プレイヤーを変更する全トランザクションが最初に players 行を `FOR UPDATE` する。開発中に 50 並行テストで 43 件のデッドロックを再現し、順序統一でゼロにした（[docs/database.md](docs/database.md#ロック順序の原則最重要)）。
3. **並行テストを CI で毎回回す**: 100 並行の報酬取得、50 並行のアイテム使用、20 並行の同一キー再送を実際の MySQL に対して実行し、DB の最終状態まで検証する。
4. **多層のセキュリティ**: JWT（`alg` 固定 + 失効表）、ロール認可、入力検証、プレースホルダ、二段のレート制限、ボディ上限、CORS 許可リスト、セキュリティヘッダ。それぞれにテストがある。
5. **運用を見据えた作り**: 監査ログは変更と同一トランザクション、Prometheus / Grafana、JSON ログ + request_id、graceful shutdown、本番設定の強制チェック。

## 技術スタック

| 層 | 技術 |
|---|---|
| API | Go 1.27, chi v5, `database/sql` + go-sql-driver/mysql, golang-migrate（埋め込み）, log/slog |
| DB | MySQL 8.4 (InnoDB, utf8mb4) |
| 管理画面 | React 19 + TypeScript + Vite + Tailwind CSS v4 + TanStack Query + React Router（nginx で同一オリジン配信） |
| Observability | log/slog（JSON、request_id 付き）, Prometheus `/metrics`（client_golang）, Grafana ダッシュボード |
| 基盤 | Docker / Docker Compose（mysql / api / admin / prometheus / grafana）, GitHub Actions |

設計の詳細: [docs/architecture.md](docs/architecture.md) / [docs/database.md](docs/database.md)

## 必要なもの

- Docker Desktop（Compose v2 以上）
- ローカルで Go を直接動かす場合: Go 1.27 以上
- 管理画面を開発する場合: Node.js 24（`frontend/admin`）

## クイックスタート（Docker Compose）

```bash
cp .env.example .env        # 必要に応じてポートやパスワードを変更
docker compose up -d --build
```

起動確認:

```bash
curl http://localhost:8080/health   # {"status":"ok","version":"dev"}
curl http://localhost:8080/ready    # {"checks":{"mysql":"ok"},"status":"ready"}
```

管理画面は http://localhost:5173 （`ADMIN_HOST_PORT`）。`.env` の `ADMIN_EMAIL` / `ADMIN_PASSWORD` でログインする。
nginx が `/api/` を `api` コンテナへ同一オリジンで中継するため、ブラウザ側に CORS 設定は不要。

Prometheus は http://localhost:9090 （`PROMETHEUS_HOST_PORT`）、Grafana は http://localhost:3000 （`GRAFANA_HOST_PORT`、
`GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD`、既定 admin / admin）。Grafana には "Mini Game API" ダッシュボードが
プロビジョニング済みで、起動直後から表示される。

API 起動時に `AUTO_MIGRATE=true`（開発時の既定値）であれば、埋め込まれた
`migrations/*.sql` が自動適用される。

### ホスト側の 8080 / 3306 / 5173 / 9090 / 3000 が使用中の場合

`.env` でホスト側ポートだけ変更できる（コンテナ内のポートは変わらない）。

```dotenv
API_HOST_PORT=8081
MYSQL_HOST_PORT=13306
DB_PORT=13306   # ホストから go run する場合はこちらも合わせる
ADMIN_HOST_PORT=5174
PROMETHEUS_HOST_PORT=19090
GRAFANA_HOST_PORT=13000
```

### TLS 検査型のプロキシ / アンチウイルス環境でビルドが失敗する場合

`go mod download` が `x509: certificate signed by unknown authority` で失敗するときは、
ルート CA 証明書（PEM）を `docker/certs/*.crt` に置くとビルド段階で信頼される。
手順は [docker/certs/README.md](docker/certs/README.md) を参照。

## ローカルで直接実行する（Go）

```bash
# MySQL だけコンテナで起動
docker compose up -d mysql

# .env を読み込んで API を起動（.env が無ければ環境変数から）
go run ./cmd/api

# マイグレーション CLI
go run ./cmd/migrate -cmd up
go run ./cmd/migrate -cmd version
go run ./cmd/migrate -cmd down -steps 1
```

`make` が使える環境なら `make help` で一覧を表示できる（Windows は `scoop install make`）。

## 管理画面を開発モードで動かす

```bash
cd frontend/admin
cp .env.example .env            # VITE_API_PROXY_TARGET を API のポートに合わせる
npm ci
npm run dev                     # http://localhost:5173 、/api は API へ proxy
npm run build                   # 型チェック + 本番ビルド（dist/）
```

## テスト

最終確認時点の結果（2026-11-13、`make test-docker` と GitHub Actions）:

| 種別 | 内容 | 結果 |
|---|---|---|
| 単体テスト | 12 パッケージ（config / httpx / middleware / auth / player / item / reward / ranking / admin / health / observability / server） | 全て PASS |
| 結合テスト | `tests/integration` 22 本（認証、ゲームロジック、並行 3 本、管理 3 本、セキュリティ 6 本、冪等性 3 本） | 全て PASS（約 25 秒） |
| race detector | CI（Linux）で単体・結合とも `-race` 付きで実行 | PASS |
| フロントエンド | `tsc` + `vite build` | PASS |
| スタック起動 | CI でイメージをビルドし、healthcheck → API / 管理画面 / Prometheus の疎通 | CI の compose ジョブで確認（初回実行は最終ステップの curl 構文で失敗、修正済み） |

```bash
go vet ./...
go test ./... -count=1
go test ./... -race -count=1     # race detector は CGO（gcc）が必要。Windows では CI (Linux) で実行する
```

結合テスト（MySQL 必須）は `tests/integration` にあり、build tag `integration` で選択する。
開発データを壊さないよう **別データベース `minigame_test`** を使う（Compose の初期化スクリプトが作成する）。

```bash
docker compose up -d mysql
TEST_DB_NAME=minigame_test go test ./tests/... -tags=integration -count=1 -v
```

接続先は `.env` の `DB_*` を使い、`TEST_DB_HOST` / `TEST_DB_PORT` / `TEST_DB_USER` / `TEST_DB_PASSWORD` / `TEST_DB_NAME` で上書きできる。
テストは実行のたびに対象 DB の全テーブルを TRUNCATE する。

ホストのアンチウイルスが新しいテストバイナリを長時間ホールドする環境では、`make test-docker`
（Go コンテナ内で vet + 単体 + 結合テストを実行、Compose のネットワーク経由で MySQL に接続）を使う。

並行テスト（`tests/integration/concurrency_test.go`）は実際の MySQL に対して同時リクエストを投げ、DB の最終状態まで検証する。
セキュリティテスト（`security_test.go`）と冪等性の再送テスト（`idempotency_test.go`）も同じハーネスで動く。

| テスト | 内容 | 期待結果 |
|---|---|---|
| `TestConcurrentRewardClaims` | 同一プレイヤーが報酬を 100 並行で取得 | 200 ×1 / 409 ×99、`reward_claims` 1 行、コイン +100 ちょうど |
| `TestConcurrentItemUse` | 所持 10 個のアイテムを 50 並行で使用（別キー） | 200 ×10 / 409 ×40、所持数 0（負にならない）、台帳 10 行、経験値は 10 回分 |
| `TestConcurrentIdempotentReplay` | 同じ `Idempotency-Key` で 20 並行 | 全て 200、`replayed=false` は 1 件、transaction_id は 1 種類、消費は 1 個だけ |
| `TestIdempotencyRetry` ほか | 同一キーで 5 回連続再送、失敗した試行のキー再利用、2 プレイヤーで同じキー、キー形式 | 台帳 1 行・同じ transaction_id、失敗はキーを消費しない、キーはプレイヤー単位、不正な形式は 400 |
| `TestSecurityMalformedInput` | 空ボディ / 非 JSON / 途中で切れた JSON / 未知フィールド / 型違い / 不正なページング | すべて 400 の JSON エラー。内部情報（SQL・スタック）を含まない |
| `TestSecuritySQLInjectionIsInert` | 代表的な SQLi 文字列をログイン・登録・管理検索・アイテム名へ投入 | バリデーションで 400、または単なる文字列として扱われる。テーブル数・行数は不変 |
| `TestSecurityTokens` | 期限切れ / 別鍵署名 / `alg: none` / 改竄 / ログアウト後のトークン | すべて 401（`TOKEN_EXPIRED` / `UNAUTHORIZED` / `TOKEN_REVOKED`） |
| `TestSecurityAdminAuthorizationMatrix` | 全 admin ルート × 匿名 / player / admin | 匿名 401、player 403（監査ログも残らない）、admin は通過 |
| `TestSecurityRateLimitAgainstLogin` | `AUTH_RATE_LIMIT_PER_MINUTE=5` でログインを 6 回 | 6 回目は正しいパスワードでも 429 + `Retry-After`。別 IP は影響なし |
| `TestSecurityBodyLimit` | `MAX_BODY_BYTES=1024` で 2 KB の登録リクエスト | 413、ユーザーは作られない |

## 設定（環境変数）

| 変数 | 既定値 | 説明 |
|---|---|---|
| `APP_ENV` | `development` | `production` では `AUTO_MIGRATE=false` と 32 文字以上の `JWT_SECRET` を強制 |
| `HTTP_ADDR` | `:8080` | 待受アドレス |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `AUTO_MIGRATE` | `true` | 起動時にマイグレーションを適用 |
| `REQUEST_TIMEOUT` | `10s` | 1 リクエストのタイムアウト |
| `SHUTDOWN_TIMEOUT` | `15s` | graceful shutdown の猶予 |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:5173` | 別オリジンから API を呼ぶブラウザクライアントの許可リスト（カンマ区切り、空なら全拒否、`production` では `*` 不可）。コンテナ版の管理画面は nginx 経由の同一オリジンなので不要 |
| `MAX_BODY_BYTES` | `1048576` | リクエストボディの上限（超過は 413 `PAYLOAD_TOO_LARGE`） |
| `RATE_LIMIT_PER_MINUTE` | `300` | クライアント IP ごとの `/api/v1/*` 全体の予算（1 分のスライディングウィンドウ、超過は 429 `RATE_LIMITED`）。`0` で無効（`production` では不可） |
| `AUTH_RATE_LIMIT_PER_MINUTE` | `10` | 同じく `register` / `login` だけに掛かる厳しめの予算（ブルートフォース対策） |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | – | MySQL 接続情報（`DB_PASSWORD` は必須） |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME` | `25` / `10` / `5m` | コネクションプール |
| `JWT_SECRET` / `JWT_EXPIRY` | –（必須） / `1h` | JWT 署名鍵と有効期限 |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | – | 起動時に管理者アカウントを作成（両方指定、既存ならスキップ） |

全一覧は [.env.example](.env.example) を参照。

## Observability

- **ログ**: `log/slog` の JSON を標準出力へ。1 リクエスト 1 行（request_id / method / path / status / bytes / latency_ms / remote_ip）。`X-Request-Id` をレスポンスにも返すので、問い合わせからログを引ける。
- **メトリクス**: `GET /metrics`（Prometheus テキスト形式、レート制限の対象外）。ラベルは method / chi のルートパターン / status の 3 つだけで、`/api/v1/users/42` のような生のパスは `/api/v1/admin/users/{id}` に正規化され、404 は `unmatched` にまとめる（カーディナリティを固定）。

| メトリクス | 種類 | 説明 |
|---|---|---|
| `minigame_http_requests_total{method,route,status}` | counter | リクエスト数。`route="/api/v1/rewards/claim", status="409"` で「二重取得の拒否回数」のように業務指標としても読める |
| `minigame_http_request_duration_seconds{method,route}` | histogram | レイテンシ（1 ms〜10 s の 13 バケット） |
| `minigame_http_requests_in_flight` | gauge | 処理中リクエスト数 |
| `minigame_build_info{version}` | gauge | ビルドバージョン（常に 1） |
| `go_sql_*{db_name}` | gauge / counter | `database/sql` プールの open / in-use / idle / wait |
| `go_*`, `process_*` | – | Go ランタイム（goroutine、GC、ヒープ）とプロセス（CPU、RSS、FD） |

- **Prometheus**: `docker/prometheus/prometheus.yml` が `api:8080/metrics` を 15 秒ごとに取得。保持期間 7 日。
- **Grafana**: `docker/grafana/provisioning` でデータソースとダッシュボード（`docker/grafana/dashboards/minigame-api.json`）を自動登録。パネルは RPS、5xx 比率、p50/p95/p99、ルート別・ステータス別 RPS、報酬取得 / アイテム使用の結果別件数、DB プール、Go ランタイム。
- `make metrics` でアプリ指標だけを手元に表示できる。
- `/metrics` には認証を掛けていない。本番ではネットワーク層（内部ネットワークのみ公開、または reverse proxy で遮断）で保護する前提。

## CI（GitHub Actions）

`.github/workflows/ci.yml` が push / pull request ごとに 3 ジョブを実行する。

| ジョブ | 内容 |
|---|---|
| `go` | `gofmt -s -l`、`go vet`（integration タグ含む）、単体テスト、`-race` 付き単体テスト、MySQL 8.4 サービスコンテナに対する結合・並行・セキュリティテスト（`-race`） |
| `frontend` | `npm ci` → `tsc` + `vite build` |
| `compose` | `docker compose build api admin` → `docker compose up -d` → api / admin の healthcheck を待って `/health` `/ready` `/metrics`、管理画面の `/healthz` と nginx 経由の管理者ログイン + dashboard、Prometheus の `up{job="minigame-api"}` を確認 → `down -v` |

ローカルで同等のチェックをするには `make ci`（fmt / vet / unit）と `make test-docker`（結合テストまで）を使う。

## セキュリティ

- **認証**: bcrypt (cost 12) + JWT HS256（`alg` 固定、`iss` / `exp` 必須、`jti` の失効表でログアウト）。管理者は環境変数からのみ作成。
- **認可**: `/api/v1/admin/*` は `Authenticate` → `RequireRole(admin)`。player ロールは 403。
- **入力**: JSON は未知フィールド・末尾データを拒否、構造体タグでバリデーション。パス ID は整数のみ。SQL は全てプレースホルダ。
- **レート制限**: クライアント IP 別のスライディングウィンドウ（`go-chi/httprate`）。`/api/v1/*` 全体と `register` / `login` の二段構え。`/health` / `/ready` は対象外。429 は統一エラー形式 + `Retry-After` / `X-RateLimit-*`。
- **ボディ上限**: `MAX_BODY_BYTES`（既定 1 MiB）。`Content-Length` 超過は読まずに 413、chunked は読み取り時に打ち切り。
- **CORS**: `CORS_ALLOWED_ORIGINS` のオリジンだけに `GET/POST/PUT/DELETE` と `Authorization` / `Content-Type` / `Idempotency-Key` ヘッダを許可。credentials は常に不許可（Cookie を使わないため）。
- **レスポンスヘッダ**: `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Referrer-Policy: no-referrer`、`Content-Security-Policy: default-src 'none'`、`Cache-Control: no-store`。
- **サーバー**: `ReadHeaderTimeout` / `ReadTimeout` / `WriteTimeout` / `IdleTimeout`、`MaxHeaderBytes` 64 KB、リクエストごとのタイムアウト。
- **ログ**: パスワード・トークン・メールアドレスを出さない。5xx のみ request_id 付きで原因をログに残し、クライアントには固定文言。

## エンドポイント（実装済み）

| メソッド | パス | 認証 | 説明 |
|---|---|---|---|
| GET | `/health` | – | プロセス稼働確認（依存先は見ない） |
| GET | `/ready` | – | MySQL への疎通を含む準備確認。失敗時は 503 |
| POST | `/api/v1/auth/register` | – | ユーザー + プレイヤー登録（同一トランザクション） |
| POST | `/api/v1/auth/login` | – | JWT 発行 |
| POST | `/api/v1/auth/logout` | Bearer | トークン失効（`jti` を失効表へ） |
| GET | `/api/v1/auth/me` | Bearer | 認証ユーザー情報 |
| GET | `/api/v1/player/me` | Bearer | 自分のプレイヤー情報 |
| GET | `/api/v1/player/me/items` | Bearer | 所持アイテム一覧 |
| GET | `/api/v1/items` | Bearer | アイテムカタログ（論理削除済みは除く） |
| POST | `/api/v1/items/use` | Bearer + `Idempotency-Key` | アイテム使用。条件付き UPDATE で二重消費を防止、同一キーの再送は保存済み結果を返す |
| POST | `/api/v1/rewards/claim` | Bearer | デイリー報酬（JST 1 日 1 回、UNIQUE 制約で保証）。2 回目は 409 |
| GET | `/api/v1/rewards/status` | Bearer | 本日の報酬を取得済みか |
| GET | `/api/v1/ranking` | Bearer | ランキング（`?limit=20&offset=0`、level → experience → id の安定ソート） |
| GET | `/api/v1/ranking/me` | Bearer | 自分の順位 |
| GET | `/api/v1/admin/dashboard` | admin | 件数サマリ（本日 = JST）と上位プレイヤー |
| GET | `/api/v1/admin/users` | admin | ユーザー一覧（`?q=` でメール / プレイヤー名検索、`limit` / `offset`） |
| GET | `/api/v1/admin/users/{id}` | admin | ユーザー詳細（プレイヤー、所持アイテム、直近取引、報酬回数） |
| GET | `/api/v1/admin/items` | admin | アイテム一覧（`?include_deleted=true` で論理削除済みも） |
| POST / PUT / DELETE | `/api/v1/admin/items[/{id}]` | admin | アイテム作成 / 更新 / 論理削除。監査ログを同一 Tx で記録 |
| POST | `/api/v1/admin/players/{id}/items` | admin | アイテム付与（acquire 取引 + 監査ログ） |
| GET | `/api/v1/admin/audit-logs` | admin | 監査ログ一覧 |

```bash
# 登録 → ログイン → 自分の情報
curl -X POST localhost:8080/api/v1/auth/register -H 'Content-Type: application/json'   -d '{"email":"hero@example.com","password":"password123","player_name":"hero"}'
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json'   -d '{"email":"hero@example.com","password":"password123"}' | jq -r .access_token)
curl localhost:8080/api/v1/player/me -H "Authorization: Bearer $TOKEN"

# 初期アイテム（Small Coin Pouch x3, Experience Potion x3）を確認して使う
curl localhost:8080/api/v1/player/me/items -H "Authorization: Bearer $TOKEN"
curl -X POST localhost:8080/api/v1/items/use -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: $(uuidgen)" \
  -d '{"item_id":3,"quantity":2}'
# → {"transaction_id":..,"item_id":3,"quantity":2,"replayed":false,"player":{"level":2,"experience":0,"coins":0}}

# デイリー報酬とランキング
curl -X POST localhost:8080/api/v1/rewards/claim -H "Authorization: Bearer $TOKEN"
curl "localhost:8080/api/v1/ranking?limit=10" -H "Authorization: Bearer $TOKEN"
```

主なエラーコード: `INSUFFICIENT_ITEM` (409)、`REWARD_ALREADY_CLAIMED` (409)、`ITEM_NOT_FOUND` (404)、
`IDEMPOTENCY_KEY_REQUIRED` (400)、`IDEMPOTENCY_KEY_MISMATCH` (422)。

エラーレスポンスは全エンドポイント共通で以下の形式。

```json
{ "error": { "code": "NOT_FOUND", "message": "resource not found" } }
```

## ディレクトリ構成

```
cmd/api            API サーバー本体
cmd/migrate        マイグレーション CLI
internal/config    環境変数の読み込み
internal/database  接続プール・トランザクションヘルパー・migrate 実行
internal/httpx     JSON / 統一エラー / リクエストデコード / バリデーション
internal/identity  認証済み主体（Claims）と context ヘルパー
internal/middleware ログ・リカバリ・セキュリティヘッダ・ボディ上限・CORS・レート制限・認証 (JWT)・認可 (role)
internal/auth      登録・ログイン・ログアウト・JWT・管理者ブートストラップ
internal/player    プレイヤー情報・所持アイテム・レベルアップ計算・インベントリ増減 SQL
internal/item      アイテムカタログ・初期付与・使用トランザクション（Idempotency-Key）
internal/reward    デイリー報酬（JST 日付、UNIQUE 制約）
internal/ranking   ランキング（安定ソート・ページネーション・自分の順位）
internal/admin     管理 API（ユーザー / アイテム CRUD / 付与 / 監査ログ）
internal/observability Prometheus レジストリ・HTTP メトリクスミドルウェア・/metrics ハンドラ
frontend/admin     React 管理画面（Vite + TS + Tailwind + TanStack Query）、nginx 用 Dockerfile
internal/health    /health, /ready
internal/server    ルーター組み立て・サービス生成
tests/integration  MySQL を使う結合テスト（-tags=integration）
migrations/        SQL マイグレーション（バイナリに埋め込み）
docs/              設計ドキュメント
docker/            Compose 補助ファイル（certs、mysql 初期化 SQL、prometheus 設定、grafana プロビジョニング）
.github/workflows  GitHub Actions（CI）
```
