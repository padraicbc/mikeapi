package handlers

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/padraicbc/mikeapi/models"
)

type courseData struct {
	CourseID  int    `json:"courseID"`
	Course    string `json:"course"`
	Direction string `json:"direction"`
	IsAW      bool   `json:"isAw"`
	Code      string `json:"code"`
}

type createCourseRequest struct {
	Course    string `json:"course"`
	Direction string `json:"direction"`
	IsAW      bool   `json:"isAw"`
	Code      string `json:"code"`
}

// Courses returns all courses, optionally filtered by race date.
func (h *Handler) Courses(c echo.Context) error {
	date := c.QueryParam("date")

	var courses []models.Course
	q := h.db.NewSelect().
		Distinct().
		Model(&courses).
		Column("c.course_id", "c.course", "c.direction", "c.is_aw", "c.code").
		OrderExpr("c.course ASC")

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
			Code:      cr.Code,
		}
	}

	return c.JSON(http.StatusOK, result)
}

// CreateCourse inserts a new course.
func (h *Handler) CreateCourse(c echo.Context) error {
	var req createCourseRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	req.Course = strings.TrimSpace(req.Course)
	req.Direction = strings.ToUpper(strings.TrimSpace(req.Direction))
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))

	if req.Course == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "course is required")
	}
	if req.Direction == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "direction is required")
	}
	if req.Direction != "R" && req.Direction != "L" {
		return echo.NewHTTPError(http.StatusBadRequest, "direction must be R or L")
	}
	if req.Code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code is required")
	}
	if req.Code != "GB" && req.Code != "IRE" {
		return echo.NewHTTPError(http.StatusBadRequest, "code must be GB or IRE")
	}

	course := &models.Course{
		Course:    req.Course,
		Direction: req.Direction,
		IsAW:      req.IsAW,
		Code:      req.Code,
	}

	if _, err := h.db.NewInsert().Model(course).Exec(c.Request().Context()); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key value") {
			return echo.NewHTTPError(http.StatusConflict, "course already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, courseData{
		CourseID:  course.CourseID,
		Course:    course.Course,
		Direction: course.Direction,
		IsAW:      course.IsAW,
		Code:      course.Code,
	})
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
