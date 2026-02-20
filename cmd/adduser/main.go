// cmd/adduser/main.go
// Creates or updates a user in the database.
//
// Usage:
//
//	go run ./cmd/adduser -username padraic -password testing
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"

	"github.com/padraicbc/mikeapi/config"
	bundb "github.com/padraicbc/mikeapi/db"
	"github.com/padraicbc/mikeapi/models"
)

func main() {
	username := flag.String("username", "", "username (required)")
	password := flag.String("password", "", "plain-text password (required)")
	flag.Parse()

	if *username == "" || *password == "" {
		log.Fatal("both -username and -password are required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("bcrypt:", err)
	}

	cfg := config.Load()
	db := bundb.Setup(cfg)
	defer db.Close()

	user := &models.User{
		Username: *username,
		Password: string(hash),
	}

	_, err = db.NewInsert().Model(user).
		On("CONFLICT (username) DO UPDATE SET password = EXCLUDED.password").
		Exec(context.Background())
	if err != nil {
		log.Fatal("insert user:", err)
	}

	fmt.Printf("user %q saved\n", *username)
}
