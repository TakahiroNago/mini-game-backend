// Package reward implements the daily login reward. Exactly-once per player
// per JST calendar day is enforced by the UNIQUE (player_id, reward_date)
// key, never by an application-side "already claimed?" check.
package reward

import "time"

// Fixed daily reward. Making it configurable is a one-line change once the
// admin UI needs it.
const (
	DailyCoins      = 100
	DailyExperience = 50
)

// JST is the game's calendar zone. A fixed offset is used on purpose: Japan
// has no daylight saving time, and a fixed zone needs no tzdata inside the
// (scratch) container image.
var JST = time.FixedZone("Asia/Tokyo", 9*60*60)

// DateLayout is how reward dates are exchanged with MySQL (DATE column) and
// clients.
const DateLayout = "2006-01-02"

// RewardDate returns the JST calendar day that now falls on, formatted for
// the reward_claims.reward_date DATE column. Storage stays UTC everywhere
// else; only this business day is defined in Japanese time.
func RewardDate(now time.Time) string {
	return now.In(JST).Format(DateLayout)
}
