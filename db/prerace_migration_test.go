package db

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// This test requires an explicitly supplied disposable database.
func TestPreRaceMigration(t *testing.T) {
	dsn := os.Getenv("PRERACE_TEST_DSN")
	if dsn == "" {
		t.Skip("set PRERACE_TEST_DSN to a disposable PostgreSQL database")
	}
	ctx := context.Background()
	sqlDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	sqlDB.SetMaxOpenConns(1)
	db := bun.NewDB(sqlDB, pgdialect.New())
	defer db.Close()
	exec := func(query string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE SCHEMA prerace_migration_test`)
	defer db.ExecContext(ctx, `DROP SCHEMA prerace_migration_test CASCADE`)
	exec(`SET search_path TO prerace_migration_test`)
	if err := CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	var legacy bool
	if err := db.NewRaw(`SELECT to_regclass('pre_race') IS NOT NULL`).Scan(ctx, &legacy); err != nil || legacy {
		t.Fatalf("fresh setup created legacy table: %v", err)
	}
	exec(`INSERT INTO courses (course_id, course, direction, is_aw, code) VALUES (1, 'Course', 'L', false, 'C')`)
	exec(`INSERT INTO horses (horse_id, horse) VALUES (1, 'One'), (2, 'Two')`)
	exec(`INSERT INTO races (race_id, course_id, date, time, url, distance, going) VALUES
  (1, 1, '2026-09-06', '12:00', 'one', 8, 'G'),
  (2, 1, '2026-09-06', '13:00', 'two', 8, 'G')`)
	exec(`CREATE TABLE pre_race (race_id integer PRIMARY KEY, runners jsonb NOT NULL)`)
	exec(`INSERT INTO pre_race VALUES
  (1, '[{"horseID":"1","trainer":"legacy"}]'),
  (2, '[{"horseID":"2","trainer":"stale"}]')`)
	exec(`ALTER TABLE pre_race_runners DROP CONSTRAINT pre_race_runners_race_fk`)
	exec(`ALTER TABLE pre_race_runners ADD CONSTRAINT pre_race_runners_race_fk FOREIGN KEY (race_id) REFERENCES pre_race (race_id)`)
	exec(`INSERT INTO pre_race_runners (race_id, horse_id, runner) VALUES (2, 2, '{"horseID":"2","trainer":"edited"}')`)
	for range 2 {
		if err := CreateTables(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.NewRaw(`SELECT count(*) FROM pre_race_runners`).Scan(ctx, &count); err != nil || count != 2 {
		t.Fatalf("expected two migrated runners, got %d: %v", count, err)
	}
	var trainer string
	if err := db.NewRaw(`SELECT runner->>'trainer' FROM pre_race_runners WHERE race_id = 2`).Scan(ctx, &trainer); err != nil || trainer != "edited" {
		t.Fatalf("edited runner overwritten: %q %v", trainer, err)
	}
	var target string
	if err := db.NewRaw(`SELECT confrelid::regclass::text FROM pg_constraint WHERE conrelid = 'pre_race_runners'::regclass AND conname = 'pre_race_runners_race_fk'`).Scan(ctx, &target); err != nil || target != "races" {
		t.Fatalf("wrong foreign key target %q: %v", target, err)
	}
	exec(`DELETE FROM pre_race_runners WHERE race_id = 1`)
	if err := CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.NewRaw(`SELECT count(*) FROM pre_race_runners WHERE race_id = 1`).Scan(ctx, &count); err != nil || count != 0 {
		t.Fatalf("archived card was reimported after deletion: %v", err)
	}
	exec(`INSERT INTO pre_race_runners (race_id, horse_id, runner) VALUES (1, 1, '{}')`)
	exec(`DELETE FROM races WHERE race_id = 1`)
	if err := db.NewRaw(`SELECT count(*) FROM pre_race_runners WHERE race_id = 1`).Scan(ctx, &count); err != nil || count != 0 {
		t.Fatalf("race deletion did not cascade: %v", err)
	}
}
