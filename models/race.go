package models

import "github.com/uptrace/bun"

// Race represents a horse race event.
type Race struct {
	bun.BaseModel `bun:"table:races,alias:rc"`

	RaceID      int      `bun:"race_id,pk,autoincrement" json:"raceID"`
	CourseID    int      `bun:"course_id,notnull" json:"courseID"`
	Date        string   `bun:"date,notnull,type:date" json:"date"`
	Time        string   `bun:"time,notnull" json:"time"`
	URL         string   `bun:"url,notnull" json:"url"`
	Class       *string  `bun:"class" json:"class,omitempty"`
	Distance    float64  `bun:"distance,notnull" json:"distance"`
	Going       string   `bun:"going,notnull" json:"going"`
	Mr          *int     `bun:"mr" json:"mr,omitempty"`
	Mr2         *int     `bun:"mr2" json:"mr2,omitempty"`
	Analysed    bool     `bun:"analysed,notnull,default:false" json:"analysed"`
	PreDone     bool     `bun:"pre_done,notnull,default:false" json:"preDone"`
	MainComment *string  `bun:"main_comment" json:"mainComment,omitempty"`
	Amended     bool     `bun:"amended,notnull,default:false" json:"amended"`

	Course *Course `bun:"rel:belongs-to,join:course_id=course_id" json:"-"`
}
