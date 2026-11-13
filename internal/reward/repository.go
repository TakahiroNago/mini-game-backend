package reward

import (
	"context"
	"fmt"

	"github.com/takahironago/mini-game-backend/internal/database"
)

// Repository holds the SQL for reward_claims.
type Repository struct{}

// NewRepository returns a Repository.
func NewRepository() *Repository { return &Repository{} }

// InsertClaim records today's claim. A second claim for the same
// (player, day) violates the UNIQUE key and surfaces as MySQL 1062.
func (r *Repository) InsertClaim(ctx context.Context, ex database.Executor, playerID int64, rewardDate string, coins, experience int) (int64, error) {
	res, err := ex.ExecContext(ctx,
		`INSERT INTO reward_claims (player_id, reward_date, coins, experience) VALUES (?, ?, ?, ?)`,
		playerID, rewardDate, coins, experience)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// HasClaimed reports whether the player already claimed on rewardDate.
func (r *Repository) HasClaimed(ctx context.Context, ex database.Executor, playerID int64, rewardDate string) (bool, error) {
	var n int
	err := ex.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reward_claims WHERE player_id = ? AND reward_date = ?`,
		playerID, rewardDate).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
