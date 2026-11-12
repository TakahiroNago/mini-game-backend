-- revoked_tokens: JWT ids (jti) invalidated by logout. Rows are useless after
-- the token's own expiry, so they are purged opportunistically on logout.
CREATE TABLE revoked_tokens (
    jti        CHAR(32)        NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL,
    expires_at DATETIME(3)     NOT NULL,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (jti),
    KEY idx_revoked_tokens_expires_at (expires_at),
    CONSTRAINT fk_revoked_tokens_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;
