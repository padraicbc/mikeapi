package handlers

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	mw "github.com/padraicbc/mikeapi/middleware"
	"github.com/padraicbc/mikeapi/models"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HashPasswordForUser validates username/password input and returns a bcrypt hash for storage.
func HashPasswordForUser(username, password string) (string, error) {
	if strings.TrimSpace(username) == "" {
		return "", errors.New("username is required")
	}
	if strings.TrimSpace(password) == "" {
		return "", errors.New("password is required")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hashedPassword), nil
}

func isAdminUser(username string) bool {
	adminUsers := strings.TrimSpace(os.Getenv("ADMIN_USERS"))
	if adminUsers == "" {
		adminUsers = "admin"
	}

	normalizedUsername := strings.ToLower(strings.TrimSpace(username))
	for _, admin := range strings.Split(adminUsers, ",") {
		if normalizedUsername == strings.ToLower(strings.TrimSpace(admin)) {
			return true
		}
	}

	return false
}

// PasswordHash returns a bcrypt hash from username/password input for manual user registration.
// Access is limited to authenticated admin users.
func (h *Handler) PasswordHash(c echo.Context) error {
	requester, _ := c.Get("username").(string)
	requester = strings.TrimSpace(requester)
	if requester == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	exists, err := h.db.NewSelect().Model((*models.User)(nil)).
		Where("username = ?", requester).
		Exists(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !exists {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if !isAdminUser(requester) {
		return echo.NewHTTPError(http.StatusForbidden, "admin access required")
	}

	var creds credentials
	if err := c.Bind(&creds); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	hash, err := HashPasswordForUser(creds.Username, creds.Password)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{
		"username":      strings.TrimSpace(creds.Username),
		"password_hash": hash,
	})
}

// Signin validates credentials and returns a JWT token valid for 30 days.
func (h *Handler) Signin(c echo.Context) error {
	var creds credentials
	if err := c.Bind(&creds); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	creds.Username = strings.TrimSpace(creds.Username)

	user := &models.User{}
	err := h.db.NewSelect().Model(user).
		Where("username = ?", creds.Username).
		Scan(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "incorrect username or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(creds.Password)); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	expiresAt := time.Now().AddDate(0, 0, 30)
	claims := &mw.Claims{
		Username: creds.Username,
		UserHash: mw.UserHashFromUsername(creds.Username, h.JWTKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.JWTKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"token": tokenString})
}
