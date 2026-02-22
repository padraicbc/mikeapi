package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

type preRaceSaveJSON struct {
	Time      string          `json:"time"`
	Course    string          `json:"course"`
	CourseID  string          `json:"courseID"`
	RaceID    string          `json:"raceID"`
	Date      string          `json:"date,omitempty"`
	Runners   json.RawMessage `json:"runners"`
	Direction string          `json:"direction"`
	Distance  float64         `json:"distance"`
	URL       string          `json:"url"`
	IsAW      bool            `json:"isAw"`
	Mr        *int            `json:"mr,omitempty"`
	Class     *string         `json:"class,omitempty"`
}

type interMed struct {
	HorseID  string `json:"horseID"`
	RaceID   string `json:"raceID"`
	MrPlusOr string `json:"mrPlusOr"`
	Tfr      string `json:"tfr"`
}

// GetPreRace returns pre-race card data for a given date.
func (h *Handler) GetPreRace(c echo.Context) error {
	date := c.QueryParam("date")
	if date == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing date param")
	}

	type preRaceRow struct {
		Course    string          `bun:"course"`
		CourseID  int             `bun:"course_id"`
		RaceID    int             `bun:"race_id"`
		Time      string          `bun:"time"`
		Direction string          `bun:"direction"`
		Distance  float64         `bun:"distance"`
		Runners   json.RawMessage `bun:"runners"`
		URL       string          `bun:"url"`
		Mr        *int            `bun:"mr"`
		Class     *string         `bun:"class"`
	}

	var rows []preRaceRow
	err := h.db.NewRaw(`
		SELECT pr.course, pr.course_id, pr.race_id, pr.time, pr.direction,
		       pr.distance, pr.runners, pr.url, rc.mr, rc.class
		FROM pre_race pr
		INNER JOIN races rc ON rc.race_id = pr.race_id
		WHERE pr.date = ?`,
		date,
	).Scan(c.Request().Context(), &rows)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	result := make([]preRaceSaveJSON, len(rows))
	for i, row := range rows {
		result[i] = preRaceSaveJSON{
			Course:    row.Course,
			CourseID:  fmt.Sprintf("%d", row.CourseID),
			RaceID:    fmt.Sprintf("%d", row.RaceID),
			Time:      row.Time,
			Direction: row.Direction,
			Distance:  row.Distance,
			Runners:   row.Runners,
			URL:       row.URL,
			Mr:        row.Mr,
			Class:     row.Class,
		}
	}

	return c.JSON(http.StatusOK, result)
}

// SaveToIntermediary saves pre-race MR+OR/TFR data and marks the race pre_done.
func (h *Handler) SaveToIntermediary(c echo.Context) error {
	raceID := c.QueryParam("raceID")
	if raceID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing raceID param")
	}
	mr := c.QueryParam("mr")
	if mr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing mr param")
	}

	var pre []interMed
	if err := c.Bind(&pre); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	ctx := c.Request().Context()
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		lastErr = doInterInsert(ctx, h.db, pre, mr, raceID)
		if lastErr == nil {
			break
		}
		fmt.Printf("SaveToIntermediary attempt %d: %v\n", attempt+1, lastErr)
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, lastErr.Error())
	}

	return c.NoContent(http.StatusAccepted)
}

func doInterInsert(ctx context.Context, db *bun.DB, pre []interMed, mr, raceID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, im := range pre {
		tfr := nullableString(im.Tfr)
		mrPlusOr := nullableString(im.MrPlusOr)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO intermediary (horse_id, race_id, mr_plus_or, tfr)
			 VALUES (?, ?, NULLIF(?,'')::integer, NULLIF(?,''))
			 ON CONFLICT (race_id, horse_id)
			 DO UPDATE SET mr_plus_or = EXCLUDED.mr_plus_or, tfr = EXCLUDED.tfr`,
			im.HorseID, im.RaceID, mrPlusOr, tfr,
		)
		if err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE races SET pre_done = true, mr = NULLIF(?,'')::integer WHERE race_id = ?`,
		mr, raceID,
	)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true

	return nil
}

// UpdatePreRace updates runners JSON in pre_race and mr in races.
func (h *Handler) UpdatePreRace(c echo.Context) error {
	mr := c.QueryParam("mr")
	raceID := c.QueryParam("raceID")
	if raceID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing raceID param")
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	payload := strings.TrimSpace(string(body))
	if payload == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "empty runners payload")
	}
	if !json.Valid([]byte(payload)) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid runners JSON payload")
	}

	ctx := c.Request().Context()
	var lastErr error
	for attempt := range 5 {
		lastErr = func() error {
			tx, err := h.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			committed := false
			defer func() {
				if !committed {
					_ = tx.Rollback()
				}
			}()

			if _, err = tx.ExecContext(ctx,
				`UPDATE pre_race SET runners = ?::jsonb WHERE race_id = ?`, payload, raceID,
			); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx,
				`UPDATE races SET mr = NULLIF(?,'')::integer WHERE race_id = ?`, mr, raceID,
			); err != nil {
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			committed = true

			return nil
		}()
		if lastErr == nil {
			break
		}
		fmt.Printf("UpdatePreRace attempt %d: %v\n", attempt+1, lastErr)
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, lastErr.Error())
	}

	return c.NoContent(http.StatusAccepted)
}

// nullableString returns empty string as-is; bun/pgdriver handles NULLIF conversion in SQL.
func nullableString(s string) string { return s }
