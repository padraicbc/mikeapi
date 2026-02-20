package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/padraicbc/mikeapi/models"
)

type courseData struct {
	CourseID  int    `json:"courseID"`
	Course    string `json:"course"`
	Direction string `json:"direction"`
	IsAW      bool   `json:"isAw"`
}

// Courses returns all courses, optionally filtered by race date.
func (h *Handler) Courses(c echo.Context) error {
	date := c.QueryParam("date")

	var courses []models.Course
	q := h.db.NewSelect().
		Model(&courses).
		Column("c.course_id", "c.course", "c.direction", "c.is_aw")

	if date != "" {
		q = q.Join("INNER JOIN races rc ON rc.course_id = c.course_id").
			Where("rc.date = ?", date)
	}

	if err := q.Scan(c.Request().Context()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	result := make([]courseData, len(courses))
	for i, cr := range courses {
		result[i] = courseData{
			CourseID:  cr.CourseID,
			Course:    cr.Course,
			Direction: cr.Direction,
			IsAW:      cr.IsAW,
		}
	}

	return c.JSON(http.StatusOK, result)
}

// Dates returns all distinct race dates, optionally filtered by course ID.
func (h *Handler) Dates(c echo.Context) error {
	courseID := c.QueryParam("courseID")

	var dates []string
	q := h.db.NewSelect().
		TableExpr("races").
		ColumnExpr("DISTINCT date::text").
		OrderExpr("date DESC")

	if courseID != "" {
		q = q.Where("course_id = ?", courseID)
	}

	if err := q.Scan(c.Request().Context(), &dates); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, dates)
}
