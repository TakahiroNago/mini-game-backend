-- users: authentication identity. One user owns exactly one player.
CREATE TABLE users (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    email         VARCHAR(255)    NOT NULL,
    password_hash VARCHAR(255)    NOT NULL,
    role          ENUM('player', 'admin') NOT NULL DEFAULT 'player',
    created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_email (email)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- players: in-game progression. experience is "exp within the current level".
CREATE TABLE players (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id     BIGINT UNSIGNED NOT NULL,
    player_name VARCHAR(32)     NOT NULL,
    level       INT             NOT NULL DEFAULT 1,
    experience  BIGINT          NOT NULL DEFAULT 0,
    coins       BIGINT          NOT NULL DEFAULT 0,
    created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_players_user_id (user_id),
    UNIQUE KEY uk_players_player_name (player_name),
    -- matches ORDER BY level DESC, experience DESC, id ASC exactly (no filesort)
    KEY idx_players_ranking (level DESC, experience DESC, id ASC),
    CONSTRAINT fk_players_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT chk_players_level CHECK (level >= 1),
    CONSTRAINT chk_players_experience CHECK (experience >= 0),
    CONSTRAINT chk_players_coins CHECK (coins >= 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- items: master data managed by admins. Soft-deleted so history keeps its FK.
CREATE TABLE items (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name         VARCHAR(64)     NOT NULL,
    description  VARCHAR(255)    NOT NULL DEFAULT '',
    item_type    ENUM('coin', 'experience') NOT NULL,
    effect_value INT             NOT NULL,
    deleted_at   DATETIME(3)     NULL,
    created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_items_deleted_at (deleted_at),
    CONSTRAINT chk_items_effect_value CHECK (effect_value > 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- player_items: inventory. CHECK is the last line of defence; the application
-- decrements with a conditional UPDATE (... WHERE quantity >= ?).
CREATE TABLE player_items (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    player_id  BIGINT UNSIGNED NOT NULL,
    item_id    BIGINT UNSIGNED NOT NULL,
    quantity   INT             NOT NULL DEFAULT 0,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_player_items_player_item (player_id, item_id),
    KEY idx_player_items_item (item_id),
    CONSTRAINT fk_player_items_player FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE,
    CONSTRAINT fk_player_items_item FOREIGN KEY (item_id) REFERENCES items (id),
    CONSTRAINT chk_player_items_quantity CHECK (quantity >= 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- reward_claims: one row per player per JST calendar day. The UNIQUE key is
-- what makes 100 concurrent claims yield exactly one reward.
CREATE TABLE reward_claims (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    player_id   BIGINT UNSIGNED NOT NULL,
    reward_date DATE            NOT NULL,
    coins       INT             NOT NULL,
    experience  INT             NOT NULL,
    created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_reward_claims_player_date (player_id, reward_date),
    CONSTRAINT fk_reward_claims_player FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- item_transactions: append-only ledger of acquisitions and uses.
-- (player_id, idempotency_key) UNIQUE gives "use" requests exactly-once semantics;
-- NULL keys (system grants) are exempt from the UNIQUE check in MySQL.
CREATE TABLE item_transactions (
    id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    player_id        BIGINT UNSIGNED NOT NULL,
    item_id          BIGINT UNSIGNED NOT NULL,
    quantity         INT             NOT NULL,
    transaction_type ENUM('acquire', 'use') NOT NULL,
    idempotency_key  VARCHAR(64)     NULL,
    created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_item_transactions_idempotency (player_id, idempotency_key),
    KEY idx_item_transactions_player_created (player_id, created_at),
    KEY idx_item_transactions_item (item_id),
    CONSTRAINT fk_item_transactions_player FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE,
    CONSTRAINT fk_item_transactions_item FOREIGN KEY (item_id) REFERENCES items (id),
    CONSTRAINT chk_item_transactions_quantity CHECK (quantity > 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- audit_logs: who changed what through the admin API.
CREATE TABLE audit_logs (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    actor_user_id BIGINT UNSIGNED NOT NULL,
    action        VARCHAR(64)     NOT NULL,
    target_type   VARCHAR(32)     NOT NULL,
    target_id     BIGINT UNSIGNED NULL,
    detail        JSON            NULL,
    created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_audit_logs_actor_created (actor_user_id, created_at),
    KEY idx_audit_logs_created (created_at),
    CONSTRAINT fk_audit_logs_actor FOREIGN KEY (actor_user_id) REFERENCES users (id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;
