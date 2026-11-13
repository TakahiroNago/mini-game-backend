// Package admin is the operations API behind the React admin UI: user and
// player inspection, item master CRUD, manual grants and the audit trail.
// Every route requires the admin role; every mutation writes audit_logs in
// the same transaction as the change.
package admin

import (
	"encoding/json"
	"time"

	"github.com/takahironago/mini-game-backend/internal/player"
	"github.com/takahironago/mini-game-backend/internal/ranking"
)

// Audit actions recorded in audit_logs.action.
const (
	ActionItemCreate = "item.create"
	ActionItemUpdate = "item.update"
	ActionItemDelete = "item.delete"
	ActionGrantItem  = "player.grant_item"
)

// Target types recorded in audit_logs.target_type.
const (
	TargetItem   = "item"
	TargetPlayer = "player"
)

// PlayerSummary is the progression slice shown next to a user.
type PlayerSummary struct {
	ID         int64  `json:"id"`
	PlayerName string `json:"player_name"`
	Level      int    `json:"level"`
	Experience int64  `json:"experience"`
	Coins      int64  `json:"coins"`
}

// UserSummary is one row of the users list. Player is nil for admins.
type UserSummary struct {
	ID        int64          `json:"id"`
	Email     string         `json:"email"`
	Role      string         `json:"role"`
	CreatedAt time.Time      `json:"created_at"`
	Player    *PlayerSummary `json:"player"`
}

// UsersPage is a paginated users list.
type UsersPage struct {
	Users  []UserSummary `json:"users"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
	Total  int64         `json:"total"`
}

// TransactionView is a ledger row joined with the item name.
type TransactionView struct {
	ID             int64     `json:"id"`
	ItemID         int64     `json:"item_id"`
	ItemName       string    `json:"item_name"`
	Quantity       int       `json:"quantity"`
	Type           string    `json:"transaction_type"`
	IdempotencyKey *string   `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

// UserDetail is the full view of one account.
type UserDetail struct {
	User               UserSummary            `json:"user"`
	Inventory          []player.InventoryItem `json:"inventory"`
	RecentTransactions []TransactionView      `json:"recent_transactions"`
	RewardClaims       int                    `json:"reward_claims"`
}

// Dashboard is the landing-page summary.
type Dashboard struct {
	Users              int64           `json:"users"`
	Players            int64           `json:"players"`
	Items              int64           `json:"items"`
	RegistrationsToday int64           `json:"registrations_today"`
	RewardClaimsToday  int64           `json:"reward_claims_today"`
	ItemUsesToday      int64           `json:"item_uses_today"`
	RewardDate         string          `json:"reward_date"`
	TopPlayers         []ranking.Entry `json:"top_players"`
}

// GrantResult is the outcome of a manual item grant.
type GrantResult struct {
	PlayerID      int64 `json:"player_id"`
	ItemID        int64 `json:"item_id"`
	Quantity      int   `json:"quantity"`
	TransactionID int64 `json:"transaction_id"`
}

// AuditLog is one audit_logs row joined with the actor's email.
type AuditLog struct {
	ID          int64           `json:"id"`
	ActorUserID int64           `json:"actor_user_id"`
	ActorEmail  string          `json:"actor_email"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    *int64          `json:"target_id"`
	Detail      json.RawMessage `json:"detail"`
	CreatedAt   time.Time       `json:"created_at"`
}

// AuditPage is a paginated audit trail.
type AuditPage struct {
	Logs   []AuditLog `json:"logs"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
	Total  int64      `json:"total"`
}
