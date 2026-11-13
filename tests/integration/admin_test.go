//go:build integration

package integration

import (
	"context"
	"net/http"
	"strconv"
	"testing"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func loginAdmin(t *testing.T, app http.Handler, ensure func(ctx context.Context, email, password string) error) string {
	t.Helper()
	if err := ensure(context.Background(), "admin@example.com", "admin-password"); err != nil {
		t.Fatal(err)
	}
	status, body := call(t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": "admin@example.com", "password": "admin-password",
	})
	if status != http.StatusOK {
		t.Fatalf("admin login: %d %v", status, body)
	}
	return body["access_token"].(string)
}

func TestAdminRoleSeparation(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, svcs := newApp(t)
	playerTok := registerAndLogin(t, app, "p@example.com", "player1")
	adminTok := loginAdmin(t, app, svcs.Auth.EnsureAdmin)

	for _, p := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/dashboard"},
		{http.MethodGet, "/api/v1/admin/users"},
		{http.MethodGet, "/api/v1/admin/items"},
		{http.MethodPost, "/api/v1/admin/items"},
		{http.MethodGet, "/api/v1/admin/audit-logs"},
	} {
		if status, body := call(t, app, p.method, p.path, playerTok, nil); status != http.StatusForbidden || errCode(body) != "FORBIDDEN" {
			t.Fatalf("player %s %s: %d %v", p.method, p.path, status, body)
		}
		if status, _ := call(t, app, p.method, p.path, "", nil); status != http.StatusUnauthorized {
			t.Fatalf("anonymous %s %s: %d", p.method, p.path, status)
		}
	}
	if status, body := call(t, app, http.MethodGet, "/api/v1/admin/dashboard", adminTok, nil); status != http.StatusOK || body["users"] != float64(2) || body["players"] != float64(1) {
		t.Fatalf("admin dashboard: %d %v", status, body)
	}
}

