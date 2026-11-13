// Package ranking serves the leaderboard: players ordered by level, then
// experience, then id so the order is total and stable across pages.
package ranking

import (
	"context"

	"github.com/takahironago/mini-game-backend/internal/database"
	"github.com/takahironago/mini-game-backend/internal/player"
)

// Entry is one leaderboard row.
type Entry struct {
	Rank       int64  `json:"rank"`
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	Level      int    `json:"level"`
	Experience int64  `json:"experience"`
}

// Repository holds the leaderboard SQL.
type Repository struct{}

// NewRepository returns a Repository.
func NewRepository() *Repository { return &Repository{} }

// List returns one page. The ORDER BY matches idx_players_ranking exactly so
// InnoDB walks the index instead of sorting; ties are broken by id (older
// player first) which keeps the order deterministic between requests.
func (r *Repository) List(ctx context.Context, ex database.Executor, limit, offset int) ([]Entry, error) {
	rows, err := ex.QueryContext(ctx, `
		SELECT id, player_name, level, experience
		FROM players
		ORDER BY level DESC, experience DESC, id ASC
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]Entry, 0, limit)
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.PlayerID, &e.PlayerName, &e.Level, &e.Experience); err != nil {
			return nil, err
		}
		e.Rank = int64(offset) + int64(len(entries)) + 1
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Count returns the number of ranked players.
func (r *Repository) Count(ctx context.Context, ex database.Executor) (int64, error) {
	var n int64
	err := ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM players`).Scan(&n)
	return n, err
}

// RankOf computes a player's 1-based position with the same total order as
// List: 1 + the number of players strictly ahead.
func (r *Repository) RankOf(ctx context.Context, ex database.Executor, p player.Player) (int64, error) {
	var ahead int64
	err := ex.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM players
		WHERE level > ?
		   OR (level = ? AND experience > ?)
		   OR (level = ? AND experience = ? AND id < ?)`,
		p.Level, p.Level, p.Experience, p.Level, p.Experience, p.ID).Scan(&ahead)
	if err != nil {
		return 0, err
	}
	return ahead + 1, nil
}
