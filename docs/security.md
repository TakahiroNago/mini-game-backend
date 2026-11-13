# セキュリティ設計

このドキュメントは「何を脅威とみなし、どこで、どう防いでいるか」を 1 枚にまとめたもの。実装箇所とテストを併記する。

## 1. 前提と信頼境界

```
Internet ──▶ [nginx (admin)] ──▶ [Go API] ──▶ [MySQL]
                 ▲ 同一オリジン        ▲ Bearer JWT       ▲ 専用ユーザー / プレースホルダ
ゲームクライアント ─────────────▶ (CORS 許可オリジンのみ)
Prometheus ──────────────────▶ /metrics (内部ネットワーク)
```

- 認証情報は **ヘッダの Bearer トークンのみ**。Cookie を使わないため CSRF が成立する前提がなく、CORS でも credentials を許可しない。
- API の `RealIP` は前段 nginx の `X-Forwarded-For` / `X-Real-IP` を信用する。API を直接公開する構成ではこのミドルウェアを外すか、信頼プロキシを限定する。
- `/metrics` は無認証。Compose では内部ネットワークとホストポートからしか到達しない。本番では reverse proxy で遮断する。

## 2. 脅威と対策の対応表

| 脅威 | 対策 | 実装 | テスト |
|---|---|---|---|
| パスワード漏えい | bcrypt cost 12、平文はログに出さない、72 文字上限（bcrypt 制約） | `internal/auth/password.go` | `auth` 単体テスト |
| 総当たりログイン | `register` / `login` に IP 別 10 回/分（`AUTH_RATE_LIMIT_PER_MINUTE`）、成功・失敗とも同じ予算 | `middleware.RateLimit` を `/auth` 公開ルートに重ねる | `TestAuthRoutesHaveTheirOwnRateLimit`、`TestSecurityRateLimitAgainstLogin` |
| アカウント列挙 | 不存在メールでもダミーハッシュと bcrypt 比較し、`INVALID_CREDENTIALS` に統一 | `auth.Service.Login` | `TestAuthFlow` |
| トークン偽造 | HS256 固定（`alg: none` / RS→HS 混同を拒否）、`iss` / `exp` 必須、`sub` と `role` と `jti` の形式検証 | `auth.TokenIssuer.Parse` | `TestSecurityTokens`（期限切れ / 別鍵 / alg none / 改竄） |
| ログアウト後の再利用 | `jti` を `revoked_tokens` に登録し、毎リクエスト参照 | `middleware.Authenticate` | `TestSecurityTokens`（revoked） |
| 権限昇格 | `role` は DB の `users.role` から発行時に埋め込み、管理ルートは `RequireRole(admin)`。管理者は API から作れず環境変数からのみ | `middleware.RequireRole`、`auth.Service.EnsureAdmin` | `TestSecurityAdminAuthorizationMatrix`（全 admin ルート × 匿名 / player / admin） |
| SQL インジェクション | 全クエリがプレースホルダ。文字列連結なし。パス ID は `strconv.ParseInt`、`LIKE` の検索語も `?` に渡す | `*/repository.go`、`httpx.PathID` | `TestSecuritySQLInjectionIsInert`（テーブル数・行数の不変を確認） |
| 不正入力 | JSON は未知フィールド・末尾データ拒否、構造体タグのバリデーション、`quantity` 上限、`limit` / `offset` 範囲 | `httpx.Decode` / `httpx.Validate` / `httpx.Pagination` | `TestSecurityMalformedInput` |
| 巨大ボディ / Slowloris | `MAX_BODY_BYTES`（既定 1 MiB、`Content-Length` 超過は読まずに 413）、`ReadHeaderTimeout` 5s / `ReadTimeout` 15s / `IdleTimeout` 60s / `MaxHeaderBytes` 64 KB、リクエストタイムアウト | `middleware.BodyLimit`、`cmd/api/main.go` | `TestBodyLimit`、`TestSecurityBodyLimit` |
| API 乱用 | `/api/v1/*` に IP 別 300 回/分、429 は JSON + `Retry-After`。probe と `/metrics` は対象外 | `middleware.RateLimit` | `TestGlobalRateLimitCoversTheAPI` |
| 二重消費・二重取得（整合性） | players 行 `FOR UPDATE` のロック順序統一、条件付き UPDATE、UNIQUE 制約、`Idempotency-Key` | `item.Service.Use`、`reward.Service.Claim` | `TestConcurrentItemUse`、`TestConcurrentRewardClaims`、`TestIdempotency*` |
| 他人の冪等キーを使った再生 | キーは `(player_id, idempotency_key)` でスコープされ、他プレイヤーの受領証は返らない | `item_transactions` UNIQUE | `TestIdempotencyKeyScopeAndFormat` |
| クロスオリジンからの呼び出し | 許可リスト方式。空なら全拒否（ライブラリ既定の `*` を上書き）、本番では `*` を設定エラー、credentials 常に不許可 | `middleware.CORS`、`config.Load` | `TestCORS`、`TestCORSPreflightThroughRouter`、`TestLoad_SecurityValidation` |
| レスポンスの誤解釈（MIME sniffing、クリックジャッキング） | `nosniff`、`X-Frame-Options: DENY`、CSP `default-src 'none'`、`Referrer-Policy: no-referrer`、`Cache-Control: no-store` | `middleware.SecurityHeaders`、chi `NoCache` | `TestSecurityHeadersOnEveryResponse` |
| 情報漏えい（エラー・ログ） | 5xx は固定文言、原因は `request_id` 付きでサーバーログのみ。ログにヘッダ・ボディ・クエリを出さない | `httpx.WriteError`、`middleware.Logger` | `TestSecurityMalformedInput`（`sql` / `goroutine` / `.go:` が本文に出ないこと） |
| 管理操作の否認 | 監査ログを変更と同一トランザクションで記録（誰が・何を・before / after） | `admin.Service` | `TestAdminItemsCRUDWithAudit` |
| 設定ミスでの本番起動 | `APP_ENV=production` では `AUTO_MIGRATE=false`、32 文字以上の `JWT_SECRET`、レート制限の無効化禁止、CORS `*` 禁止を強制 | `config.Load` | `TestLoad_ProductionHardening`、`TestLoad_SecurityValidation` |
| 依存・ビルド | Go モジュールは `go.sum` で固定、コンテナは非 root（uid 10001）の alpine、`CGO_ENABLED=0` の静的バイナリ | `Dockerfile` | CI の compose ジョブ |

