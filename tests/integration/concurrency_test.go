//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/takahironago/mini-game-backend/internal/player"
	"github.com/takahironago/mini-game-backend/internal/reward"
)

// fanOut runs n requests concurrently and returns the status histogram plus
// every decoded body (in completion order).
func fanOut(t *testing.T, n int, req func(i int) (int, map[string]any)) (map[int]int, []map[string]any) {
	t.Helper()
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		counts = map[int]int{}
		bodies = make([]map[string]any, 0, n)
		start  = make(chan struct{})
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // release everyone at once to maximise contention
			status, body := req(i)
			mu.Lock()
			counts[status]++
			bodies = append(bodies, body)
			mu.Unlock()
		}(i)
	}
	close(start)
	wg.Wait()
	return counts, bodies
}

// 100 simultaneous claims of the same daily reward: exactly one succeeds,
// the rest get 409, and the player is credited exactly once.
func TestConcurrentRewardClaims(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")

	const n = 100
	counts, _ := fanOut(t, n, func(int) (int, map[string]any) {
		return call(t, app, http.MethodPost, "/api/v1/rewards/claim", tok, nil)
	})
	if counts[http.StatusOK] != 1 || counts[http.StatusConflict] != n-1 || len(counts) != 2 {
		t.Fatalf("status histogram = %v, want {200:1 409:%d}", counts, n-1)
	}
	if rows := countRows(t, `SELECT COUNT(*) FROM reward_claims`); rows != 1 {
		t.Fatalf("reward_claims rows = %d, want 1", rows)
	}
	if got := playerProgress(t, app, tok); got != (player.Progress{Level: 1, Experience: reward.DailyExperience, Coins: reward.DailyCoins}) {
		t.Fatalf("progress = %+v, want exactly one reward applied", got)
	}
}

// 50 simultaneous uses (distinct keys) of an item owned 10 times: exactly 10
// succeed, inventory ends at zero (never negative) and the experience
// credited equals exactly 10 uses.
func TestConcurrentItemUse(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")

	const owned, attempts = 10, 50
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE player_items SET quantity = ? WHERE item_id = ?`, owned, itemPotion); err != nil {
		t.Fatal(err)
	}

	counts, _ := fanOut(t, attempts, func(i int) (int, map[string]any) {
		return useItem(t, app, tok, fmt.Sprintf("use-%03d", i), itemPotion, 1)
	})
	if counts[http.StatusOK] != owned || counts[http.StatusConflict] != attempts-owned || len(counts) != 2 {
		t.Fatalf("status histogram = %v, want {200:%d 409:%d}", counts, owned, attempts-owned)
	}

	var quantity int
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT quantity FROM player_items WHERE item_id = ?`, itemPotion).Scan(&quantity); err != nil {
		t.Fatal(err)
	}
	if quantity != 0 {
		t.Fatalf("remaining quantity = %d, want 0", quantity)
	}
	if rows := countRows(t, `SELECT COUNT(*) FROM item_transactions WHERE transaction_type = 'use'`); rows != owned {
		t.Fatalf("use ledger rows = %d, want %d (failed attempts must roll back)", rows, owned)
	}

	wantLevel, wantExp := player.ApplyExperience(1, 0, int64(owned*50))
	if got := playerProgress(t, app, tok); got.Level != wantLevel || got.Experience != wantExp {
		t.Fatalf("progress = %+v, want level %d exp %d", got, wantLevel, wantExp)
	}
}

// 20 simultaneous retries of the same request (same Idempotency-Key): every
// caller gets 200 with the same transaction id, but the item is consumed
// and the effect applied exactly once.
func TestConcurrentIdempotentReplay(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")

	const n = 20
	counts, bodies := fanOut(t, n, func(int) (int, map[string]any) {
		return useItem(t, app, tok, "same-key", itemPotion, 1)
	})
	if counts[http.StatusOK] != n {
		t.Fatalf("status histogram = %v, want all 200", counts)
	}
	fresh := 0
	txIDs := map[any]struct{}{}
	for _, b := range bodies {
		if b["replayed"] == false {
			fresh++
		}
		txIDs[b["transaction_id"]] = struct{}{}
	}
	if fresh != 1 || len(txIDs) != 1 {
		t.Fatalf("fresh responses = %d (want 1), distinct transaction ids = %d (want 1)", fresh, len(txIDs))
	}
	if inv := inventory(t, app, tok); inv[itemPotion] != 2 {
		t.Fatalf("potion quantity = %d, want 2 (3 starter - 1 use)", inv[itemPotion])
	}
	if rows := countRows(t, `SELECT COUNT(*) FROM item_transactions WHERE idempotency_key = 'same-key'`); rows != 1 {
		t.Fatalf("ledger rows for the key = %d, want 1", rows)
	}
	if got := playerProgress(t, app, tok); got != (player.Progress{Level: 1, Experience: 50, Coins: 0}) {
		t.Fatalf("progress = %+v, want exactly one potion applied", got)
	}
}
