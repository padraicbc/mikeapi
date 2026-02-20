package models

import "github.com/uptrace/bun"

// Trainer holds trainer name and notes.
type Trainer struct {
	bun.BaseModel `bun:"table:trainers,alias:t"`

	TrainerID int     `bun:"trainer_id,pk,autoincrement" json:"trainerID"`
	Trainer   string  `bun:"trainer,notnull,unique" json:"trainer"`
	Info      *string `bun:"info" json:"info,omitempty"`
}
