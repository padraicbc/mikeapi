package models

import (
	"github.com/uptrace/bun"
	"time"
)

// MeetingDiscovery records successful coverage, including days with no qualifying races.
type MeetingDiscovery struct {
	bun.BaseModel `bun:"table:meeting_discoveries,alias:md"`
	Date          string    `bun:"date,pk,type:date"`
	Country       string    `bun:"country,pk"`
	State         string    `bun:"state,notnull"`
	RaceCount     int       `bun:"race_count,notnull,default:0"`
	LastError     *string   `bun:"last_error"`
	UpdatedAt     time.Time `bun:"updated_at,notnull"`
}
