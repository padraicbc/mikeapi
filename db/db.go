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
		(*models.ResultImport)(nil),
		(*models.MeetingDiscovery)(nil),
		(*models.PreRaceRunner)(nil),
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
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'result_imports'::regclass AND conname = 'result_imports_race_id_fkey') THEN ALTER TABLE result_imports ADD CONSTRAINT result_imports_race_id_fkey FOREIGN KEY (race_id) REFERENCES races (race_id) ON DELETE CASCADE; END IF; END $$`,
		`ALTER TABLE races ADD COLUMN IF NOT EXISTS band_start integer`,
		`ALTER TABLE races ADD COLUMN IF NOT EXISTS band_end integer`,
		`ALTER TABLE races ADD COLUMN IF NOT EXISTS age_restriction text`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'races_no_dupes') THEN ALTER TABLE races ADD CONSTRAINT races_no_dupes UNIQUE (course_id, date, time); END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'intermediary_no_dupes') THEN ALTER TABLE intermediary ADD CONSTRAINT intermediary_no_dupes UNIQUE (race_id, horse_id); END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'results_no_dupes') THEN ALTER TABLE results ADD CONSTRAINT results_no_dupes UNIQUE (race_id, horse_id); END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'pre_race_runners_horse_fk') THEN ALTER TABLE pre_race_runners ADD CONSTRAINT pre_race_runners_horse_fk FOREIGN KEY (horse_id) REFERENCES horses (horse_id); END IF; END $$`,
	}

	// Migrate legacy cards and repoint their runner foreign key atomically.
	// Keep the old table as an unused archive; fresh databases do not create it.
	if err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx, `
   DO $$ BEGIN
    IF to_regclass('pre_race') IS NOT NULL AND NOT EXISTS (
     SELECT 1 FROM pg_constraint WHERE conrelid = 'pre_race_runners'::regclass
      AND conname = 'pre_race_runners_race_fk' AND confrelid = 'races'::regclass
    ) THEN
     INSERT INTO pre_race_runners (race_id, horse_id, runner)
     SELECT pr.race_id, (runner->>'horseID')::integer, runner
     FROM pre_race pr
     CROSS JOIN LATERAL jsonb_array_elements(pr.runners) AS runner
     WHERE jsonb_typeof(pr.runners) = 'array'
      AND COALESCE(runner->>'horseID', '') ~ '^[0-9]+$'
      AND NOT EXISTS (SELECT 1 FROM pre_race_runners prr WHERE prr.race_id = pr.race_id)
     ON CONFLICT (race_id, horse_id) DO NOTHING;
    END IF;
   END $$`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
   DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint
     WHERE conrelid = 'pre_race_runners'::regclass
      AND conname = 'pre_race_runners_race_fk'
      AND confrelid <> 'races'::regclass) THEN
     ALTER TABLE pre_race_runners DROP CONSTRAINT pre_race_runners_race_fk;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
     WHERE conrelid = 'pre_race_runners'::regclass AND conname = 'pre_race_runners_race_fk') THEN
     ALTER TABLE pre_race_runners ADD CONSTRAINT pre_race_runners_race_fk
      FOREIGN KEY (race_id) REFERENCES races (race_id) ON DELETE CASCADE;
    END IF;
   END $$`)
		return err
	}); err != nil {
		return fmt.Errorf("migrating pre-race storage: %w", err)
	}

	// Scraped cards may omit a runner later recorded in the results (for example,
	// a late card change). Add only those missing rows so every recorded result
	// is available to form views, without changing a card's scraped runners.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO pre_race_runners (race_id, horse_id, runner)
		SELECT r.race_id, r.horse_id,
			jsonb_build_object(
				'horseID', r.horse_id::text,
				'horse', h.horse,
				'age', r.age::text,
				'rpr', COALESCE(r.rpr::text, ''),
				'ts', COALESCE(r.ts::text, ''),
				'tfr', r.tfr,
				'gear', r.headgear,
				'claim', r.claim,
				'drawn', '',
				'jockey', r.jockey,
				'number', r.number::text,
				'trainer', r.trainer,
				'courseWin', '',
				'hasRecord', true,
				'cardWeight', r.card_weight,
				'daysLastRun', '',
				'officialRat', COALESCE(r.official_rat::text, ''),
				'highestWinOr', h.highest_win_or,
				'lastRunClaim', h.last_run_claim,
				'lastWinClaim', h.last_win_claim,
				'lastRunWeight', h.last_run_weight,
				'lastWinWeight', h.last_win_weight,
				'weightCarried', r.weight_carried,
				'highestWinWeight', h.highest_win_weight
			)
		FROM results r
		INNER JOIN horses h ON h.horse_id = r.horse_id
		WHERE NOT EXISTS (
				SELECT 1 FROM pre_race_runners prr
				WHERE prr.race_id = r.race_id AND prr.horse_id = r.horse_id
			)
		ON CONFLICT (race_id, horse_id) DO NOTHING`); err != nil {
		return fmt.Errorf("filling result runners omitted from pre-race cards: %w", err)
	}
	for _, stmt := range constraints {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			zap.L().Warn("constraint setup failed", zap.Error(err))
		}
	}

	return nil
}
