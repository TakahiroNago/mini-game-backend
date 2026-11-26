-- Demo seed, loaded by reset.sh right after the migrations on an empty schema.
-- It only fills the ranking so a visitor's first screen is not empty.
--
-- Seed users cannot log in: '!' is not a bcrypt hash, so every password
-- comparison fails. The admin account is created by the API on start-up.

CREATE TEMPORARY TABLE seed_players (
    player_name VARCHAR(32) NOT NULL,
    level       INT         NOT NULL,
    experience  BIGINT      NOT NULL, -- progress inside the level: < level * 100
    coins       BIGINT      NOT NULL
);

INSERT INTO seed_players (player_name, level, experience, coins) VALUES
    ('Aoi',     12, 340, 8200),
    ('Haruto',   9, 610, 5400),
    ('Mio',      9, 120, 6100),
    ('Ren',      7, 455, 3900),
    ('Yuna',     6,  80, 2700),
    ('Sora',     5, 310, 2300),
    ('Kaito',    4, 150, 1500),
    ('Hina',     3, 220, 1100),
    ('Riku',     3,  40,  900),
    ('Akari',    2, 130,  600),
    ('Nagi',     2,  10,  400),
    ('Tsumugi',  1,  60,  200);

INSERT INTO users (email, password_hash, role)
SELECT CONCAT('seed-', LOWER(player_name), '@example.com'), '!', 'player'
FROM seed_players;

INSERT INTO players (user_id, player_name, level, experience, coins)
SELECT u.id, s.player_name, s.level, s.experience, s.coins
FROM seed_players s
JOIN users u ON u.email = CONCAT('seed-', LOWER(s.player_name), '@example.com');

-- Same starter grant a real registration performs: inventory plus the
-- matching 'acquire' ledger rows.
INSERT INTO player_items (player_id, item_id, quantity)
SELECT p.id, i.id, i.starter_quantity
FROM players p
JOIN items i ON i.starter_quantity > 0 AND i.deleted_at IS NULL;

INSERT INTO item_transactions (player_id, item_id, quantity, transaction_type)
SELECT player_id, item_id, quantity, 'acquire'
FROM player_items;

DROP TEMPORARY TABLE seed_players;
