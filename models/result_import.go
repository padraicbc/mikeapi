package models

import (
	"github.com/uptrace/bun"
	"time"
)

// ResultImport tracks discovery and verified completion independently of partial result rows.
type ResultImport struct {
	bun.BaseModel   `bun:"table:result_imports,alias:ri"`
	SourceRaceID    string     `bun:"source_race_id,pk"`
	RaceID          int        `bun:"race_id,notnull"`
	URL             string     `bun:"url,notnull"`
	Title           string     `bun:"title,notnull"`
	Country         string     `bun:"country,notnull"`
	SourceStatus    string     `bun:"source_status,notnull"`
	ExpectedRunners *int       `bun:"expected_runners"`
	State           string     `bun:"state,notnull,default:'pending'"`
	ImportedRunners int        `bun:"imported_runners,notnull,default:0"`
	Attempts        int        `bun:"attempts,notnull,default:0"`
	LastError       *string    `bun:"last_error"`
	StartedAt       *time.Time `bun:"started_at"`
	CompletedAt     *time.Time `bun:"completed_at"`
	Race            *Race      `bun:"rel:belongs-to,join:race_id=race_id"`
}
