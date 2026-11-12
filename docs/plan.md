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
- [ ] users / players リポジトリ
- [ ] POST /auth/register（bcrypt、players 行を同一 Tx で作成）
- [ ] POST /auth/login（JWT 発行）
- [ ] POST /auth/logout（jti 失効）
- [ ] GET /auth/me
- [ ] 認証ミドルウェア（Bearer JWT → context にユーザー）
- [ ] GET /player/me, GET /player/me/items
- [ ] Unit Test（パスワード、JWT、レベルアップ計算）

## Day 3：ゲームロジック
- [ ] GET /items
- [ ] POST /items/use（条件付き UPDATE + Tx + Idempotency-Key）
- [ ] POST /rewards/claim（UNIQUE + Tx + JST 日付）
- [ ] GET /ranking（安定ソート + ページネーション）
- [ ] 初期アイテム付与（登録時）
- [ ] 並行テスト：報酬 100 並行、アイテム使用並行、整合性検証

## Day 4：React 管理画面
- [ ] Vite + React + TS + Tailwind + TanStack Query
- [ ] ログイン画面（admin のみ）
- [ ] Dashboard / Users / Items 画面
- [ ] 管理者 API（users 一覧・詳細、items CRUD、監査ログ）
- [ ] compose に admin サービス追加

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
