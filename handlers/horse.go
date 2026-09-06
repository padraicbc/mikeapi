package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

type horseProfileRun struct {
	Date        string  `json:"date" bun:"date"`
	Time        string  `json:"time" bun:"time"`
	Course      string  `json:"course" bun:"course"`
	Distance    float64 `json:"distance" bun:"distance"`
	Class       *string `json:"class,omitempty" bun:"class"`
	Going       string  `json:"going" bun:"going"`
	Placed      string  `json:"placed" bun:"placed"`
	Age         int     `json:"age" bun:"age"`
	Trainer     string  `json:"trainer" bun:"trainer"`
	Jockey      string  `json:"jockey" bun:"jockey"`
	Price       string  `json:"price" bun:"price"`
	OfficialRat *int    `json:"officialRat,omitempty" bun:"official_rat"`
	RPR         *int    `json:"rpr,omitempty" bun:"rpr"`
	TS          *int    `json:"ts,omitempty" bun:"ts"`
	URL         string  `json:"url" bun:"url"`
}

type horseProfile struct {
	HorseID  int               `json:"horseID"`
	Horse    string            `json:"horse"`
	Age      *int              `json:"age,omitempty"`
	Trainer  string            `json:"trainer,omitempty"`
	Runs     int               `json:"runs"`
	Wins     int               `json:"wins"`
	WinRate  float64           `json:"winRate"`
	RaceRuns []horseProfileRun `json:"raceRuns"`
}

// HorseProfile returns a horse's stored racing history and summary statistics.
func (h *Handler) HorseProfile(c echo.Context) error {
	horseID, err := strconv.Atoi(c.Param("horseID"))
	if err != nil || horseID < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid horse ID")
	}

	var profile horseProfile
	if err := h.db.NewRaw(`SELECT horse_id, horse FROM horses WHERE horse_id = ?`, horseID).
		Scan(c.Request().Context(), &profile); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "horse not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if profile.HorseID == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "horse not found")
	}

	if err := h.db.NewRaw(`
		SELECT rc.date::text AS date, rc.time, c.course, rc.distance, rc.class, rc.going,
		       r.placed, r.age, r.trainer, r.jockey, r.price, r.official_rat, r.rpr, r.ts, rc.url
		FROM results r
		INNER JOIN races rc ON rc.race_id = r.race_id
		INNER JOIN courses c ON c.course_id = r.course_id
		WHERE r.horse_id = ?
		ORDER BY rc.date DESC, rc.time DESC`, horseID).Scan(c.Request().Context(), &profile.RaceRuns); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	profile.Runs = len(profile.RaceRuns)
	for i, run := range profile.RaceRuns {
		if run.Placed == "1" {
			profile.Wins++
		}
		if i == 0 {
			age := run.Age
			profile.Age = &age
			profile.Trainer = run.Trainer
		}
	}
	if profile.Runs > 0 {
		profile.WinRate = float64(profile.Wins) * 100 / float64(profile.Runs)
	}

	// A forthcoming card is more current than the horse's latest completed
	// result. Its runner row is refreshed whenever the daily card is scraped.
	var currentCard struct {
		Age     *int   `bun:"age"`
		Trainer string `bun:"trainer"`
	}
	if err := h.db.NewRaw(`
		SELECT NULLIF(prr.runner->>'age', '')::integer AS age,
		       COALESCE(prr.runner->>'trainer', '') AS trainer
		FROM pre_race_runners prr
		INNER JOIN races rc ON rc.race_id = prr.race_id
		WHERE prr.horse_id = ?
		ORDER BY rc.date DESC, rc.time DESC
		LIMIT 1`, horseID).Scan(c.Request().Context(), &currentCard); err == nil && currentCard.Age != nil {
		profile.Age = currentCard.Age
		if currentCard.Trainer != "" {
			profile.Trainer = currentCard.Trainer
		}
	}

	return c.JSON(http.StatusOK, profile)
}
