package item

import (
	"testing"

	"github.com/takahironago/mini-game-backend/internal/player"
)

func TestApplyEffect(t *testing.T) {
	coin := Item{ItemType: TypeCoin, EffectValue: 100}
	potion := Item{ItemType: TypeExperience, EffectValue: 50}
	start := player.Progress{Level: 1, Experience: 0, Coins: 10}

	tests := []struct {
		name     string
		item     Item
		quantity int
		want     player.Progress
	}{
		{"coins add linearly", coin, 3, player.Progress{Level: 1, Experience: 0, Coins: 310}},
		{"experience levels up", potion, 2, player.Progress{Level: 2, Experience: 0, Coins: 10}}, // 100 exp = exactly L1 threshold
		{"experience carries remainder", potion, 3, player.Progress{Level: 2, Experience: 50, Coins: 10}},
		{"zero quantity is a no-op", potion, 0, start},
		{"negative quantity is a no-op", coin, -1, start},
		{"unknown type changes nothing", Item{ItemType: "mystery", EffectValue: 999}, 1, start},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ApplyEffect(start, tt.item, tt.quantity); got != tt.want {
				t.Fatalf("ApplyEffect() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
