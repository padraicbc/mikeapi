package handlers

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

// resultsAnalysisRow is a flat scan target for the results join query.
type resultsAnalysisRow struct {
	// results table (alias r)
	ID               int      `bun:"id"`
	Placed           string   `bun:"placed"`
	OfficialRat      *int     `bun:"official_rat"`
	WeightCarried    int      `bun:"weight_carried"`
	HorseName        string   `bun:"horse"`
	MrPlusOr         *int     `bun:"mr_plus_or"`
	Mr2PlusOr        *int     `bun:"mr2_plus_or"`
	Tfsf             *int     `bun:"tfsf"`
	SecT             *float64 `bun:"sec_t"`
	SpeedPer         *float64 `bun:"speed_per"`
	Comment          *string  `bun:"comment"`
	DistBehindWinner *float64 `bun:"dist_behind_winner"`
	// races table (alias rc)
	Date        string  `bun:"date"`
	Time        string  `bun:"time"`
	Class       *string `bun:"class"`
	Distance    float64 `bun:"distance"`
	Going       string  `bun:"going"`
	URL         string  `bun:"url"`
	RaceID      int     `bun:"race_id"`
	Mr          *int    `bun:"mr"`
	Mr2         *int    `bun:"mr2"`
	MainComment *string `bun:"main_comment"`
	// courses table (alias c)
	Course    string `bun:"course"`
	CourseID  int    `bun:"course_id"`
	Direction string `bun:"direction"`
	IsAW      bool   `bun:"is_aw"`
}

type resultsAnalysisRunner struct {
	ID               int      `json:"id"`
	Placed           string   `json:"placed"`
	OfficialRat      *int     `json:"officialRat"`
	WeightCarried    int      `json:"weightCarried"`
	HorseName        string   `json:"horse"`
	MrPlusOr         *int     `json:"mrPlusOr,omitempty"`
	Mr2PlusOr        *int     `json:"mr2PlusOr,omitempty"`
	Tfsf             *int     `json:"tfsf,omitempty"`
	SecT             *float64 `json:"secT,omitempty"`
	SpeedPer         *float64 `json:"speedPer,omitempty"`
	Comment          *string  `json:"comment,omitempty"`
	DistBehindWinner *float64 `json:"distBehindWinner,omitempty"`
}

type resultsAnalysisRace struct {
	Runners     []resultsAnalysisRunner `json:"runners"`
	Date        string                  `json:"date,omitempty"`
	Time        string                  `json:"time,omitempty"`
	Class       *string                 `json:"class,omitempty"`
	Distance    float64                 `json:"distance,omitempty"`
	Going       string                  `json:"going,omitempty"`
	URL         string                  `json:"url"`
	RaceID      string                  `json:"raceID"`
	Mr          *int                    `json:"mr,omitempty"`
	Mr2         *int                    `json:"mr2,omitempty"`
	MainComment *string                 `json:"mainComment,omitempty"`
	Course      string                  `json:"course,omitempty"`
	CourseID    int                     `json:"courseID,omitempty"`
	Direction   string                  `json:"direction,omitempty"`
	IsAW        bool                    `json:"isAw,omitempty"`
}

const resultsJoinSQL = `
SELECT
	r.id, r.placed, r.official_rat, r.weight_carried, h.horse,
	r.mr_plus_or, r.mr2_plus_or, r.tfsf, r.sec_t, r.speed_per, r.comment, r.dist_behind_winner,
	rc.date::text AS date, rc.time, rc.class, rc.distance, rc.going, rc.url,
	rc.race_id, rc.mr, rc.mr2, rc.main_comment,
	c.course, c.course_id, c.direction, c.is_aw
FROM results r
INNER JOIN courses c  ON r.course_id  = c.course_id
INNER JOIN horses  h  ON r.horse_id   = h.horse_id
INNER JOIN races   rc ON r.race_id    = rc.race_id
`

// Results returns all race results for a given date, grouped by race.
func (h *Handler) Results(c echo.Context) error {
	date := c.QueryParam("date")
	if date == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing date param")
	}

	var rows []resultsAnalysisRow
	q := resultsJoinSQL + `WHERE rc.date = ? AND NOT rc.amended ORDER BY r.race_id, LENGTH(r.placed), r.placed`

	if err := h.db.NewRaw(q, date).Scan(c.Request().Context(), &rows); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, groupResultsByRace(rows))
}

