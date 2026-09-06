package models

import (
	"encoding/json"

	"github.com/uptrace/bun"
)

// PreRaceRunner stores one runner per race. Runner retains the UI payload while
// race_id and horse_id remain relational, constrained keys.
type PreRaceRunner struct {
	bun.BaseModel `bun:"table:pre_race_runners,alias:prr"`

	ID      int             `bun:"id,pk,autoincrement" json:"id"`
	RaceID  int             `bun:"race_id,notnull,unique:pre_race_runner_no_dupes" json:"raceID"`
	HorseID int             `bun:"horse_id,notnull,unique:pre_race_runner_no_dupes" json:"horseID"`
	Race    *Race           `bun:"rel:belongs-to,join:race_id=race_id" json:"-"`
	Horse   *Horse          `bun:"rel:belongs-to,join:horse_id=horse_id" json:"-"`
	Runner  json.RawMessage `bun:"runner,notnull,type:jsonb" json:"runner"`
}
