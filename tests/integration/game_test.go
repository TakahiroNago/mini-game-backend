//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/takahironago/mini-game-backend/internal/item"
	"github.com/takahironago/mini-game-backend/internal/player"
	"github.com/takahironago/mini-game-backend/internal/reward"
)

// Item ids used by seedItems. resetDB truncates items (the seed migration's
// rows included), so every game test inserts a known catalogue itself.
const (
	itemCoinPouch = 1 // coin x100, starter x3
	itemPotion    = 2 // experience x50, starter x3
	itemElixir    = 3 // experience x300, not a starter
	itemRetired   = 4 // soft-deleted
)

func seedItems(t *testing.T) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(), `
		INSERT INTO items (id, name, description, item_type, effect_value, starter_quantity, deleted_at) VALUES
		(1, 'Small Coin Pouch',  '', 'coin',       100, 3, NULL),
		(2, 'Experience Potion', '', 'experience',  50, 3, NULL),
		(3, 'Elixir of Wisdom',  '', 'experience', 300, 0, NULL),
		(4, 'Retired Item',      '', 'coin',         1, 0, NOW(3))`)
	if err != nil {
		t.Fatalf("seed items: %v", err)
	}
}

func useItem(t *testing.T, h http.Handler, token, key string, itemID int64, quantity int) (int, map[string]any) {
	t.Helper()
	var headers map[string]string
	if key != "" {
		headers = map[string]string{item.IdempotencyKeyHeader: key}
	}
	return callWith(t, h, http.MethodPost, "/api/v1/items/use", token, headers,
		map[string]any{"item_id": itemID, "quantity": quantity})
}

// inventory returns item_id -> quantity as reported by /player/me/items.
func inventory(t *testing.T, h http.Handler, token string) map[int64]int {
	t.Helper()
	status, body := call(t, h, http.MethodGet, "/api/v1/player/me/items", token, nil)
	if status != http.StatusOK {
		t.Fatalf("player/me/items: %d %v", status, body)
	}
	out := map[int64]int{}
	for _, raw := range body["items"].([]any) {
		it := raw.(map[string]any)
		out[int64(it["item_id"].(float64))] = int(it["quantity"].(float64))
	}
	return out
}

func countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := testDB.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func playerProgress(t *testing.T, h http.Handler, token string) player.Progress {
	t.Helper()
	status, body := call(t, h, http.MethodGet, "/api/v1/player/me", token, nil)
	if status != http.StatusOK {
		t.Fatalf("player/me: %d %v", status, body)
	}
	return player.Progress{
		Level:      int(body["level"].(float64)),
		Experience: int64(body["experience"].(float64)),
		Coins:      int64(body["coins"].(float64)),
	}
}

func progressOf(body map[string]any) player.Progress {
	p := body["player"].(map[string]any)
	return player.Progress{
		Level:      int(p["level"].(float64)),
		Experience: int64(p["experience"].(float64)),
		Coins:      int64(p["coins"].(float64)),
	}
}

