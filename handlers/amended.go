package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// ResultsAmended returns all races flagged as amended, grouped by race.
func (h *Handler) ResultsAmended(c echo.Context) error {
	var rows []resultsAnalysisRow
	q := resultsJoinSQL + `WHERE rc.amended = true ORDER BY r.race_id, LENGTH(r.placed), r.placed`

	if err := h.db.NewRaw(q).Scan(c.Request().Context(), &rows); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, groupResultsByRace(rows))
}

// UpdateAmended corrects placed/dist-behind-winner for amended races.
func (h *Handler) UpdateAmended(c echo.Context) error {
	raceID := c.QueryParam("raceID")
	if raceID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing raceID param")
	}
	comment := c.QueryParam("comment")

	type rowUpdate struct {
		ID         string `json:"id"`
		Placed     string `json:"placed,omitempty"`
		DistBehind string `json:"distBehindWinner"`
		Comment    string `json:"comment"`
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
			`UPDATE results SET placed = ?, dist_behind_winner = NULLIF(?,'')::numeric, comment = NULLIF(?,'') WHERE id = ?`,
			ru.Placed, ru.DistBehind, ru.Comment, ru.ID,
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE races SET amended = false, main_comment = NULLIF(?,'') WHERE race_id = ?`,
		comment, raceID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err = tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusAccepted)
}
