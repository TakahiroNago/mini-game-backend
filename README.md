# Mini Game Backend

Go + MySQL で実装するゲームバックエンド API と、React 製の運営管理画面。
**トランザクションと並行処理によるデータ整合性**、**認証・認可**、**Observability** を重点的に実装する。

> 現在の進捗: **Day 2（認証・プレイヤー）完了**。アイテム・報酬・ランキング・管理 API と管理画面は未実装。
> 進捗の詳細は [docs/plan.md](docs/plan.md) を参照。

## 技術スタック

| 層 | 技術 |
|---|---|
| API | Go 1.27, chi v5, `database/sql` + go-sql-driver/mysql, golang-migrate（埋め込み）, log/slog |
| DB | MySQL 8.4 (InnoDB, utf8mb4) |
| 管理画面 | React + TypeScript + Vite + Tailwind CSS + TanStack Query（Day 4） |
| 基盤 | Docker / Docker Compose, GitHub Actions（Day 6）, Prometheus（Day 6） |

設計の詳細: [docs/architecture.md](docs/architecture.md) / [docs/database.md](docs/database.md)

## 必要なもの

- Docker Desktop（Compose v2 以上）
- ローカルで Go を直接動かす場合: Go 1.27 以上
- 管理画面を開発する場合: Node.js 20 以上（Day 4 以降）

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

API 起動時に `AUTO_MIGRATE=true`（開発時の既定値）であれば、埋め込まれた
`migrations/*.sql` が自動適用される。

### ホスト側の 8080 / 3306 が使用中の場合

`.env` でホスト側ポートだけ変更できる（コンテナ内のポートは変わらない）。

```dotenv
API_HOST_PORT=8081
MYSQL_HOST_PORT=13306
DB_PORT=13306   # ホストから go run する場合はこちらも合わせる
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

## テスト

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

## 設定（環境変数）

| 変数 | 既定値 | 説明 |
|---|---|---|
| `APP_ENV` | `development` | `production` では `AUTO_MIGRATE=false` と 32 文字以上の `JWT_SECRET` を強制 |
| `HTTP_ADDR` | `:8080` | 待受アドレス |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `AUTO_MIGRATE` | `true` | 起動時にマイグレーションを適用 |
| `REQUEST_TIMEOUT` | `10s` | 1 リクエストのタイムアウト |
| `SHUTDOWN_TIMEOUT` | `15s` | graceful shutdown の猶予 |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:5173` | 管理画面のオリジン（カンマ区切り） |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | – | MySQL 接続情報（`DB_PASSWORD` は必須） |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME` | `25` / `10` / `5m` | コネクションプール |
| `JWT_SECRET` / `JWT_EXPIRY` | –（必須） / `1h` | JWT 署名鍵と有効期限 |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | – | 起動時に管理者アカウントを作成（両方指定、既存ならスキップ） |

全一覧は [.env.example](.env.example) を参照。

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

```bash
# 登録 → ログイン → 自分の情報
curl -X POST localhost:8080/api/v1/auth/register -H 'Content-Type: application/json'   -d '{"email":"hero@example.com","password":"password123","player_name":"hero"}'
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json'   -d '{"email":"hero@example.com","password":"password123"}' | jq -r .access_token)
curl localhost:8080/api/v1/player/me -H "Authorization: Bearer $TOKEN"
```

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
internal/middleware ログ・リカバリ・認証 (JWT)・認可 (role)
internal/auth      登録・ログイン・ログアウト・JWT・管理者ブートストラップ
internal/player    プレイヤー情報・所持アイテム・レベルアップ計算
internal/health    /health, /ready
internal/server    ルーター組み立て・サービス生成
tests/integration  MySQL を使う結合テスト（-tags=integration）
migrations/        SQL マイグレーション（バイナリに埋め込み）
docs/              設計ドキュメント
docker/            Compose 補助ファイル（certs など）
```
