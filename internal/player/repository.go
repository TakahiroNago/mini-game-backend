package player

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/takahironago/mini-game-backend/internal/database"
)

// ErrPlayerNotFound is returned when a lookup matches no row.
var ErrPlayerNotFound = errors.New("player not found")

// Repository holds the SQL for players and player_items.
type Repository struct{}

// NewRepository returns a Repository.
func NewRepository() *Repository { return &Repository{} }

// Create inserts a level-1 player for userID. Duplicate player_name surfaces
// as MySQL 1062.
func (r *Repository) Create(ctx context.Context, ex database.Executor, userID int64, playerName string) (int64, error) {
	res, err := ex.ExecContext(ctx,
		`INSERT INTO players (user_id, player_name) VALUES (?, ?)`, userID, playerName)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

const playerColumns = `id, user_id, player_name, level, experience, coins, created_at, updated_at`

func scanPlayer(row interface{ Scan(dest ...any) error }) (Player, error) {
	var p Player
	err := row.Scan(&p.ID, &p.UserID, &p.PlayerName, &p.Level, &p.Experience, &p.Coins, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Player{}, ErrPlayerNotFound
	}
	return p, err
}

// FindByUserID returns the player owned by userID.
func (r *Repository) FindByUserID(ctx context.Context, ex database.Executor, userID int64) (Player, error) {
	return scanPlayer(ex.QueryRowContext(ctx, `SELECT `+playerColumns+` FROM players WHERE user_id = ?`, userID))
}

// FindByID returns a player by primary key.
func (r *Repository) FindByID(ctx context.Context, ex database.Executor, id int64) (Player, error) {
	return scanPlayer(ex.QueryRowContext(ctx, `SELECT `+playerColumns+` FROM players WHERE id = ?`, id))
}

// FindByIDForUpdate locks the player row for the rest of the transaction.
// Every mutation of level/experience/coins goes through this lock so two
// concurrent updates serialize instead of overwriting each other.
func (r *Repository) FindByIDForUpdate(ctx context.Context, tx *sql.Tx, id int64) (Player, error) {
	return scanPlayer(tx.QueryRowContext(ctx, `SELECT `+playerColumns+` FROM players WHERE id = ? FOR UPDATE`, id))
}

// UpdateProgress writes level, experience and coins for a locked player row.
func (r *Repository) UpdateProgress(ctx context.Context, ex database.Executor, id int64, level int, experience, coins int64) error {
	_, err := ex.ExecContext(ctx,
		`UPDATE players SET level = ?, experience = ?, coins = ? WHERE id = ?`,
		level, experience, coins, id)
	return err
}

// ListInventory returns the items a player owns (quantity > 0), including
// soft-deleted items so previously granted inventory stays visible.
func (r *Repository) ListInventory(ctx context.Context, ex database.Executor, playerID int64) ([]InventoryItem, error) {
	rows, err := ex.QueryContext(ctx, `
		SELECT i.id, i.name, i.description, i.item_type, i.effect_value, pi.quantity
		FROM player_items pi
		JOIN items i ON i.id = pi.item_id
		WHERE pi.player_id = ? AND pi.quantity > 0
		ORDER BY i.id`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]InventoryItem, 0)
	for rows.Next() {
		var it InventoryItem
		if err := rows.Scan(&it.ItemID, &it.Name, &it.Description, &it.ItemType, &it.EffectValue, &it.Quantity); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
