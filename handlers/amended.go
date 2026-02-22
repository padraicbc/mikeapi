package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

// jsonText accepts string, number, or null JSON values and normalizes to string.
type jsonText string

func (t *jsonText) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*t = ""
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*t = jsonText(s)
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var n json.Number
	if err := dec.Decode(&n); err == nil {
		*t = jsonText(n.String())
		return nil
	}

	return fmt.Errorf("expected string, number, or null")
}

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
		Placed     jsonText `json:"placed,omitempty"`
		DistBehind jsonText `json:"distBehindWinner"`
		Comment    jsonText `json:"comment"`
	}

	var fields []rowUpdate
	if err := c.Bind(&fields); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, ru := range fields {
		_, err = tx.ExecContext(ctx,
			`UPDATE results SET placed = ?, dist_behind_winner = NULLIF(?,'')::numeric, comment = NULLIF(?,'') WHERE id = ?`,
			string(ru.Placed), string(ru.DistBehind), string(ru.Comment), ru.ID,
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
	committed = true

	return c.NoContent(http.StatusAccepted)
}
