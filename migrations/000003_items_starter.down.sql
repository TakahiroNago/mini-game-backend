-- Remove the seeded master data, but only rows nothing references yet:
-- item_transactions / player_items carry FKs to items, so rows already in
-- play are kept (and the column drop below still succeeds).
DELETE i FROM items i
    LEFT JOIN player_items pi ON pi.item_id = i.id
    LEFT JOIN item_transactions t ON t.item_id = i.id
WHERE pi.id IS NULL AND t.id IS NULL
  AND i.name IN ('Small Coin Pouch', 'Large Coin Pouch', 'Experience Potion', 'Elixir of Wisdom');

ALTER TABLE items
    DROP CONSTRAINT chk_items_starter_quantity,
    DROP COLUMN starter_quantity;