func TestStarterItemsAndCatalogue(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")

	inv := inventory(t, app, tok)
	if len(inv) != 2 || inv[itemCoinPouch] != 3 || inv[itemPotion] != 3 {
		t.Fatalf("starter inventory = %v, want {1:3 2:3}", inv)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM item_transactions WHERE transaction_type = 'acquire' AND idempotency_key IS NULL`); n != 2 {
		t.Fatalf("acquire ledger rows = %d, want 2", n)
	}

	status, body := call(t, app, http.MethodGet, "/api/v1/items", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("items: %d %v", status, body)
	}
	items := body["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("catalogue has %d items, want 3 (soft-deleted hidden): %v", len(items), items)
	}
	for _, raw := range items {
		if id := raw.(map[string]any)["id"].(float64); id == itemRetired {
			t.Fatal("soft-deleted item must not be listed")
		}
	}
}

func TestItemUse(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, svcs := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")

	// Header and body validation never reach the database.
	if status, body := useItem(t, app, tok, "", itemPotion, 1); status != http.StatusBadRequest || errCode(body) != "IDEMPOTENCY_KEY_REQUIRED" {
		t.Fatalf("missing key: %d %v", status, body)
	}
	if status, body := useItem(t, app, tok, "k0", itemPotion, -1); status != http.StatusBadRequest || errCode(body) != "VALIDATION_ERROR" {
		t.Fatalf("negative quantity: %d %v", status, body)
	}

	// Two potions: 100 exp takes a level-1 player exactly to level 2.
	status, body := useItem(t, app, tok, "k1", itemPotion, 2)
	if status != http.StatusOK || body["replayed"] != false {
		t.Fatalf("use potion: %d %v", status, body)
	}
	txID := body["transaction_id"]
	if got := progressOf(body); got != (player.Progress{Level: 2, Experience: 0, Coins: 0}) {
		t.Fatalf("progress after potion = %+v", got)
	}
	if inv := inventory(t, app, tok); inv[itemPotion] != 1 {
		t.Fatalf("potion quantity = %d, want 1", inv[itemPotion])
	}

	// Same key + same payload: replayed, nothing consumed again.
	status, body = useItem(t, app, tok, "k1", itemPotion, 2)
	if status != http.StatusOK || body["replayed"] != true || body["transaction_id"] != txID {
		t.Fatalf("replay: %d %v", status, body)
	}
	if inv := inventory(t, app, tok); inv[itemPotion] != 1 {
		t.Fatalf("replay consumed items: potion = %d", inv[itemPotion])
	}
	if n := countRows(t, `SELECT COUNT(*) FROM item_transactions WHERE transaction_type = 'use'`); n != 1 {
		t.Fatalf("use ledger rows = %d, want 1", n)
	}

	// Same key + different payload is a client bug.
	if status, body = useItem(t, app, tok, "k1", itemPotion, 1); status != http.StatusUnprocessableEntity || errCode(body) != "IDEMPOTENCY_KEY_MISMATCH" {
		t.Fatalf("key mismatch: %d %v", status, body)
	}

	// Not enough owned: rejected, and the ledger row is rolled back with it.
	if status, body = useItem(t, app, tok, "k2", itemPotion, 2); status != http.StatusConflict || errCode(body) != "INSUFFICIENT_ITEM" {
		t.Fatalf("insufficient: %d %v", status, body)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM item_transactions WHERE idempotency_key = 'k2'`); n != 0 {
		t.Fatal("failed use must not leave a ledger row")
	}
	if status, body = useItem(t, app, tok, "k3", itemElixir, 1); status != http.StatusConflict {
		t.Fatalf("unowned item: %d %v", status, body)
	}

	// Unknown and soft-deleted items look the same.
	for _, id := range []int64{itemRetired, 999} {
		if status, body = useItem(t, app, tok, "k4", id, 1); status != http.StatusNotFound || errCode(body) != "ITEM_NOT_FOUND" {
			t.Fatalf("item %d: %d %v", id, status, body)
		}
	}

	// Coin item adds coins; a stock that hits zero disappears from the inventory.
	status, body = useItem(t, app, tok, "k5", itemCoinPouch, 3)
	if status != http.StatusOK || progressOf(body).Coins != 300 {
		t.Fatalf("use coin pouch: %d %v", status, body)
	}
	if inv := inventory(t, app, tok); len(inv) != 1 || inv[itemPotion] != 1 {
		t.Fatalf("inventory after spending pouches = %v", inv)
	}
	if got := playerProgress(t, app, tok); got != (player.Progress{Level: 2, Experience: 0, Coins: 300}) {
		t.Fatalf("persisted progress = %+v", got)
	}

	// Accounts without a player (admins) cannot use items.
	if err := svcs.Auth.EnsureAdmin(context.Background(), "admin@example.com", "admin-password"); err != nil {
		t.Fatal(err)
	}
	_, adminBody := call(t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]any{"email": "admin@example.com", "password": "admin-password"})
	adminTok := adminBody["access_token"].(string)
	if status, body = useItem(t, app, adminTok, "k6", itemPotion, 1); status != http.StatusNotFound || errCode(body) != "PLAYER_NOT_FOUND" {
		t.Fatalf("admin use: %d %v", status, body)
	}
}

func TestDailyReward(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")
	today := reward.RewardDate(time.Now())

	status, body := call(t, app, http.MethodGet, "/api/v1/rewards/status", tok, nil)
	if status != http.StatusOK || body["claimed"] != false || body["reward_date"] != today {
		t.Fatalf("status before: %d %v", status, body)
	}

	status, body = call(t, app, http.MethodPost, "/api/v1/rewards/claim", tok, nil)
	if status != http.StatusOK || body["reward_date"] != today || body["coins"] != float64(reward.DailyCoins) {
		t.Fatalf("claim: %d %v", status, body)
	}
	if got := progressOf(body); got != (player.Progress{Level: 1, Experience: reward.DailyExperience, Coins: reward.DailyCoins}) {
		t.Fatalf("progress after claim = %+v", got)
	}

	status, body = call(t, app, http.MethodGet, "/api/v1/rewards/status", tok, nil)
	if status != http.StatusOK || body["claimed"] != true {
		t.Fatalf("status after: %d %v", status, body)
	}
	status, body = call(t, app, http.MethodPost, "/api/v1/rewards/claim", tok, nil)
	if status != http.StatusConflict || errCode(body) != "REWARD_ALREADY_CLAIMED" {
		t.Fatalf("second claim: %d %v", status, body)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM reward_claims`); n != 1 {
		t.Fatalf("reward_claims rows = %d, want 1", n)
	}
	if got := playerProgress(t, app, tok); got.Coins != reward.DailyCoins {
		t.Fatalf("coins = %d after a rejected second claim, want %d", got.Coins, reward.DailyCoins)
	}
}

// The reward day is the JST calendar day: 14:59Z and 15:00Z on the same UTC
// date are different reward days, so both claims succeed.
func TestDailyRewardJSTBoundary(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")
	_, me := call(t, app, http.MethodGet, "/api/v1/auth/me", tok, nil)
	userID := int64(me["id"].(float64))

	svcAt := func(now time.Time) *reward.Service {
		return reward.NewService(testDB, reward.NewRepository(), player.NewRepository(),
			slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now })
	}
	ctx := context.Background()

	before := svcAt(time.Date(2026, 11, 13, 14, 59, 59, 0, time.UTC))
	res, err := before.Claim(ctx, userID)
	if err != nil || res.RewardDate != "2026-11-13" {
		t.Fatalf("claim at 14:59Z: %+v %v", res, err)
	}
	if _, err := before.Claim(ctx, userID); !errors.Is(err, reward.ErrAlreadyClaimed) {
		t.Fatalf("second claim same JST day: err = %v", err)
	}

	after := svcAt(time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC))
	res, err = after.Claim(ctx, userID)
	if err != nil || res.RewardDate != "2026-11-14" {
		t.Fatalf("claim at 15:00Z: %+v %v", res, err)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM reward_claims`); n != 2 {
		t.Fatalf("reward_claims rows = %d, want 2", n)
	}
}

