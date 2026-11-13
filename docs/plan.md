# 7日間開発チェックリスト

## Day 1：基盤構築
- [x] 環境確認（Go / Docker / Node / Git）
- [x] アーキテクチャ・DB 設計ドキュメント
- [x] Go モジュール作成、chi ルーター
- [x] 設定読み込み（環境変数）
- [x] MySQL 接続（database/sql）
- [x] golang-migrate 埋め込み + 初期マイグレーション
- [x] /health, /ready
- [x] 構造化ログ（slog JSON）・request_id・recover ミドルウェア
- [x] Dockerfile（マルチステージ・非 root）
- [x] docker-compose.yml（api + mysql）
- [x] Makefile / .env.example / README 初版
- [x] `go vet` / `go test` 通過、`docker compose up` 起動確認

## Day 2：認証・プレイヤー
- [x] users / players リポジトリ
- [x] POST /auth/register（bcrypt、players 行を同一 Tx で作成）
- [x] POST /auth/login（JWT 発行、タイミング攻撃対策）
- [x] POST /auth/logout（jti 失効テーブル）
- [x] GET /auth/me
- [x] 認証ミドルウェア（Bearer JWT → context）、RequireRole
- [x] GET /player/me, GET /player/me/items
- [x] 管理者アカウントの起動時ブートストラップ（ADMIN_EMAIL / ADMIN_PASSWORD）
- [x] Unit Test（パスワード、JWT、ミドルウェア、ハンドラ、レベルアップ計算）
- [x] 結合テスト基盤（tests/integration、minigame_test DB）+ 認証フローのテスト

## Day 3：ゲームロジック
- [x] GET /items
- [x] POST /items/use（条件付き UPDATE + Tx + Idempotency-Key、再送は保存済み結果を返却、内容不一致は 422）
- [x] POST /rewards/claim（UNIQUE + Tx + JST 日付）+ GET /rewards/status
- [x] GET /ranking（安定ソート + ページネーション）+ GET /ranking/me
- [x] 初期アイテム付与（登録時、`items.starter_quantity`、マイグレーション 000003）
- [x] 並行テスト：報酬 100 並行、アイテム使用 50 並行（所持 10）、同一 Idempotency-Key 20 並行、整合性検証
- [x] ロック順序の統一（players 行を先に FOR UPDATE）で FK 由来のデッドロックを解消

## Day 4：React 管理画面
- [x] Vite + React + TS + Tailwind v4 + TanStack Query + React Router（`frontend/admin`）
- [x] ログイン画面（admin 以外はトークンを即失効させて拒否）
- [x] Dashboard / Users / User 詳細（所持・取引・アイテム付与）/ Items（CRUD）/ 監査ログ 画面
- [x] 管理者 API（dashboard、users 一覧・詳細、items CRUD、players/{id}/items 付与、audit-logs）
- [x] 監査ログを変更と同一トランザクションで記録（before/after を JSON で保持）
- [x] compose に admin サービス追加（nginx が `/api/` を api へ同一オリジンで proxy、CORS 不要）

## Day 5：セキュリティ・品質
- [ ] admin ミドルウェア、権限テスト
- [ ] レート制限、CORS、ボディサイズ制限
- [ ] セキュリティテスト（未認証、権限、不正入力、SQLi 文字列）
- [ ] 冪等性の再送テスト

## Day 6：Observability・CI/CD
- [ ] Prometheus メトリクス（/metrics）
- [ ] compose に prometheus 追加（Grafana は余裕があれば）
- [ ] GitHub Actions（gofmt / vet / test / race / frontend / docker build）

## Day 7：仕上げ
- [ ] README 完成（起動手順、テスト結果、設計説明）
- [ ] docs/api-spec.md, docs/security.md
