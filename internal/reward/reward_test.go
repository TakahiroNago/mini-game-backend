package reward

import (
	"testing"
	"time"
)

func TestRewardDate(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{"UTC midnight is the same JST day", time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC), "2026-11-13"},
		{"one second before JST midnight", time.Date(2026, 11, 13, 14, 59, 59, 0, time.UTC), "2026-11-13"},
		{"JST midnight rolls the day", time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC), "2026-11-14"},
		{"already in JST", time.Date(2026, 1, 1, 0, 0, 0, 0, JST), "2026-01-01"},
		{"year boundary", time.Date(2025, 12, 31, 20, 0, 0, 0, time.UTC), "2026-01-01"},
		{"other zone is converted", time.Date(2026, 6, 1, 23, 30, 0, 0, time.FixedZone("PDT", -7*3600)), "2026-06-02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RewardDate(tt.now); got != tt.want {
				t.Fatalf("RewardDate(%v) = %q, want %q", tt.now, got, tt.want)
			}
		})
	}
}
