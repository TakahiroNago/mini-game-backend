package admin

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
	"github.com/takahironago/mini-game-backend/internal/item"
	"github.com/takahironago/mini-game-backend/internal/player"
	"github.com/takahironago/mini-game-backend/internal/ranking"
	"github.com/takahironago/mini-game-backend/internal/reward"
)

// API errors owned by this package.
var (
	ErrNoUser   = httpx.NewError(http.StatusNotFound, "USER_NOT_FOUND", "user does not exist")
	ErrNoPlayer = httpx.NewError(http.StatusNotFound, "PLAYER_NOT_FOUND", "player does not exist")
	ErrNoItem   = httpx.NewError(http.StatusNotFound, "ITEM_NOT_FOUND", "item does not exist or is deleted")
)

const (
	recentTransactions = 20
	topPlayers         = 5
	txAttempts         = 3
)

// Service implements the admin use cases.
type Service struct {
	db      *sql.DB
	repo    *Repository
	items   *item.Repository
	players *player.Repository
	ranks   *ranking.Repository
	now     func() time.Time
	log     *slog.Logger
}

// NewService wires the service. now is injectable for tests; nil means time.Now.
func NewService(db *sql.DB, repo *Repository, items *item.Repository, players *player.Repository, ranks *ranking.Repository, log *slog.Logger, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, repo: repo, items: items, players: players, ranks: ranks, now: now, log: log}
}

// Dashboard returns the landing-page summary. "Today" follows the game's
// JST calendar, the same day boundary the daily reward uses.
func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	now := s.now().In(reward.JST)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, reward.JST)
	d, err := s.repo.DashboardCounts(ctx, s.db, dayStart, reward.RewardDate(now))
	if err != nil {
		return Dashboard{}, fmt.Errorf("dashboard counts: %w", err)
	}
	d.TopPlayers, err = s.ranks.List(ctx, s.db, topPlayers, 0)
	if err != nil {
		return Dashboard{}, fmt.Errorf("top players: %w", err)
	}
	return d, nil
}

// ListUsers returns one page of accounts.
func (s *Service) ListUsers(ctx context.Context, q string, limit, offset int) (UsersPage, error) {
	users, total, err := s.repo.ListUsers(ctx, s.db, q, limit, offset)
	if err != nil {
		return UsersPage{}, fmt.Errorf("list users: %w", err)
	}
	return UsersPage{Users: users, Limit: limit, Offset: offset, Total: total}, nil
}

// GetUser returns one account with inventory and recent ledger rows.
func (s *Service) GetUser(ctx context.Context, id int64) (UserDetail, error) {
	u, err := s.repo.FindUser(ctx, s.db, id)
	if errors.Is(err, ErrUserNotFound) {
		return UserDetail{}, ErrNoUser
	}
	if err != nil {
		return UserDetail{}, fmt.Errorf("find user: %w", err)
	}
	d := UserDetail{User: u, Inventory: []player.InventoryItem{}, RecentTransactions: []TransactionView{}}
	if u.Player == nil {
		return d, nil
	}
	if d.Inventory, err = s.players.ListInventory(ctx, s.db, u.Player.ID); err != nil {
		return UserDetail{}, fmt.Errorf("inventory: %w", err)
	}
	if d.RecentTransactions, err = s.repo.ListTransactions(ctx, s.db, u.Player.ID, recentTransactions); err != nil {
		return UserDetail{}, fmt.Errorf("transactions: %w", err)
	}
	if d.RewardClaims, err = s.repo.CountRewardClaims(ctx, s.db, u.Player.ID); err != nil {
		return UserDetail{}, fmt.Errorf("reward claims: %w", err)
	}
	return d, nil
}

// ListItems returns the item master, optionally with soft-deleted rows.
func (s *Service) ListItems(ctx context.Context, includeDeleted bool) ([]item.Item, error) {
	return s.items.ListAll(ctx, s.db, includeDeleted)
}

// CreateItem adds a master item and audits it.
func (s *Service) CreateItem(ctx context.Context, actorUserID int64, in item.Input) (item.Item, error) {
	var created item.Item
	err := database.WithTx(ctx, s.db, nil, func(tx *sql.Tx) error {
		id, err := s.items.Create(ctx, tx, in)
		if err != nil {
			return fmt.Errorf("create item: %w", err)
		}
		if created, err = s.items.FindByID(ctx, tx, id); err != nil {
			return fmt.Errorf("reload item: %w", err)
		}
		return s.repo.InsertAudit(ctx, tx, actorUserID, ActionItemCreate, TargetItem, &id, map[string]any{"after": created})
	})
	if err != nil {
		return item.Item{}, err
	}
	s.log.InfoContext(ctx, "item created", "actor_user_id", actorUserID, "item_id", created.ID)
	return created, nil
}

