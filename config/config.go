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
	DBSSLMode   string

	// JWT signing secret (required in production).
	JWTSecret string

	// Server
	Debug      bool
	Port       string
	TLSDomains []string

	// MySQL – used only by cmd/migrate.
	MySQLDSN string
}

// RPConfig holds configuration used by the mikerp scraper app.
type RPConfig struct {
	DatabaseURL string
	DBUser      string
	DBPass      string
	// RPPass preserves compatibility with existing mikerp env files.
	RPPass    string
	DBHost    string
	DBPort    string
	DBName    string
	DBSSLMode string

	ReportTo   string
	ReportFrom string
}

// Load reads configuration from a .env file (if present) and then from
// environment variables. Environment variables always win.
func Load() *Config {
	v := newViper()

	// Defaults
	v.SetDefault("DB_USER", "padraic")
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", "5432")
	v.SetDefault("DB_NAME", "rpdata")
	v.SetDefault("DB_SSLMODE", "disable")
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
		DBSSLMode:   v.GetString("DB_SSLMODE"),
		JWTSecret:   v.GetString("JWT_SECRET"),
		Debug:       v.GetBool("DEBUG"),
		Port:        v.GetString("PORT"),
		TLSDomains:  splitTrimmed(v.GetString("TLS_DOMAINS")),
		MySQLDSN:    v.GetString("MYSQL_DSN"),
	}

	cfg.validate()
	return cfg
}

// LoadRP reads config shared by mikerp from .env and environment variables.
func LoadRP() *RPConfig {
	v := newViper()

	// Defaults
	v.SetDefault("DB_USER", "padraic")
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", "5432")
	v.SetDefault("DB_NAME", "rpdata")
	v.SetDefault("DB_SSLMODE", "disable")
	v.SetDefault("RP_REPORT_TO", "pcunningham80@gmail.com")
	v.SetDefault("RP_REPORT_FROM", "error@mail.padraicbc.com")

	cfg := &RPConfig{
		DatabaseURL: v.GetString("DATABASE_URL"),
		DBUser:      v.GetString("DB_USER"),
		DBPass:      v.GetString("DB_PASS"),
		RPPass:      v.GetString("RPPASS"),
		DBHost:      v.GetString("DB_HOST"),
		DBPort:      v.GetString("DB_PORT"),
		DBName:      v.GetString("DB_NAME"),
		DBSSLMode:   v.GetString("DB_SSLMODE"),
		ReportTo:    v.GetString("RP_REPORT_TO"),
		ReportFrom:  v.GetString("RP_REPORT_FROM"),
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
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser,
		c.DBPass,
		c.DBHost,
		c.DBPort,
		c.DBName,
		c.DBSSLMode,
	)
}

// JWTKey returns the JWT signing key as a byte slice.
func (c *Config) JWTKey() []byte {
	return []byte(c.JWTSecret)
}

// PostgresDSN returns the full PostgreSQL connection string.
// DATABASE_URL takes precedence over individual fields.
func (c *RPConfig) PostgresDSN() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}

	pass := c.DBPass
	if pass == "" {
		pass = c.RPPass
	}

	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser,
		pass,
		c.DBHost,
		c.DBPort,
		c.DBName,
		c.DBSSLMode,
	)
}

func (c *Config) validate() {
	if c.DatabaseURL == "" && c.DBPass == "" {
		log.Fatal("config: DATABASE_URL or DB_PASS must be set")
	}
	if c.JWTSecret == "" {
		log.Fatal("config: JWT_SECRET must be set")
	}
}

func (c *RPConfig) validate() {
	if c.DatabaseURL == "" && c.DBPass == "" && c.RPPass == "" {
		log.Fatal("config: DATABASE_URL or DB_PASS (or legacy RPPASS) must be set")
	}
}

func newViper() *viper.Viper {
	// Silently load .env – OK if the file doesn't exist (production uses real env vars).
	if err := godotenv.Load(); err != nil {
		log.Println("config: no .env file found, using environment variables only")
	}

	v := viper.New()
	v.AutomaticEnv()
	return v
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
