package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/takahironago/mini-game-backend/internal/database"
)

// ErrUserNotFound is returned when no user row matches.
var ErrUserNotFound = errors.New("user not found")

// Repository holds the admin-only SQL (cross-table reads and audit_logs).
type Repository struct{}

// NewRepository returns a Repository.
func NewRepository() *Repository { return &Repository{} }

const userSummaryColumns = `
	u.id, u.email, u.role, u.created_at,
	p.id, p.player_name, p.level, p.experience, p.coins`

func scanUserSummary(row interface{ Scan(dest ...any) error }) (UserSummary, error) {
	var (
		u    UserSummary
		pid  sql.NullInt64
		name sql.NullString
		lvl  sql.NullInt64
		exp  sql.NullInt64
		coin sql.NullInt64
	)
	err := row.Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt, &pid, &name, &lvl, &exp, &coin)
	if errors.Is(err, sql.ErrNoRows) {
		return UserSummary{}, ErrUserNotFound
	}
	if err != nil {
		return UserSummary{}, err
	}
	if pid.Valid {
		u.Player = &PlayerSummary{
			ID: pid.Int64, PlayerName: name.String, Level: int(lvl.Int64),
			Experience: exp.Int64, Coins: coin.Int64,
		}
	}
	return u, nil
}

// ListUsers returns one page of accounts (newest first) with their player,
// optionally filtered by a substring of the email or player name.
func (r *Repository) ListUsers(ctx context.Context, ex database.Executor, q string, limit, offset int) ([]UserSummary, int64, error) {
	where, args := "", []any{}
	if q != "" {
		where = ` WHERE u.email LIKE ? OR p.player_name LIKE ?`
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	var total int64
	if err := ex.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users u LEFT JOIN players p ON p.user_id = u.id`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := ex.QueryContext(ctx, `
		SELECT `+userSummaryColumns+`
		FROM users u LEFT JOIN players p ON p.user_id = u.id`+where+`
		ORDER BY u.id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	users := make([]UserSummary, 0, limit)
	for rows.Next() {
		u, err := scanUserSummary(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// FindUser returns one account with its player.
func (r *Repository) FindUser(ctx context.Context, ex database.Executor, id int64) (UserSummary, error) {
	return scanUserSummary(ex.QueryRowContext(ctx, `
		SELECT `+userSummaryColumns+`
		FROM users u LEFT JOIN players p ON p.user_id = u.id WHERE u.id = ?`, id))
}

// ListTransactions returns a player's most recent ledger rows.
func (r *Repository) ListTransactions(ctx context.Context, ex database.Executor, playerID int64, limit int) ([]TransactionView, error) {
	rows, err := ex.QueryContext(ctx, `
		SELECT t.id, t.item_id, i.name, t.quantity, t.transaction_type, t.idempotency_key, t.created_at
		FROM item_transactions t JOIN items i ON i.id = t.item_id
		WHERE t.player_id = ? ORDER BY t.id DESC LIMIT ?`, playerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TransactionView, 0, limit)
	for rows.Next() {
		var t TransactionView
		if err := rows.Scan(&t.ID, &t.ItemID, &t.ItemName, &t.Quantity, &t.Type, &t.IdempotencyKey, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CountRewardClaims returns how many daily rewards a player has claimed.
func (r *Repository) CountRewardClaims(ctx context.Context, ex database.Executor, playerID int64) (int, error) {
	var n int
	err := ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM reward_claims WHERE player_id = ?`, playerID).Scan(&n)
	return n, err
}

// DashboardCounts fills the numeric part of the dashboard. dayStart is the
// UTC instant at which the current JST day began; rewardDate the JST date.
func (r *Repository) DashboardCounts(ctx context.Context, ex database.Executor, dayStart time.Time, rewardDate string) (Dashboard, error) {
	var d Dashboard
	err := ex.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM players),
			(SELECT COUNT(*) FROM items WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM users WHERE created_at >= ?),
			(SELECT COUNT(*) FROM reward_claims WHERE reward_date = ?),
			(SELECT COUNT(*) FROM item_transactions WHERE transaction_type = 'use' AND created_at >= ?)`,
		dayStart.UTC(), rewardDate, dayStart.UTC()).
		Scan(&d.Users, &d.Players, &d.Items, &d.RegistrationsToday, &d.RewardClaimsToday, &d.ItemUsesToday)
	if err != nil {
		return Dashboard{}, err
	}
	d.RewardDate = rewardDate
	return d, nil
}

// InsertAudit records who did what. detail is JSON-encoded; nil stores NULL.
func (r *Repository) InsertAudit(ctx context.Context, ex database.Executor, actorUserID int64, action, targetType string, targetID *int64, detail any) error {
	var payload any
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("encode audit detail: %w", err)
		}
		payload = string(b)
	}
	_, err := ex.ExecContext(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, detail)
		VALUES (?, ?, ?, ?, ?)`, actorUserID, action, targetType, targetID, payload)
	return err
}

// ListAudit returns one page of the audit trail, newest first.
func (r *Repository) ListAudit(ctx context.Context, ex database.Executor, limit, offset int) ([]AuditLog, int64, error) {
	var total int64
	if err := ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := ex.QueryContext(ctx, `
		SELECT a.id, a.actor_user_id, u.email, a.action, a.target_type, a.target_id, a.detail, a.created_at
		FROM audit_logs a JOIN users u ON u.id = a.actor_user_id
		ORDER BY a.id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	logs := make([]AuditLog, 0, limit)
	for rows.Next() {
		var (
			l      AuditLog
			detail sql.NullString
		)
		if err := rows.Scan(&l.ID, &l.ActorUserID, &l.ActorEmail, &l.Action, &l.TargetType, &l.TargetID, &detail, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		if detail.Valid {
			l.Detail = json.RawMessage(detail.String)
		} else {
			l.Detail = json.RawMessage("null")
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}
