package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
	"go.uber.org/zap"

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
		zap.L().Fatal("failed to connect to database", zap.Error(err))
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
			zap.L().Warn("constraint setup failed", zap.Error(err))
		}
	}

	// Runners are stored as a JSON array, so a conventional unique index cannot
	// enforce the one-horse-per-race invariant. Keep that invariant in Postgres
	// rather than trusting scraper or UI validation.
	if _, err := db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION pre_race_runners_have_unique_horse_ids(runners jsonb)
		RETURNS boolean
		LANGUAGE sql
		IMMUTABLE
		AS $$
			SELECT CASE
				WHEN jsonb_typeof(runners) <> 'array' THEN false
				ELSE NOT EXISTS (
					SELECT runner->>'horseID'
					FROM jsonb_array_elements(runners) AS runner
					WHERE COALESCE(runner->>'horseID', '') <> ''
					GROUP BY runner->>'horseID'
					HAVING count(*) > 1
				)
			END
		$$`); err != nil {
		return fmt.Errorf("creating pre-race runner uniqueness function: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'pre_race_unique_runner_horse_ids'
			) THEN
				ALTER TABLE pre_race
				ADD CONSTRAINT pre_race_unique_runner_horse_ids
				CHECK (pre_race_runners_have_unique_horse_ids(runners));
			END IF;
		END $$`); err != nil {
		return fmt.Errorf("enforcing unique horse IDs in pre-race runners: %w", err)
	}

	return nil
}
