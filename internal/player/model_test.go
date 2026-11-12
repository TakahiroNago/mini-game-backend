package player

import "testing"

func TestApplyExperience(t *testing.T) {
	tests := []struct {
		name           string
		level          int
		exp, gain      int64
		wantLevel      int
		wantExperience int64
	}{
		{"no level up", 1, 0, 50, 1, 50},
		{"exact threshold levels up", 1, 50, 50, 2, 0},
		{"carry over remainder", 1, 90, 30, 2, 20},
		{"multiple level ups in one gain", 1, 0, 350, 3, 50}, // 100 -> L2 (250 left), 200 -> L3 (50 left)
		{"higher level threshold", 5, 499, 1, 6, 0},          // needs 500 at L5
		{"negative gain ignored", 3, 10, -100, 3, 10},
		{"max level keeps accumulating", MaxLevel, 10, 1_000_000, MaxLevel, 1_000_010},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLevel, gotExp := ApplyExperience(tt.level, tt.exp, tt.gain)
			if gotLevel != tt.wantLevel || gotExp != tt.wantExperience {
				t.Fatalf("ApplyExperience(%d, %d, %d) = (%d, %d), want (%d, %d)",
					tt.level, tt.exp, tt.gain, gotLevel, gotExp, tt.wantLevel, tt.wantExperience)
			}
		})
	}
}
