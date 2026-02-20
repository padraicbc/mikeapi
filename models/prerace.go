package models

import (
	"encoding/json"

	"github.com/uptrace/bun"
)

// PreRace stores pre-race card data with runner JSON.
type PreRace struct {
	bun.BaseModel `bun:"table:pre_race,alias:pr"`

	ID        int             `bun:"id,pk,autoincrement" json:"id"`
	Runners   json.RawMessage `bun:"runners,notnull,type:jsonb" json:"runners"`
	Course    string          `bun:"course,notnull" json:"course"`
	CourseID  int             `bun:"course_id,notnull" json:"courseID"`
	Date      string          `bun:"date,notnull,type:date" json:"date"`
	Time      string          `bun:"time,notnull" json:"time"`
	RaceID    int             `bun:"race_id,notnull,unique:pre_race_no_dupes" json:"raceID"`
	Direction string          `bun:"direction,notnull" json:"direction"`
	Distance  float64         `bun:"distance,notnull" json:"distance"`
	Class     string          `bun:"class,notnull" json:"class"`
	URL       string          `bun:"url,notnull" json:"url"`
}
