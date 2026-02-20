package models

import "github.com/uptrace/bun"

// Result holds race result data for a single runner.
type Result struct {
	bun.BaseModel `bun:"table:results,alias:r"`

	ID               int      `bun:"id,pk,autoincrement" json:"id"`
	HorseID          int      `bun:"horse_id,notnull" json:"horseID"`
	CourseID         int      `bun:"course_id,notnull" json:"courseID"`
	RaceID           int      `bun:"race_id,notnull" json:"raceID"`
	Age              int      `bun:"age,notnull" json:"age"`
	Price            string   `bun:"price,notnull" json:"price"`
	Trainer          string   `bun:"trainer,notnull" json:"trainer"`
	Jockey           string   `bun:"jockey,notnull" json:"jockey"`
	Number           int      `bun:"number,notnull" json:"number"`
	Headgear         *string  `bun:"headgear" json:"headgear,omitempty"`
	Placed           string   `bun:"placed,notnull" json:"placed"`
	Pace             *string  `bun:"pace" json:"pace,omitempty"`
	OfficialRat      *int     `bun:"official_rat" json:"officialRat,omitempty"`
	WinDist          *float64 `bun:"win_dist" json:"winDist,omitempty"`
	DistBehindWinner *float64 `bun:"dist_behind_winner" json:"distBehindWinner,omitempty"`
	WeightCarried    int      `bun:"weight_carried,notnull" json:"weightCarried"`
	CardWeight       int      `bun:"card_weight,notnull" json:"cardWeight"`
	Claim            *int     `bun:"claim" json:"claim,omitempty"`
	RPR              *int     `bun:"rpr" json:"rpr,omitempty"`
	TS               *int     `bun:"ts" json:"ts,omitempty"`
	MrPlusOr         *int     `bun:"mr_plus_or" json:"mrPlusOr,omitempty"`
	Mr2PlusOr        *int     `bun:"mr2_plus_or" json:"mr2PlusOr,omitempty"`
	WCmr2PlusOr      *int     `bun:"wc_mr2_plus_or" json:"wCmr2PlusOr,omitempty"`
	WCmr1PlusOr      *int     `bun:"wc_mr1_plus_or" json:"wCmr1PlusOr,omitempty"`
	TotRPR           *int     `bun:"tot_rpr" json:"totRPR,omitempty"`
	Tfr              *string  `bun:"tfr" json:"tfr,omitempty"`
	Tfsf             *int     `bun:"tfsf" json:"tfsf,omitempty"`
	TfsfMinusOr      *int     `bun:"tfsf_minus_or" json:"tfsfMinusOr,omitempty"`
	SecT             *float64 `bun:"sec_t" json:"secT,omitempty"`
	SpeedPer         *float64 `bun:"speed_per" json:"speedPer,omitempty"`
	Comment          *string  `bun:"comment" json:"comment,omitempty"`
	Analysed         bool     `bun:"analysed,notnull,default:false" json:"analysed"`
}
