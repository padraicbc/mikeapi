package models

import "github.com/uptrace/bun"

// Intermediary stores pre-race MR+OR and TFR data before results are available.
type Intermediary struct {
	bun.BaseModel `bun:"table:intermediary,alias:i"`

	ID       int     `bun:"id,pk,autoincrement" json:"id"`
	HorseID  int     `bun:"horse_id,notnull" json:"horseID"`
	RaceID   int     `bun:"race_id,notnull" json:"raceID"`
	MrPlusOr *int    `bun:"mr_plus_or" json:"mrPlusOr,omitempty"`
	Tfr      *string `bun:"tfr" json:"tfr,omitempty"`
}
