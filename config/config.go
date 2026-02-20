// Package config loads application settings from a .env file and environment variables.
// Environment variables always take precedence over .env file values.
package config

import (
	"fmt"
	"log"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	// PostgreSQL – either set DatabaseURL directly, or the individual fields.
	DatabaseURL string
	DBUser      string
	DBPass      string
	DBHost      string
	DBPort      string
	DBName      string

	// JWT signing secret (required in production).
	JWTSecret string

	// Server
	Debug      bool
	Port       string
	TLSDomains []string

	// MySQL – used only by cmd/migrate.
	MySQLDSN string
}

// Load reads configuration from a .env file (if present) and then from
// environment variables. Environment variables always win.
func Load() *Config {
	// Silently load .env – OK if the file doesn't exist (production uses real env vars).
	if err := godotenv.Load(); err != nil {
		log.Println("config: no .env file found, using environment variables only")
	}

	v := viper.New()
	v.AutomaticEnv()

	// Defaults
	v.SetDefault("DB_USER", "padraic")
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", "5432")
	v.SetDefault("DB_NAME", "rpdata")
	v.SetDefault("PORT", ":9000")
	v.SetDefault("TLS_DOMAINS", "mmrace.app,www.mmrace.app")
	v.SetDefault("DEBUG", false)

	cfg := &Config{
		DatabaseURL: v.GetString("DATABASE_URL"),
		DBUser:      v.GetString("DB_USER"),
		DBPass:      v.GetString("DB_PASS"),
		DBHost:      v.GetString("DB_HOST"),
		DBPort:      v.GetString("DB_PORT"),
		DBName:      v.GetString("DB_NAME"),
		JWTSecret:   v.GetString("JWT_SECRET"),
		Debug:       v.GetBool("DEBUG"),
		Port:        v.GetString("PORT"),
		TLSDomains:  splitTrimmed(v.GetString("TLS_DOMAINS")),
		MySQLDSN:    v.GetString("MYSQL_DSN"),
	}

	cfg.validate()
	return cfg
}

// PostgresDSN returns the full PostgreSQL connection string.
// DATABASE_URL takes precedence over individual fields.
func (c *Config) PostgresDSN() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBName,
	)
}

// JWTKey returns the JWT signing key as a byte slice.
func (c *Config) JWTKey() []byte {
	return []byte(c.JWTSecret)
}

func (c *Config) validate() {
	if c.DatabaseURL == "" && c.DBPass == "" {
		log.Fatal("config: DATABASE_URL or DB_PASS must be set")
	}
	if c.JWTSecret == "" {
		log.Fatal("config: JWT_SECRET must be set")
	}
}

func splitTrimmed(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