// ResultsAnalysis updates result rows with analysis fields after a race.
func (h *Handler) ResultsAnalysis(c echo.Context) error {
	prm := c.QueryParams()
	mr, mr2, raceID, comm := prm.Get("mr"), prm.Get("mr2"), prm.Get("raceID"), prm.Get("comment")

	if mr == "" || mr2 == "" || raceID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing mr, mr2, or raceID param")
	}

	type rowUpdate struct {
		ID          string `json:"id"`
		Tfsf        string `json:"tfsf"`
		SecT        string `json:"secT"`
		WCmrPlusOr  string `json:"wCmrPlusOr"`
		WCmr2PlusOr string `json:"wCmr2PlusOr"`
		MrPlusOr    string `json:"mrPlusOr"`
		Mr2PlusOr   string `json:"mr2PlusOr"`
		Comment     string `json:"comment"`
		SpeedPer    string `json:"speedPer"`
		TfsfMinusOr string `json:"tfsfMinusOr"`
		DistBehind  string `json:"distBehindWinner"`
		Placed      string `json:"placed,omitempty"`
	}

	var fields []rowUpdate
	if err := c.Bind(&fields); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	ctx := c.Request().Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer tx.Rollback()

	for _, ru := range fields {
		_, err = tx.ExecContext(ctx,
			`UPDATE results SET tfsf = NULLIF(?,'')::integer, sec_t = NULLIF(?,'')::numeric,
			speed_per = NULLIF(?,'')::numeric, comment = NULLIF(?,''),
			mr2_plus_or = NULLIF(?,'')::integer, mr_plus_or = NULLIF(?,'')::integer,
			wc_mr2_plus_or = NULLIF(?,'')::integer, wc_mr1_plus_or = NULLIF(?,'')::integer,
			analysed = true, tfsf_minus_or = NULLIF(?,'')::integer WHERE id = ?`,
			ru.Tfsf, ru.SecT, ru.SpeedPer, ru.Comment,
			ru.Mr2PlusOr, ru.MrPlusOr, ru.WCmr2PlusOr, ru.WCmrPlusOr,
			ru.TfsfMinusOr, ru.ID,
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE races SET mr = NULLIF(?,'')::integer, mr2 = NULLIF(?,'')::integer, main_comment = NULLIF(?,'') WHERE race_id = ?`,
		mr, mr2, comm, raceID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err = tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusAccepted)
}

// groupResultsByRace converts flat rows into race-grouped slices.
func groupResultsByRace(rows []resultsAnalysisRow) []resultsAnalysisRace {
	order := []string{}
	races := map[string]*resultsAnalysisRace{}

	for _, row := range rows {
		key := fmt.Sprintf("%d", row.RaceID)
		runner := resultsAnalysisRunner{
			ID:               row.ID,
			Placed:           row.Placed,
			OfficialRat:      row.OfficialRat,
			WeightCarried:    row.WeightCarried,
			HorseName:        row.HorseName,
			MrPlusOr:         row.MrPlusOr,
			Mr2PlusOr:        row.Mr2PlusOr,
			Tfsf:             row.Tfsf,
			SecT:             row.SecT,
			SpeedPer:         row.SpeedPer,
			Comment:          row.Comment,
			DistBehindWinner: row.DistBehindWinner,
		}

		if _, ok := races[key]; !ok {
			order = append(order, key)
			races[key] = &resultsAnalysisRace{
				Date:        row.Date,
				Time:        row.Time,
				Class:       row.Class,
				Distance:    row.Distance,
				Going:       row.Going,
				URL:         row.URL,
				RaceID:      key,
				Mr:          row.Mr,
				Mr2:         row.Mr2,
				MainComment: row.MainComment,
				Course:      row.Course,
				CourseID:    row.CourseID,
				Direction:   row.Direction,
				IsAW:        row.IsAW,
				Runners:     []resultsAnalysisRunner{},
			}
		}
		races[key].Runners = append(races[key].Runners, runner)
	}

	out := make([]resultsAnalysisRace, 0, len(order))
	for _, k := range order {
		out = append(out, *races[k])
	}
	return out
}
