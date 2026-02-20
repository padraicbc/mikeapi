package models

import "github.com/uptrace/bun"

// Horse represents a racehorse with historical weight/rating data.
type Horse struct {
	bun.BaseModel `bun:"table:horses,alias:h"`

	HorseID          int     `bun:"horse_id,pk,autoincrement" json:"horseID"`
	Horse            string  `bun:"horse,notnull,unique" json:"horse"`
	LastWinID        *int    `bun:"last_win_id" json:"lastWinID,omitempty"`
	HighestWinWeight *int    `bun:"highest_win_weight,default:0" json:"highestWinWeight,omitempty"`
	LastWinWeight    *int    `bun:"last_win_weight,default:0" json:"lastWinWeight,omitempty"`
	LastRunWeight    *int    `bun:"last_run_weight,default:0" json:"lastRunWeight,omitempty"`
	LastWinClaim     *int    `bun:"last_win_claim,default:0" json:"lastWinClaim,omitempty"`
	LastRunClaim     *int    `bun:"last_run_claim,default:0" json:"lastRunClaim,omitempty"`
	HighestWinOr     *int    `bun:"highest_win_or,default:0" json:"highestWinOr,omitempty"`
}
