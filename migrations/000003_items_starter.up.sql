-- starter_quantity: how many of this item every new player receives at
-- registration (0 = not a starter item). Admins tune it through the item API;
-- the grant itself is recorded in item_transactions as an 'acquire' row with
-- no idempotency key, inside the same transaction that creates the player.
ALTER TABLE items
    ADD COLUMN starter_quantity INT NOT NULL DEFAULT 0 AFTER effect_value,
    ADD CONSTRAINT chk_items_starter_quantity CHECK (starter_quantity >= 0);

-- Initial master data so a fresh environment is playable without the admin UI.
INSERT INTO items (name, description, item_type, effect_value, starter_quantity) VALUES
    ('Small Coin Pouch',  'A pouch holding 100 coins.',          'coin',       100, 3),
    ('Large Coin Pouch',  'A heavy pouch holding 500 coins.',    'coin',       500, 0),
    ('Experience Potion', 'Grants 50 experience points.',        'experience',  50, 3),
    ('Elixir of Wisdom',  'Grants 300 experience points.',       'experience', 300, 0);