func TestAdminItemsCRUDWithAudit(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, svcs := newApp(t)
	adminTok := loginAdmin(t, app, svcs.Auth.EnsureAdmin)

	// Create.
	status, body := call(t, app, http.MethodPost, "/api/v1/admin/items", adminTok, map[string]any{
		"name": "Golden Apple", "description": "rare", "item_type": "experience", "effect_value": 1000, "starter_quantity": 1,
	})
	if status != http.StatusCreated || body["name"] != "Golden Apple" || body["starter_quantity"] != float64(1) {
		t.Fatalf("create: %d %v", status, body)
	}
	newID := int64(body["id"].(float64))

	// A new player now receives the new starter item as well.
	tok := registerAndLogin(t, app, "late@example.com", "latecomer")
	if inv := inventory(t, app, tok); inv[newID] != 1 || inv[itemPotion] != 3 {
		t.Fatalf("starter inventory after create = %v", inv)
	}

	// Update (and verify the player-facing catalogue reflects it).
	status, body = call(t, app, http.MethodPut, "/api/v1/admin/items/"+itoa(newID), adminTok, map[string]any{
		"name": "Golden Apple+", "description": "rarer", "item_type": "experience", "effect_value": 1500, "starter_quantity": 0,
	})
	if status != http.StatusOK || body["name"] != "Golden Apple+" || body["effect_value"] != float64(1500) {
		t.Fatalf("update: %d %v", status, body)
	}

	// Delete: hidden from players, still visible to admins with include_deleted.
	if status, _ = call(t, app, http.MethodDelete, "/api/v1/admin/items/"+itoa(newID), adminTok, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if status, body = call(t, app, http.MethodDelete, "/api/v1/admin/items/"+itoa(newID), adminTok, nil); status != http.StatusNotFound || errCode(body) != "ITEM_NOT_FOUND" {
		t.Fatalf("delete twice: %d %v", status, body)
	}
	if status, body = call(t, app, http.MethodPut, "/api/v1/admin/items/"+itoa(newID), adminTok, map[string]any{
		"name": "x", "item_type": "coin", "effect_value": 1,
	}); status != http.StatusNotFound {
		t.Fatalf("update deleted: %d %v", status, body)
	}
	_, body = call(t, app, http.MethodGet, "/api/v1/items", tok, nil)
	for _, raw := range body["items"].([]any) {
		if int64(raw.(map[string]any)["id"].(float64)) == newID {
			t.Fatal("deleted item still in the player catalogue")
		}
	}
	_, body = call(t, app, http.MethodGet, "/api/v1/admin/items?include_deleted=true", adminTok, nil)
	found := false
	for _, raw := range body["items"].([]any) {
		it := raw.(map[string]any)
		if int64(it["id"].(float64)) == newID {
			found = true
			if it["deleted_at"] == nil {
				t.Fatal("deleted_at must be set on a deleted item")
			}
		}
	}
	if !found {
		t.Fatal("deleted item missing from include_deleted listing")
	}
	// The latecomer still owns the (now deleted) item: history is preserved.
	if inv := inventory(t, app, tok); inv[newID] != 1 {
		t.Fatalf("inventory lost the deleted item: %v", inv)
	}

	// Audit trail: create, update, delete, newest first, with before/after.
	status, body = call(t, app, http.MethodGet, "/api/v1/admin/audit-logs", adminTok, nil)
	if status != http.StatusOK || body["total"] != float64(3) {
		t.Fatalf("audit: %d %v", status, body)
	}
	logs := body["logs"].([]any)
	actions := []string{}
	for _, raw := range logs {
		l := raw.(map[string]any)
		actions = append(actions, l["action"].(string))
		if l["actor_email"] != "admin@example.com" || int64(l["target_id"].(float64)) != newID || l["target_type"] != "item" {
			t.Fatalf("unexpected audit row: %v", l)
		}
	}
	if len(actions) != 3 || actions[0] != "item.delete" || actions[1] != "item.update" || actions[2] != "item.create" {
		t.Fatalf("audit actions = %v", actions)
	}
	upd := logs[1].(map[string]any)["detail"].(map[string]any)
	if upd["before"].(map[string]any)["effect_value"] != float64(1000) || upd["after"].(map[string]any)["effect_value"] != float64(1500) {
		t.Fatalf("update audit detail = %v", upd)
	}
}

func TestAdminUsersAndGrant(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, svcs := newApp(t)
	adminTok := loginAdmin(t, app, svcs.Auth.EnsureAdmin)
	heroTok := registerAndLogin(t, app, "hero@example.com", "hero")
	_ = registerAndLogin(t, app, "mage@example.com", "mage")
	useItem(t, app, heroTok, "k1", itemPotion, 1)

	// List: newest first, admin has no player, search by email / name.
	status, body := call(t, app, http.MethodGet, "/api/v1/admin/users", adminTok, nil)
	if status != http.StatusOK || body["total"] != float64(3) {
		t.Fatalf("users: %d %v", status, body)
	}
	users := body["users"].([]any)
	if users[0].(map[string]any)["email"] != "mage@example.com" || users[2].(map[string]any)["player"] != nil {
		t.Fatalf("users order / admin player: %v", users)
	}
	status, body = call(t, app, http.MethodGet, "/api/v1/admin/users?q=hero", adminTok, nil)
	if status != http.StatusOK || body["total"] != float64(1) {
		t.Fatalf("users?q=hero: %d %v", status, body)
	}
	hero := body["users"].([]any)[0].(map[string]any)
	heroUserID := int64(hero["id"].(float64))
	heroPlayerID := int64(hero["player"].(map[string]any)["id"].(float64))

	// Detail.
	status, body = call(t, app, http.MethodGet, "/api/v1/admin/users/"+itoa(heroUserID), adminTok, nil)
	if status != http.StatusOK {
		t.Fatalf("user detail: %d %v", status, body)
	}
	if n := len(body["inventory"].([]any)); n != 2 {
		t.Fatalf("inventory rows = %d, want 2", n)
	}
	txs := body["recent_transactions"].([]any)
	if len(txs) != 3 || txs[0].(map[string]any)["transaction_type"] != "use" || txs[0].(map[string]any)["item_name"] != "Experience Potion" {
		t.Fatalf("recent transactions = %v", txs)
	}
	if status, body = call(t, app, http.MethodGet, "/api/v1/admin/users/9999", adminTok, nil); status != http.StatusNotFound || errCode(body) != "USER_NOT_FOUND" {
		t.Fatalf("unknown user: %d %v", status, body)
	}

	// Grant: inventory grows, ledger + audit rows appear, errors map cleanly.
	status, body = call(t, app, http.MethodPost, "/api/v1/admin/players/"+itoa(heroPlayerID)+"/items", adminTok, map[string]any{
		"item_id": itemElixir, "quantity": 2,
	})
	if status != http.StatusOK || body["quantity"] != float64(2) {
		t.Fatalf("grant: %d %v", status, body)
	}
	if inv := inventory(t, app, heroTok); inv[itemElixir] != 2 {
		t.Fatalf("inventory after grant = %v", inv)
	}
	if status, body = call(t, app, http.MethodPost, "/api/v1/admin/players/"+itoa(heroPlayerID)+"/items", adminTok, map[string]any{
		"item_id": itemRetired, "quantity": 1,
	}); status != http.StatusNotFound || errCode(body) != "ITEM_NOT_FOUND" {
		t.Fatalf("grant deleted item: %d %v", status, body)
	}
	if status, body = call(t, app, http.MethodPost, "/api/v1/admin/players/9999/items", adminTok, map[string]any{
		"item_id": itemElixir, "quantity": 1,
	}); status != http.StatusNotFound || errCode(body) != "PLAYER_NOT_FOUND" {
		t.Fatalf("grant unknown player: %d %v", status, body)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'player.grant_item' AND target_id = ?`, heroPlayerID); n != 1 {
		t.Fatalf("grant audit rows = %d, want 1", n)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM item_transactions WHERE player_id = ? AND item_id = ? AND transaction_type = 'acquire'`, heroPlayerID, itemElixir); n != 1 {
		t.Fatalf("grant ledger rows = %d, want 1", n)
	}

	// Dashboard reflects today's activity.
	status, body = call(t, app, http.MethodGet, "/api/v1/admin/dashboard", adminTok, nil)
	if status != http.StatusOK || body["registrations_today"] != float64(3) || body["item_uses_today"] != float64(1) {
		t.Fatalf("dashboard: %d %v", status, body)
	}
	if top := body["top_players"].([]any); len(top) != 2 || top[0].(map[string]any)["player_name"] != "hero" {
		t.Fatalf("top players = %v", top)
	}
}
