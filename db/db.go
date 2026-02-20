package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"

	"github.com/padraicbc/mikeapi/config"
	"github.com/padraicbc/mikeapi/models"
)

// Setup opens a PostgreSQL connection using the provided config.
func Setup(cfg *config.Config) *bun.DB {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(cfg.PostgresDSN())))
	db := bun.NewDB(sqldb, pgdialect.New())

	if cfg.Debug {
		db.AddQueryHook(bundebug.NewQueryHook(bundebug.WithVerbose(true)))
	}

	if err := db.PingContext(context.Background()); err != nil {
		log.Fatal("failed to connect to database:", err)
	}

	return db
}

// CreateTables creates all tables in dependency order.
func CreateTables(ctx context.Context, db *bun.DB) error {
	tables := []interface{}{
		(*models.User)(nil),
		(*models.Course)(nil),
		(*models.Horse)(nil),
		(*models.Race)(nil),
		(*models.PreRace)(nil),
		(*models.Trainer)(nil),
		(*models.Intermediary)(nil),
		(*models.Result)(nil),
	}

	for _, model := range tables {
		if _, err := db.NewCreateTable().Model(model).IfNotExists().Exec(ctx); err != nil {
			return fmt.Errorf("creating table for %T: %w", model, err)
		}
	}

	constraints := []string{
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'races_no_dupes') THEN ALTER TABLE races ADD CONSTRAINT races_no_dupes UNIQUE (course_id, date, time); END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'intermediary_no_dupes') THEN ALTER TABLE intermediary ADD CONSTRAINT intermediary_no_dupes UNIQUE (race_id, horse_id); END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'results_no_dupes') THEN ALTER TABLE results ADD CONSTRAINT results_no_dupes UNIQUE (race_id, horse_id); END IF; END $$`,
	}
	for _, stmt := range constraints {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			log.Printf("constraint: %v", err)
		}
	}

	return nil
}
