package reward

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/takahironago/mini-game-backend/internal/database"
	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/player"
)

// ErrAlreadyClaimed is returned for the second and later claims of a day.
var ErrAlreadyClaimed = httpx.NewError(http.StatusConflict, "REWARD_ALREADY_CLAIMED", "today's reward has already been claimed")

// txAttempts bounds deadlock retries for the claim transaction.
const txAttempts = 3

// Service implements claiming and querying the daily reward.
type Service struct {
	db      *sql.DB
	claims  *Repository
	players *player.Repository
	now     func() time.Time
	log     *slog.Logger
}

// NewService wires the service. now is injectable for tests; nil means time.Now.
func NewService(db *sql.DB, claims *Repository, players *player.Repository, log *slog.Logger, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, claims: claims, players: players, now: now, log: log}
}

// ClaimResult is the granted reward plus the player's new progression.
type ClaimResult struct {
	ClaimID    int64
	RewardDate string
	Coins      int
	Experience int
	Progress   player.Progress
}

// Claim grants today's reward to the player owned by userID.
//
// The players row is locked (FOR UPDATE) before the INSERT, the same order
// item use follows, so all mutations of one player serialise on that lock.
// Concurrent claims queue behind the winner and then fail on the UNIQUE
// (player_id, reward_date) key, which is the actual exactly-once guarantee;
// the application never asks "already claimed?" before inserting.
func (s *Service) Claim(ctx context.Context, userID int64) (ClaimResult, error) {
	p, err := s.players.FindByUserID(ctx, s.db, userID)
	if errors.Is(err, player.ErrPlayerNotFound) {
		return ClaimResult{}, player.ErrNoPlayer
	}
	if err != nil {
		return ClaimResult{}, fmt.Errorf("find player: %w", err)
	}
	day := RewardDate(s.now())

	var res ClaimResult
	err = database.WithTxRetry(ctx, s.db, txAttempts, func(tx *sql.Tx) error {
		locked, err := s.players.FindByIDForUpdate(ctx, tx, p.ID)
		if err != nil {
			return fmt.Errorf("lock player: %w", err)
		}
		claimID, err := s.claims.InsertClaim(ctx, tx, p.ID, day, DailyCoins, DailyExperience)
		if database.IsDuplicateEntry(err) {
			return ErrAlreadyClaimed
		}
		if err != nil {
			return fmt.Errorf("insert claim: %w", err)
		}
		level, exp := player.ApplyExperience(locked.Level, locked.Experience, DailyExperience)
		coins := locked.Coins + DailyCoins
		if err := s.players.UpdateProgress(ctx, tx, p.ID, level, exp, coins); err != nil {
			return fmt.Errorf("update progress: %w", err)
		}
		res = ClaimResult{
			ClaimID: claimID, RewardDate: day, Coins: DailyCoins, Experience: DailyExperience,
			Progress: player.Progress{Level: level, Experience: exp, Coins: coins},
		}
		return nil
	})
	if err != nil {
		return ClaimResult{}, err
	}
	s.log.InfoContext(ctx, "daily reward claimed", "player_id", p.ID, "reward_date", day)
	return res, nil
}

// Status reports whether today's reward is still available.
type Status struct {
	RewardDate string `json:"reward_date"`
	Claimed    bool   `json:"claimed"`
	Coins      int    `json:"coins"`
	Experience int    `json:"experience"`
}

// Status returns today's claim state for the player owned by userID.
func (s *Service) Status(ctx context.Context, userID int64) (Status, error) {
	p, err := s.players.FindByUserID(ctx, s.db, userID)
	if errors.Is(err, player.ErrPlayerNotFound) {
		return Status{}, player.ErrNoPlayer
	}
	if err != nil {
		return Status{}, fmt.Errorf("find player: %w", err)
	}
	day := RewardDate(s.now())
	claimed, err := s.claims.HasClaimed(ctx, s.db, p.ID, day)
	if err != nil {
		return Status{}, fmt.Errorf("has claimed: %w", err)
	}
	return Status{RewardDate: day, Claimed: claimed, Coins: DailyCoins, Experience: DailyExperience}, nil
}
