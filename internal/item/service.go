package item

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/takahironago/mini-game-backend/internal/database"
	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/player"
)

// API errors owned by this package.
var (
	ErrItemNotFound     = httpx.NewError(http.StatusNotFound, "ITEM_NOT_FOUND", "item does not exist or is no longer available")
	ErrInsufficientItem = httpx.NewError(http.StatusConflict, "INSUFFICIENT_ITEM", "not enough of this item is owned")
	ErrKeyMismatch      = httpx.NewError(http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_MISMATCH", "Idempotency-Key was already used with a different request")
)

// errReplay signals, inside the transaction, that the key was already used.
var errReplay = errors.New("idempotency key already used")

// txAttempts bounds deadlock retries for the use transaction.
const txAttempts = 3

// Service implements the item catalogue, starter grants and item use.
type Service struct {
	db      *sql.DB
	items   *Repository
	players *player.Repository
	log     *slog.Logger
}

// NewService wires the service.
func NewService(db *sql.DB, items *Repository, players *player.Repository, log *slog.Logger) *Service {
	return &Service{db: db, items: items, players: players, log: log}
}

// List returns the catalogue of usable items.
func (s *Service) List(ctx context.Context) ([]Item, error) {
	return s.items.ListAvailable(ctx, s.db)
}

// GrantStarterItems gives a brand-new player every starter item. It runs in
// the caller's transaction (registration) so the player and its inventory
// appear atomically; the grants are recorded as 'acquire' ledger rows.
func (s *Service) GrantStarterItems(ctx context.Context, tx *sql.Tx, playerID int64) error {
	starters, err := s.items.ListStarter(ctx, tx)
	if err != nil {
		return fmt.Errorf("list starter items: %w", err)
	}
	for _, it := range starters {
		if err := s.players.AddItem(ctx, tx, playerID, it.ID, it.StarterQuantity); err != nil {
			return fmt.Errorf("grant item %d: %w", it.ID, err)
		}
		if _, err := s.items.InsertTransaction(ctx, tx, playerID, it.ID, it.StarterQuantity, TxAcquire, nil); err != nil {
			return fmt.Errorf("record grant %d: %w", it.ID, err)
		}
	}
	return nil
}

// UseResult is the outcome of an item use (or of its idempotent replay).
type UseResult struct {
	TransactionID int64
	ItemID        int64
	Quantity      int
	Progress      player.Progress
	// Replayed is true when the Idempotency-Key had already been processed
	// and the stored transaction is returned instead of applying it again.
	Replayed bool
}

// Use consumes quantity units of itemID for the player owned by userID and
// applies the item's effect, exactly once per (player, idempotencyKey).
//
// Transaction order matters:
//  1. SELECT ... FOR UPDATE on the players row first. Every mutation of a
//     player (item use, reward) starts with this lock, so concurrent
//     requests for one player serialise here and can never deadlock later.
//     Inserting the ledger row first looks cheaper but is a deadlock trap:
//     the FK check of that INSERT takes a *shared* lock on the players row,
//     and two requests both holding S and both wanting X deadlock.
//  2. INSERT the ledger row. Its UNIQUE (player_id, idempotency_key) is the
//     exactly-once guarantee; a duplicate is answered from the stored row.
//  3. Conditional UPDATE of player_items (quantity >= n), the only guard
//     against over-use; no application-side pre-check is trusted.
//  4. Write the new progression.
//
// Any failure rolls everything back, including the ledger row.
func (s *Service) Use(ctx context.Context, userID, itemID int64, quantity int, idempotencyKey string) (UseResult, error) {
	p, err := s.players.FindByUserID(ctx, s.db, userID)
	if errors.Is(err, player.ErrPlayerNotFound) {
		return UseResult{}, player.ErrNoPlayer
	}
	if err != nil {
		return UseResult{}, fmt.Errorf("find player: %w", err)
	}

	var res UseResult
	err = database.WithTxRetry(ctx, s.db, txAttempts, func(tx *sql.Tx) error {
		locked, err := s.players.FindByIDForUpdate(ctx, tx, p.ID)
		if err != nil {
			return fmt.Errorf("lock player: %w", err)
		}

		it, err := s.items.FindAvailable(ctx, tx, itemID)
		if errors.Is(err, ErrNotFound) {
			return ErrItemNotFound
		}
		if err != nil {
			return fmt.Errorf("find item: %w", err)
		}

		txID, err := s.items.InsertTransaction(ctx, tx, p.ID, it.ID, quantity, TxUse, &idempotencyKey)
		if database.IsDuplicateEntry(err) {
			return errReplay
		}
		if err != nil {
			return fmt.Errorf("insert transaction: %w", err)
		}

		if err := s.players.RemoveItem(ctx, tx, p.ID, it.ID, quantity); err != nil {
			if errors.Is(err, player.ErrInsufficientItem) {
				return ErrInsufficientItem
			}
			return fmt.Errorf("remove item: %w", err)
		}

		next := ApplyEffect(locked.Progress(), it, quantity)
		if err := s.players.UpdateProgress(ctx, tx, p.ID, next.Level, next.Experience, next.Coins); err != nil {
			return fmt.Errorf("update progress: %w", err)
		}
		res = UseResult{TransactionID: txID, ItemID: it.ID, Quantity: quantity, Progress: next}
		return nil
	})
	if errors.Is(err, errReplay) {
		return s.replay(ctx, p.ID, itemID, quantity, idempotencyKey)
	}
	if err != nil {
		return UseResult{}, err
	}
	s.log.InfoContext(ctx, "item used", "player_id", p.ID, "item_id", itemID, "quantity", quantity,
		"transaction_id", res.TransactionID)
	return res, nil
}

// replay answers a repeated request: same key + same payload returns the
// original transaction; same key + different payload is a client bug (422).
func (s *Service) replay(ctx context.Context, playerID, itemID int64, quantity int, key string) (UseResult, error) {
	t, err := s.items.FindTransactionByKey(ctx, s.db, playerID, key)
	if err != nil {
		return UseResult{}, fmt.Errorf("find transaction for replay: %w", err)
	}
	if t.ItemID != itemID || t.Quantity != quantity || t.Type != TxUse {
		return UseResult{}, ErrKeyMismatch
	}
	cur, err := s.players.FindByID(ctx, s.db, playerID)
	if err != nil {
		return UseResult{}, fmt.Errorf("find player: %w", err)
	}
	return UseResult{
		TransactionID: t.ID, ItemID: t.ItemID, Quantity: t.Quantity,
		Progress: cur.Progress(), Replayed: true,
	}, nil
}
