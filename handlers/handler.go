package handlers

import "github.com/uptrace/bun"

// Handler holds shared dependencies used by all route handlers.
type Handler struct {
	db     *bun.DB
	JWTKey []byte
}

// New creates a Handler with the given database connection and JWT signing key.
func New(db *bun.DB, jwtKey []byte) *Handler {
	return &Handler{db: db, JWTKey: jwtKey}
}
