package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// Claims extends jwt.RegisteredClaims with application-specific fields.
type Claims struct {
	Username string `json:"username"`
	UserHash string `json:"user_hash"`
	jwt.RegisteredClaims
}

// UserHashFromUsername returns a deterministic HMAC hash for the given username and key.
func UserHashFromUsername(username string, key []byte) string {
	normalized := strings.ToLower(strings.TrimSpace(username))
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}

// JWT returns an Echo middleware that validates the Authorization header token
// using the provided signing key.
func JWT(key []byte) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := c.Request().Header.Get("Authorization")
			if token == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "missing authorization header")
			}

			claims := &Claims{}
			tkn, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
				return key, nil
			})
			if err != nil {
				if err == jwt.ErrSignatureInvalid {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid token signature")
				}
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			if !tkn.Valid {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}

			c.Set("username", claims.Username)
			c.Set("user_hash", claims.UserHash)
			return next(c)
		}
	}
}
