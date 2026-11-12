package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/takahironago/mini-game-backend/internal/database"
	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/player"
)

// API errors owned by this package.
var (
	ErrEmailTaken      = httpx.NewError(http.StatusConflict, "EMAIL_ALREADY_EXISTS", "email is already registered")
	ErrPlayerNameTaken = httpx.NewError(http.StatusConflict, "PLAYER_NAME_ALREADY_EXISTS", "player name is already taken")
	ErrInvalidLogin    = httpx.NewError(http.StatusUnauthorized, "INVALID_CREDENTIALS", "email or password is incorrect")
)

// Service implements registration, login, logout and the current-user lookup.
type Service struct {
	db      *sql.DB
	users   *Repository
	players *player.Repository
	tokens  *TokenIssuer
	now     func() time.Time
	log     *slog.Logger
}

// NewService wires the service. now is injectable for tests; nil means time.Now.
func NewService(db *sql.DB, users *Repository, players *player.Repository, tokens *TokenIssuer, log *slog.Logger, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, users: users, players: players, tokens: tokens, now: now, log: log}
}

// RegisterResult is returned after a successful registration.
type RegisterResult struct {
	UserID   int64
	PlayerID int64
}

// Register creates the user and its player atomically: either both rows
// exist afterwards or neither does.
func (s *Service) Register(ctx context.Context, email, password, playerName string) (RegisterResult, error) {
	email = normalizeEmail(email)
	hash, err := HashPassword(password)
	if err != nil {
		return RegisterResult{}, httpx.NewError(http.StatusBadRequest, "VALIDATION_ERROR", "password length out of range")
	}

	var res RegisterResult
	err = database.WithTx(ctx, s.db, nil, func(tx *sql.Tx) error {
		userID, err := s.users.CreateUser(ctx, tx, email, hash, RolePlayer)
		if database.IsDuplicateEntry(err) {
			return ErrEmailTaken
		}
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		playerID, err := s.players.Create(ctx, tx, userID, playerName)
		if database.IsDuplicateEntry(err) {
			return ErrPlayerNameTaken
		}
		if err != nil {
			return fmt.Errorf("create player: %w", err)
		}
		res = RegisterResult{UserID: userID, PlayerID: playerID}
		return nil
	})
	if err != nil {
		return RegisterResult{}, err
	}
	s.log.InfoContext(ctx, "user registered", "user_id", res.UserID, "player_id", res.PlayerID)
	return res, nil
}

// LoginResult is the issued token plus its metadata.
type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      User
}

// Login verifies credentials and issues an access token. Unknown email and
// wrong password are indistinguishable to the client, and both paths run a
// bcrypt comparison so response time does not leak which one happened.
func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	u, err := s.users.FindUserByEmail(ctx, s.db, normalizeEmail(email))
	if errors.Is(err, ErrUserNotFound) {
		CheckPassword(dummyHash, password)
		return LoginResult{}, ErrInvalidLogin
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("find user: %w", err)
	}
	if !CheckPassword(u.PasswordHash, password) {
		return LoginResult{}, ErrInvalidLogin
	}
	token, claims, err := s.tokens.Issue(u.ID, u.Role)
	if err != nil {
		return LoginResult{}, err
	}
	s.log.InfoContext(ctx, "user logged in", "user_id", u.ID, "role", u.Role)
	return LoginResult{Token: token, ExpiresAt: claims.ExpiresAt, User: u}, nil
}

// Logout revokes the presented token until its natural expiry and takes the
// opportunity to purge revocation rows that can no longer matter.
func (s *Service) Logout(ctx context.Context, c Claims) error {
	if err := s.users.RevokeToken(ctx, s.db, c.JTI, c.UserID, c.ExpiresAt); err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if err := s.users.PurgeExpiredTokens(ctx, s.db, s.now()); err != nil {
		// Best effort: a failed purge must not fail the logout.
		s.log.WarnContext(ctx, "purge expired tokens", "error", err)
	}
	return nil
}

// IsRevoked satisfies middleware.RevocationChecker.
func (s *Service) IsRevoked(ctx context.Context, jti string) (bool, error) {
	return s.users.IsTokenRevoked(ctx, s.db, jti)
}

// CurrentUser returns the user behind a verified token.
func (s *Service) CurrentUser(ctx context.Context, userID int64) (User, error) {
	u, err := s.users.FindUserByID(ctx, s.db, userID)
	if errors.Is(err, ErrUserNotFound) {
		// Token is valid but the account is gone: treat as unauthenticated.
		return User{}, httpx.ErrUnauthorized
	}
	return u, err
}

// EnsureAdmin creates the bootstrap admin account from configuration when
// no user with that email exists. It is idempotent so it can run on every
// start-up. Admins have no player row; the admin API does not need one.
func (s *Service) EnsureAdmin(ctx context.Context, email, password string) error {
	if email == "" || password == "" {
		return nil
	}
	email = normalizeEmail(email)
	_, err := s.users.FindUserByEmail(ctx, s.db, email)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrUserNotFound) {
		return fmt.Errorf("find admin: %w", err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("admin password: %w", err)
	}
	if _, err := s.users.CreateUser(ctx, s.db, email, hash, RoleAdmin); err != nil {
		if database.IsDuplicateEntry(err) {
			return nil // raced with another replica; the admin exists now
		}
		return fmt.Errorf("create admin: %w", err)
	}
	s.log.Info("bootstrap admin created")
	return nil
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }
