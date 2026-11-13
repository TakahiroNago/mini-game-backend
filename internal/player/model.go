// Package player owns player progression (level / experience / coins) and
// the inventory read model.
package player

import "time"

// Player is the players table row.
type Player struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	PlayerName string    `json:"player_name"`
	Level      int       `json:"level"`
	Experience int64     `json:"experience"`
	Coins      int64     `json:"coins"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Progress is the mutable progression slice of a player. Endpoints that
// change it (item use, reward claim) return the post-update values.
type Progress struct {
	Level      int   `json:"level"`
	Experience int64 `json:"experience"`
	Coins      int64 `json:"coins"`
}

// Progress extracts the progression fields.
func (p Player) Progress() Progress {
	return Progress{Level: p.Level, Experience: p.Experience, Coins: p.Coins}
}

// InventoryItem is one owned item joined with its master data.
type InventoryItem struct {
	ItemID      int64  `json:"item_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ItemType    string `json:"item_type"`
	EffectValue int    `json:"effect_value"`
	Quantity    int    `json:"quantity"`
}

// MaxLevel caps progression so the level-up loop is bounded.
const MaxLevel = 999

// RequiredExp is the experience needed to go from level to level+1.
func RequiredExp(level int) int64 { return int64(level) * 100 }

// ApplyExperience adds gained experience and resolves level-ups. Experience
// is "progress within the current level", so each level-up subtracts the
// threshold. At MaxLevel experience keeps accumulating without a cap so the
// ranking tie-break still works.
func ApplyExperience(level int, experience, gain int64) (newLevel int, newExperience int64) {
	if gain < 0 {
		gain = 0
	}
	newLevel, newExperience = level, experience+gain
	for newLevel < MaxLevel && newExperience >= RequiredExp(newLevel) {
		newExperience -= RequiredExp(newLevel)
		newLevel++
	}
	return newLevel, newExperience
}
