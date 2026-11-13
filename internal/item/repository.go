package item

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/takahironago/mini-game-backend/internal/database"
)

// Sentinel errors; the service maps them to API errors.
var (
	ErrNotFound            = errors.New("item not found")
	ErrTransactionNotFound = errors.New("item transaction not found")
)

// Repository holds the SQL for items and item_transactions.
type Repository struct{}

// NewRepository returns a Repository.
func NewRepository() *Repository { return &Repository{} }

const itemColumns = `id, name, description, item_type, effect_value, starter_quantity, created_at, updated_at`

func scanItem(row interface{ Scan(dest ...any) error }) (Item, error) {
	var it Item
	err := row.Scan(&it.ID, &it.Name, &it.Description, &it.ItemType, &it.EffectValue, &it.StarterQuantity, &it.CreatedAt, &it.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return it, err
}

func collectItems(rows *sql.Rows) ([]Item, error) {
	defer rows.Close()
	items := make([]Item, 0)
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// ListAvailable returns every item that has not been soft-deleted.
func (r *Repository) ListAvailable(ctx context.Context, ex database.Executor) ([]Item, error) {
	rows, err := ex.QueryContext(ctx, `SELECT `+itemColumns+` FROM items WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return collectItems(rows)
}

// ListStarter returns the available items granted to new players.
func (r *Repository) ListStarter(ctx context.Context, ex database.Executor) ([]Item, error) {
	rows, err := ex.QueryContext(ctx,
		`SELECT `+itemColumns+` FROM items WHERE deleted_at IS NULL AND starter_quantity > 0 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return collectItems(rows)
}

// FindAvailable returns an item by id, treating soft-deleted ones as absent.
func (r *Repository) FindAvailable(ctx context.Context, ex database.Executor, id int64) (Item, error) {
	return scanItem(ex.QueryRowContext(ctx,
		`SELECT `+itemColumns+` FROM items WHERE id = ? AND deleted_at IS NULL`, id))
}

// InsertTransaction appends a ledger row. A non-nil idempotencyKey that the
// player already used surfaces as MySQL 1062 (UNIQUE player_id, key).
func (r *Repository) InsertTransaction(ctx context.Context, ex database.Executor, playerID, itemID int64, quantity int, txType string, idempotencyKey *string) (int64, error) {
	res, err := ex.ExecContext(ctx, `
		INSERT INTO item_transactions (player_id, item_id, quantity, transaction_type, idempotency_key)
		VALUES (?, ?, ?, ?, ?)`,
		playerID, itemID, quantity, txType, idempotencyKey)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// FindTransactionByKey returns the ledger row recorded for a player's
// idempotency key.
func (r *Repository) FindTransactionByKey(ctx context.Context, ex database.Executor, playerID int64, key string) (Transaction, error) {
	var t Transaction
	err := ex.QueryRowContext(ctx, `
		SELECT id, player_id, item_id, quantity, transaction_type, idempotency_key, created_at
		FROM item_transactions WHERE player_id = ? AND idempotency_key = ?`, playerID, key).
		Scan(&t.ID, &t.PlayerID, &t.ItemID, &t.Quantity, &t.Type, &t.IdempotencyKey, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Transaction{}, ErrTransactionNotFound
	}
	return t, err
}