func TestRanking(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, svcs := newApp(t)
	tokA := registerAndLogin(t, app, "a@example.com", "alpha")
	tokB := registerAndLogin(t, app, "b@example.com", "bravo")
	_ = registerAndLogin(t, app, "c@example.com", "charlie")

	// alpha and bravo tie; alpha registered first so it ranks higher.
	ctx := context.Background()
	for _, upd := range []struct {
		name  string
		level int
		exp   int64
	}{{"alpha", 5, 10}, {"bravo", 5, 10}, {"charlie", 7, 0}} {
		if _, err := testDB.ExecContext(ctx, `UPDATE players SET level = ?, experience = ? WHERE player_name = ?`, upd.level, upd.exp, upd.name); err != nil {
			t.Fatal(err)
		}
	}

	status, body := call(t, app, http.MethodGet, "/api/v1/ranking", tokA, nil)
	if status != http.StatusOK || body["total"] != float64(3) {
		t.Fatalf("ranking: %d %v", status, body)
	}
	names := func(body map[string]any) []string {
		var out []string
		for _, raw := range body["rankings"].([]any) {
			out = append(out, raw.(map[string]any)["player_name"].(string))
		}
		return out
	}
	if got := names(body); len(got) != 3 || got[0] != "charlie" || got[1] != "alpha" || got[2] != "bravo" {
		t.Fatalf("order = %v, want [charlie alpha bravo]", got)
	}

	status, body = call(t, app, http.MethodGet, "/api/v1/ranking?limit=2&offset=1", tokA, nil)
	if status != http.StatusOK || body["limit"] != float64(2) || body["offset"] != float64(1) || body["total"] != float64(3) {
		t.Fatalf("page: %d %v", status, body)
	}
	rows := body["rankings"].([]any)
	if got := names(body); len(got) != 2 || got[0] != "alpha" || got[1] != "bravo" {
		t.Fatalf("page order = %v, want [alpha bravo]", got)
	}
	if rows[0].(map[string]any)["rank"] != float64(2) || rows[1].(map[string]any)["rank"] != float64(3) {
		t.Fatalf("ranks on page 2 = %v", rows)
	}

	for _, q := range []string{"limit=0", "limit=101", "offset=-1", "limit=abc"} {
		if status, body = call(t, app, http.MethodGet, "/api/v1/ranking?"+q, tokA, nil); status != http.StatusBadRequest || errCode(body) != "VALIDATION_ERROR" {
			t.Fatalf("%s: %d %v", q, status, body)
		}
	}

	status, body = call(t, app, http.MethodGet, "/api/v1/ranking/me", tokB, nil)
	if status != http.StatusOK || body["rank"] != float64(3) || body["total"] != float64(3) || body["player_name"] != "bravo" {
		t.Fatalf("ranking/me: %d %v", status, body)
	}

	if err := svcs.Auth.EnsureAdmin(ctx, "admin@example.com", "admin-password"); err != nil {
		t.Fatal(err)
	}
	_, adminBody := call(t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]any{"email": "admin@example.com", "password": "admin-password"})
	adminTok := adminBody["access_token"].(string)
	if status, body = call(t, app, http.MethodGet, "/api/v1/ranking/me", adminTok, nil); status != http.StatusNotFound || errCode(body) != "PLAYER_NOT_FOUND" {
		t.Fatalf("admin ranking/me: %d %v", status, body)
	}
	if status, _ = call(t, app, http.MethodGet, "/api/v1/ranking", adminTok, nil); status != http.StatusOK {
		t.Fatalf("admin can read the leaderboard: %d", status)
	}
}