// UpdateItem rewrites a live item and audits before/after.
func (s *Service) UpdateItem(ctx context.Context, actorUserID, id int64, in item.Input) (item.Item, error) {
	var updated item.Item
	err := database.WithTx(ctx, s.db, nil, func(tx *sql.Tx) error {
		before, err := s.items.FindByID(ctx, tx, id)
		if errors.Is(err, item.ErrNotFound) || (err == nil && before.DeletedAt != nil) {
			return ErrNoItem
		}
		if err != nil {
			return fmt.Errorf("find item: %w", err)
		}
		if err := s.items.Update(ctx, tx, id, in); err != nil {
			if errors.Is(err, item.ErrNotFound) {
				return ErrNoItem
			}
			return fmt.Errorf("update item: %w", err)
		}
		if updated, err = s.items.FindByID(ctx, tx, id); err != nil {
			return fmt.Errorf("reload item: %w", err)
		}
		return s.repo.InsertAudit(ctx, tx, actorUserID, ActionItemUpdate, TargetItem, &id,
			map[string]any{"before": before, "after": updated})
	})
	if err != nil {
		return item.Item{}, err
	}
	s.log.InfoContext(ctx, "item updated", "actor_user_id", actorUserID, "item_id", id)
	return updated, nil
}

// DeleteItem soft-deletes a live item and audits it. History (ledger rows,
// inventories) keeps referencing the row.
func (s *Service) DeleteItem(ctx context.Context, actorUserID, id int64) error {
	err := database.WithTx(ctx, s.db, nil, func(tx *sql.Tx) error {
		before, err := s.items.FindByID(ctx, tx, id)
		if errors.Is(err, item.ErrNotFound) || (err == nil && before.DeletedAt != nil) {
			return ErrNoItem
		}
		if err != nil {
			return fmt.Errorf("find item: %w", err)
		}
		if err := s.items.SoftDelete(ctx, tx, id, s.now()); err != nil {
			if errors.Is(err, item.ErrNotFound) {
				return ErrNoItem
			}
			return fmt.Errorf("delete item: %w", err)
		}
		return s.repo.InsertAudit(ctx, tx, actorUserID, ActionItemDelete, TargetItem, &id, map[string]any{"before": before})
	})
	if err != nil {
		return err
	}
	s.log.InfoContext(ctx, "item deleted", "actor_user_id", actorUserID, "item_id", id)
	return nil
}

// GrantItem hands a player extra units of an item (support / compensation).
// It follows the global lock order: players row first, then ledger and
// inventory, then the audit row, all in one transaction.
func (s *Service) GrantItem(ctx context.Context, actorUserID, playerID, itemID int64, quantity int) (GrantResult, error) {
	var res GrantResult
	err := database.WithTxRetry(ctx, s.db, txAttempts, func(tx *sql.Tx) error {
		if _, err := s.players.FindByIDForUpdate(ctx, tx, playerID); err != nil {
			if errors.Is(err, player.ErrPlayerNotFound) {
				return ErrNoPlayer
			}
			return fmt.Errorf("lock player: %w", err)
		}
		it, err := s.items.FindAvailable(ctx, tx, itemID)
		if errors.Is(err, item.ErrNotFound) {
			return ErrNoItem
		}
		if err != nil {
			return fmt.Errorf("find item: %w", err)
		}
		txID, err := s.items.InsertTransaction(ctx, tx, playerID, it.ID, quantity, item.TxAcquire, nil)
		if err != nil {
			return fmt.Errorf("record grant: %w", err)
		}
		if err := s.players.AddItem(ctx, tx, playerID, it.ID, quantity); err != nil {
			return fmt.Errorf("add item: %w", err)
		}
		res = GrantResult{PlayerID: playerID, ItemID: it.ID, Quantity: quantity, TransactionID: txID}
		return s.repo.InsertAudit(ctx, tx, actorUserID, ActionGrantItem, TargetPlayer, &playerID,
			map[string]any{"item_id": it.ID, "item_name": it.Name, "quantity": quantity, "transaction_id": txID})
	})
	if err != nil {
		return GrantResult{}, err
	}
	s.log.InfoContext(ctx, "item granted", "actor_user_id", actorUserID, "player_id", playerID, "item_id", itemID, "quantity", quantity)
	return res, nil
}

// ListAuditLogs returns one page of the audit trail.
func (s *Service) ListAuditLogs(ctx context.Context, limit, offset int) (AuditPage, error) {
	logs, total, err := s.repo.ListAudit(ctx, s.db, limit, offset)
	if err != nil {
		return AuditPage{}, fmt.Errorf("list audit logs: %w", err)
	}
	return AuditPage{Logs: logs, Limit: limit, Offset: offset, Total: total}, nil
}
