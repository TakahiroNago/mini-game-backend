package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/takahironago/mini-game-backend/internal/database"
)

// User is the users table row.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ErrUserNotFound is returned when a lookup matches no row.
var ErrUserNotFound = errors.New("user not found")

// Repository holds the SQL for users and revoked_tokens.
type Repository struct{}

// NewRepository returns a Repository.
func NewRepository() *Repository { return &Repository{} }

// CreateUser inserts a user and returns its id. A duplicate email surfaces
// as a MySQL 1062 error; the service maps it to a 409.
func (r *Repository) CreateUser(ctx context.Context, ex database.Executor, email, passwordHash, role string) (int64, error) {
	res, err := ex.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, role) VALUES (?, ?, ?)`,
		email, passwordHash, role)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

const userColumns = `id, email, password_hash, role, created_at, updated_at`

func scanUser(row interface{ Scan(dest ...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return u, err
}

// FindUserByEmail looks a user up by email.
func (r *Repository) FindUserByEmail(ctx context.Context, ex database.Executor, email string) (User, error) {
	return scanUser(ex.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE email = ?`, email))
}

// FindUserByID looks a user up by primary key.
func (r *Repository) FindUserByID(ctx context.Context, ex database.Executor, id int64) (User, error) {
	return scanUser(ex.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

// RevokeToken records a jti as revoked until the token would have expired
// anyway. Re-revoking the same jti is a no-op (INSERT IGNORE on the PK).
func (r *Repository) RevokeToken(ctx context.Context, ex database.Executor, jti string, userID int64, expiresAt time.Time) error {
	_, err := ex.ExecContext(ctx,
		`INSERT IGNORE INTO revoked_tokens (jti, user_id, expires_at) VALUES (?, ?, ?)`,
		jti, userID, expiresAt.UTC())
	return err
}

// IsTokenRevoked reports whether jti has been revoked.
func (r *Repository) IsTokenRevoked(ctx context.Context, ex database.Executor, jti string) (bool, error) {
	var n int
	err := ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM revoked_tokens WHERE jti = ?`, jti).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// PurgeExpiredTokens deletes revocation rows whose token is already expired.
func (r *Repository) PurgeExpiredTokens(ctx context.Context, ex database.Executor, now time.Time) error {
	_, err := ex.ExecContext(ctx, `DELETE FROM revoked_tokens WHERE expires_at < ?`, now.UTC())
	return err
}
