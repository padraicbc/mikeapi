package models

import "github.com/uptrace/bun"

// User is an API user with bcrypt-hashed password.
type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`

	ID       int    `bun:"id,pk,autoincrement" json:"id"`
	Username string `bun:"username,notnull,unique" json:"username"`
	Password string `bun:"password,notnull" json:"-"`
}
