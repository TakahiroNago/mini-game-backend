// Package item owns the item master data, the acquisition / use ledger and
// the "use an item" transaction, including its idempotency guarantee.
package item

import (
	"time"

	"github.com/takahironago/mini-game-backend/internal/player"
)

// Item types decide which progression field an item affects when used.
const (
	TypeCoin       = "coin"
	TypeExperience = "experience"
)

// Transaction types recorded in item_transactions.
const (
	TxAcquire = "acquire"
	TxUse     = "use"
)

// Item is the items table row (master data).
type Item struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	ItemType        string    `json:"item_type"`
	EffectValue     int       `json:"effect_value"`
	StarterQuantity int       `json:"starter_quantity"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Transaction is one item_transactions row.
type Transaction struct {
	ID             int64
	PlayerID       int64
	ItemID         int64
	Quantity       int
	Type           string
	IdempotencyKey *string
	CreatedAt      time.Time
}

// ApplyEffect returns the progression after using quantity units of it.
// Coins add linearly; experience goes through the level-up rule.
func ApplyEffect(p player.Progress, it Item, quantity int) player.Progress {
	if quantity <= 0 {
		return p
	}
	total := int64(it.EffectValue) * int64(quantity)
	switch it.ItemType {
	case TypeCoin:
		p.Coins += total
	case TypeExperience:
		p.Level, p.Experience = player.ApplyExperience(p.Level, p.Experience, total)
	}
	return p
}
