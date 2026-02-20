package models

import "github.com/uptrace/bun"

// Course represents a racecourse.
type Course struct {
	bun.BaseModel `bun:"table:courses,alias:c"`

	CourseID  int    `bun:"course_id,pk,autoincrement" json:"courseID"`
	Course    string `bun:"course,notnull,unique" json:"course"`
	Direction string `bun:"direction,notnull" json:"direction"`
	IsAW      bool   `bun:"is_aw,notnull" json:"isAw"`
	Code      string `bun:"code,notnull" json:"code"`
}
