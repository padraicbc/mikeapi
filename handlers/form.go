package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

type formRow struct {
	Pace             *string  `bun:"pace"`
	OfficialRat      *int     `bun:"official_rat"`
	Mr2PlusOr        *int     `bun:"mr2_plus_or"`
	MrPlusOr         *int     `bun:"mr_plus_or"`
	Tfsf             *int     `bun:"tfsf"`
	TfsfMinusOr      *int     `bun:"tfsf_minus_or"`
	DistBehindWinner *float64 `bun:"dist_behind_winner"`
	SecT             *float64 `bun:"sec_t"`
	SpeedPer         *float64 `bun:"speed_per"`
	WeightCarried    int      `bun:"weight_carried"`
	WCmr2PlusOr      *int     `bun:"wc_mr2_plus_or"`
	// courses
	Course string `bun:"course"`
	// races
	Date     string  `bun:"date"`
	Time     string  `bun:"time"`
	URL      string  `bun:"url"`
	Placed   string  `bun:"placed"`
	Class    *string `bun:"class"`
	Going    string  `bun:"going"`
	Mr2      *int    `bun:"mr2"`
	Mr       *int    `bun:"mr"`
	Distance float64 `bun:"distance"`
	// horses
	LastWinWeight int `bun:"last_win_weight"`
	LastRunWeight int `bun:"last_run_weight"`
	// concatenated comment
	FullComment string `bun:"full_comment"`
}

type formJSON struct {
	Pace             *string  `json:"pace,omitempty"`
	OfficialRat      *int     `json:"officialRat,omitempty"`
	Mr2PlusOr        *int     `json:"mr2PlusOr,omitempty"`
	MrPlusOr         *int     `json:"mrPlusOr,omitempty"`
	Tfsf             *int     `json:"tfsf,omitempty"`
	TfsfMinusOr      *int     `json:"tfsfMinusOr,omitempty"`
	DistBehindWinner *float64 `json:"distBehindWinner,omitempty"`
	SecT             *float64 `json:"secT,omitempty"`
	SpeedPer         *float64 `json:"speedPer,omitempty"`
	WeightCarried    int      `json:"weightCarried"`
	WCmr2PlusOr      *int     `json:"wCmr2PlusOr"`
	Course           string   `json:"course,omitempty"`
	Date             string   `json:"date,omitempty"`
	Time             string   `json:"time,omitempty"`
	URL              string   `json:"url,omitempty"`
	Placed           string   `json:"placed,omitempty"`
	Class            *string  `json:"class,omitempty"`
	Going            string   `json:"going,omitempty"`
	Mr2              *int     `json:"mr2,omitempty"`
	Mr               *int     `json:"mr,omitempty"`
	Distance         float64  `json:"distance"`
	LastWinWeight    int      `json:"lastWinWeight"`
	LastRunWeight    int      `json:"lastRunWeight"`
	FullComment      string   `json:"fullComment,omitempty"`
}

// GetForm returns the last 8 races for a horse, with optional filters.
func (h *Handler) GetForm(c echo.Context) error {
	q := c.QueryParams()
	horseID := q.Get("horseID")
	if horseID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing horseID param")
	}

	sb := h.db.NewSelect().
		TableExpr("results r").
		ColumnExpr(`
			r.pace, r.official_rat, r.mr2_plus_or, r.mr_plus_or,
			r.tfsf, r.tfsf_minus_or, r.dist_behind_winner, r.sec_t, r.speed_per,
			r.weight_carried, r.wc_mr2_plus_or,
			c.course,
			rc.date::text AS date, rc.time, rc.url, r.placed, rc.class, rc.going,
			rc.mr2, rc.mr, rc.distance,
			h.last_win_weight, h.last_run_weight,
			CONCAT_WS(',', rc.main_comment, r.comment) AS full_comment`).
		Join("INNER JOIN courses c  ON r.course_id = c.course_id").
		Join("INNER JOIN horses  h  ON r.horse_id  = h.horse_id").
		Join("INNER JOIN races   rc ON r.race_id   = rc.race_id").
		Where("r.horse_id = ?", horseID)

	applyFormFilters(sb, q)

	sb = sb.OrderExpr("rc.date DESC").Limit(8)

	var rows []formRow
	if err := sb.Scan(c.Request().Context(), &rows); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	result := make([]formJSON, len(rows))
	for i, row := range rows {
		result[i] = formJSON{
			Pace:             row.Pace,
			OfficialRat:      row.OfficialRat,
			Mr2PlusOr:        row.Mr2PlusOr,
			MrPlusOr:         row.MrPlusOr,
			Tfsf:             row.Tfsf,
			TfsfMinusOr:      row.TfsfMinusOr,
			DistBehindWinner: row.DistBehindWinner,
			SecT:             row.SecT,
			SpeedPer:         row.SpeedPer,
			WeightCarried:    row.WeightCarried,
			WCmr2PlusOr:      row.WCmr2PlusOr,
			Course:           row.Course,
			Date:             row.Date,
			Time:             row.Time,
			URL:              row.URL,
			Placed:           row.Placed,
			Class:            row.Class,
			Going:            row.Going,
			Mr2:              row.Mr2,
			Mr:               row.Mr,
			Distance:         row.Distance,
			LastWinWeight:    row.LastWinWeight,
			LastRunWeight:    row.LastRunWeight,
			FullComment:      row.FullComment,
		}
	}

	return c.JSON(http.StatusOK, result)
}

func applyFormFilters(sb *bun.SelectQuery, q map[string][]string) {
	get := func(k string) string {
		if v, ok := q[k]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}

	if v := get("minDist"); v != "" {
		sb.Where("rc.distance >= ?", v)
	}
	if v := get("maxDist"); v != "" {
		sb.Where("rc.distance <= ?", v)
	}
	if v := get("minMr2"); v != "" {
		sb.Where("rc.mr2 >= ?", v)
	}
	if v := get("btnDist"); v != "" {
		sb.Where("r.dist_behind_winner <= ?", v)
	}
	if v := get("minWinD"); v != "" {
		sb.Where("r.win_dist >= ?", v)
	}
	if v := get("minTFSF"); v != "" {
		sb.Where("r.tfsf >= ?", v)
	}
	if v := get("maxClass"); v != "" && v != "4" {
		sb.Where("rc.class <= ?", v)
	}

	switch get("trType") {
	case "aw":
		sb.Where("c.is_aw")
	case "turf":
		sb.Where("NOT c.is_aw")
	}

	switch get("handed") {
	case "L", "R", "S":
		sb.Where("c.direction = ?", get("handed"))
	}

	if get("crsForm") == "1" {
		sb.Where("c.course = ?", get("course"))
	}

	if v := get("going"); v != "" && v != "All" {
		sb.Where("rc.going = ?", v)
	}

	mr, or_ := get("mr"), get("or")
	if v := get("minDiff"); v != "" && mr != "" && or_ != "" {
		sb.Where("r.mr2_plus_or IS NOT NULL AND (?::integer + ?::integer) - r.mr2_plus_or <= ?",
			mr, or_, v)
	}
	if v := get("maxDiff"); v != "" && mr != "" && or_ != "" {
		sb.Where("r.mr2_plus_or IS NOT NULL AND (?::integer + ?::integer) - r.mr2_plus_or >= ?",
			mr, or_, v)
	}
}
