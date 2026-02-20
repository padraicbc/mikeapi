package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/padraicbc/mikeapi/models"
)

type trainerText struct {
	Trainer string `json:"trainer,omitempty"`
	Text    string `json:"text,omitempty"`
}

// GetTrainerText returns the notes for a single trainer.
func (h *Handler) GetTrainerText(c echo.Context) error {
	tr := c.QueryParam("tr")
	if tr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tr param not set")
	}

	trainer := &models.Trainer{}
	err := h.db.NewSelect().Model(trainer).
		Where("trainer = ?", tr).
		Scan(c.Request().Context())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	info := ""
	if trainer.Info != nil {
		info = *trainer.Info
	}
	return c.JSON(http.StatusOK, trainerText{trainer.Trainer, info})
}

// GetAllTrainers searches trainers by name pattern.
func (h *Handler) GetAllTrainers(c echo.Context) error {
	tr := c.QueryParam("tr")
	if tr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tr param not set")
	}

	var names []string
	err := h.db.NewSelect().
		TableExpr("trainers").
		ColumnExpr("trainer").
		Where("trainer ILIKE ?", fmt.Sprintf("%%%s%%", tr)).
		Scan(c.Request().Context(), &names)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, names)
}

// SaveTrainerText updates the info/notes for a trainer.
func (h *Handler) SaveTrainerText(c echo.Context) error {
	tr := c.QueryParam("tr")
	if tr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tr param not set")
	}

	bdy, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer c.Request().Body.Close()

	_, err = h.db.NewUpdate().
		TableExpr("trainers").
		Set("info = ?", string(bdy)).
		Where("trainer = ?", tr).
		Exec(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusOK)
}
