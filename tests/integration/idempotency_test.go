//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/takahironago/mini-game-backend/internal/item"
)

// TestIdempotencyRetry models the client-side retry loop: a client that
// never saw the first response resends the identical request with the same
// key, possibly several times, and must end up with exactly one effect.
func TestIdempotencyRetry(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")

	// First attempt succeeds: 1 of 3 starter potions consumed.
	status, first := useItem(t, app, tok, "retry-1", itemPotion, 1)
	if status != http.StatusOK || first["replayed"] != false {
		t.Fatalf("first: %d %v", status, first)
	}

	// Five retries in a row: same transaction, no extra consumption.
	for i := 0; i < 5; i++ {
		status, body := useItem(t, app, tok, "retry-1", itemPotion, 1)
		if status != http.StatusOK || body["replayed"] != true {
			t.Fatalf("retry %d: %d %v", i+1, status, body)
		}
		if body["transaction_id"] != first["transaction_id"] || body["item_id"] != first["item_id"] || body["quantity"] != first["quantity"] {
			t.Fatalf("retry %d returned a different transaction: %v vs %v", i+1, body, first)
		}
	}
	if inv := inventory(t, app, tok); inv[itemPotion] != 2 {
		t.Fatalf("potion = %d, want 2", inv[itemPotion])
	}
	if n := countRows(t, `SELECT COUNT(*) FROM item_transactions WHERE transaction_type = 'use'`); n != 1 {
		t.Fatalf("use ledger rows = %d, want 1", n)
	}

	// A replay returns the original receipt (transaction id, item, quantity)
	// but the player's *current* progress, so a late retry still shows the
	// client an up-to-date state instead of a stale snapshot.
	status, second := useItem(t, app, tok, "retry-2", itemPotion, 1)
	if status != http.StatusOK {
		t.Fatalf("second use: %d %v", status, second)
	}
	status, replay := useItem(t, app, tok, "retry-1", itemPotion, 1)
	if status != http.StatusOK || replay["replayed"] != true || replay["transaction_id"] != first["transaction_id"] {
		t.Fatalf("replay after further use: %d %v", status, replay)
	}
	if progressOf(replay) != progressOf(second) {
		t.Fatalf("replay progress %+v, want current %+v", progressOf(replay), progressOf(second))
	}
}

func TestIdempotencyFailedAttemptDoesNotBurnKey(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")

	// The player owns no elixir: the attempt fails and its ledger row rolls back.
	if status, body := useItem(t, app, tok, "k-fail", itemElixir, 1); status != http.StatusConflict || errCode(body) != "INSUFFICIENT_ITEM" {
		t.Fatalf("elixir: %d %v", status, body)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM item_transactions WHERE idempotency_key = 'k-fail'`); n != 0 {
		t.Fatalf("ledger row survived a failed use: %d", n)
	}
	// Unknown / deleted items fail the same way, before any write.
	if status, body := useItem(t, app, tok, "k-fail", itemRetired, 1); status != http.StatusNotFound || errCode(body) != "ITEM_NOT_FOUND" {
		t.Fatalf("retired item: %d %v", status, body)
	}
	// The same key can therefore be reused for a request that does succeed.
	if status, body := useItem(t, app, tok, "k-fail", itemPotion, 1); status != http.StatusOK || body["replayed"] != false {
		t.Fatalf("reuse after failure: %d %v", status, body)
	}
}

func TestIdempotencyKeyScopeAndFormat(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tokA := registerAndLogin(t, app, "a@example.com", "alice")
	tokB := registerAndLogin(t, app, "b@example.com", "bob")

	// Keys are scoped per player: the same string never collides across accounts.
	statusA, bodyA := useItem(t, app, tokA, "shared-key", itemPotion, 1)
	statusB, bodyB := useItem(t, app, tokB, "shared-key", itemPotion, 1)
	if statusA != http.StatusOK || statusB != http.StatusOK || bodyA["replayed"] != false || bodyB["replayed"] != false {
		t.Fatalf("shared key: A %d %v / B %d %v", statusA, bodyA, statusB, bodyB)
	}
	if bodyA["transaction_id"] == bodyB["transaction_id"] {
		t.Fatal("two players got the same transaction id")
	}
	// Player B cannot replay A's receipt by guessing the key: B's own replay is returned.
	if _, body := useItem(t, app, tokB, "shared-key", itemPotion, 1); body["transaction_id"] != bodyB["transaction_id"] {
		t.Fatalf("B's replay = %v, want B's own transaction", body)
	}

	// Key format: 1..64 chars from [A-Za-z0-9_.:-]. Everything else is a 400.
	for _, bad := range []string{
		strings.Repeat("k", 65), "has space", "semi;colon", "quote'", "slash/", "日本語", "tab\tkey",
	} {
		status, body := callWith(t, app, http.MethodPost, "/api/v1/items/use", tokA,
			map[string]string{item.IdempotencyKeyHeader: bad}, map[string]any{"item_id": itemPotion, "quantity": 1})
		if status != http.StatusBadRequest || errCode(body) != "IDEMPOTENCY_KEY_REQUIRED" {
			t.Fatalf("key %q: %d %v", bad, status, body)
		}
	}
	for _, good := range []string{strings.Repeat("k", 64), "550e8400-e29b-41d4-a716-446655440000", "ns:req.1_2"} {
		if status, body := useItem(t, app, tokA, good, itemCoinPouch, 1); status != http.StatusOK {
			t.Fatalf("key %q: %d %v", good, status, body)
		}
	}
	// Mismatch detection covers every field of the payload.
	if status, body := useItem(t, app, tokA, "ns:req.1_2", itemPotion, 1); status != http.StatusUnprocessableEntity || errCode(body) != "IDEMPOTENCY_KEY_MISMATCH" {
		t.Fatalf("mismatch item: %d %v", status, body)
	}
	if status, body := useItem(t, app, tokA, "ns:req.1_2", itemCoinPouch, 2); status != http.StatusUnprocessableEntity || errCode(body) != "IDEMPOTENCY_KEY_MISMATCH" {
		t.Fatalf("mismatch quantity: %d %v", status, body)
	}
}