## 3. ミドルウェアの順序

```
RequestID → RealIP → Metrics → Logger → Recoverer → Timeout → NoCache → SecurityHeaders
→ BodyLimit → CORS → RateLimit(/api/v1) → RateLimit(auth) → Authenticate → RequireRole
```

- CORS をレート制限・認証より前に置くので、429 / 401 にも CORS ヘッダが付き、ブラウザが本文を読める。
- Recoverer が Timeout より外にあるので、タイムアウト中の panic も JSON の 500 になる。
- Metrics が最外側なので、panic → 500 / timeout → 504 も実際に返した status で集計される。

## 4. 意図的にやっていないこと（MVP の範囲外）

| 項目 | 理由 / 代替 |
|---|---|
| リフレッシュトークン | 有効期限 1h のアクセストークン + 失効表で十分。長期セッションが要件になった時点で追加 |
| メール認証・パスワードリセット | メール送信基盤が要る。本プロジェクトの主題（整合性・並行処理）から外れる |
| `/metrics` の認証 | ネットワーク層で守る方が運用上一般的 |
| 分散レート制限（Redis） | 1 インスタンス前提。`httprate.WithLimitCounter` で差し替え可能にしてある |
| WAF / Bot 対策 | インフラ層の責務 |
| 監査ログの改竄防止（WORM） | DB への書き込み権限を持つ者の脅威は対象外 |

## 5. 運用上の注意

- `JWT_SECRET` を変えると全トークンが即失効する（緊急時の全ログアウト手段として使える）。
- `revoked_tokens` は `expires_at` を持つので、期限切れ行は定期削除してよい（現状は手動）。
- `ADMIN_PASSWORD` は初回起動時にしか使われない。変更する場合は DB の `password_hash` を更新する。
- レート制限はプロセス内メモリ。再起動でリセットされ、複数インスタンスでは台数分だけ緩くなる。
