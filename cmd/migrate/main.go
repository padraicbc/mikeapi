// cmd/migrate/main.go
// Migrates data between PostgreSQL rpdata databases.
//
// Usage:
//
//	SOURCE_DATABASE_URL="postgres://user:pass@source:5432/rpdata?sslmode=disable" \
//	DATABASE_URL="postgres://user:pass@target:5432/rpdata?sslmode=disable" \
//	go run ./cmd/migrate
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/padraicbc/mikeapi/config"
	bundb "github.com/padraicbc/mikeapi/db"
	"github.com/padraicbc/mikeapi/models"
)

const batchSize = 500

func main() {
	ctx := context.Background()

	cfg := config.Load()

	// --- source PostgreSQL ---
	if !cfg.HasPostgresSource() {
		log.Fatal("SOURCE_DATABASE_URL or SOURCE_DB_HOST is required")
	}
	sourceDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(cfg.SourcePostgresDSN())))
	defer sourceDB.Close()
	sourceDB.SetMaxOpenConns(4)
	if err := sourceDB.PingContext(ctx); err != nil {
		log.Fatalf("ping source PostgreSQL: %v", err)
	}
	log.Println("connected to source PostgreSQL")

	// --- PostgreSQL ---
	pgDB := bundb.Setup(cfg)
	defer pgDB.Close()
	log.Println("connected to PostgreSQL")

	// Create tables (idempotent)
	if err := bundb.CreateTables(ctx, pgDB); err != nil {
		if !isInsufficientPrivilege(err) {
			log.Fatalf("create tables: %v", err)
		}
		log.Printf("create tables skipped (insufficient privilege): %v", err)

		missing, checkErr := missingTables(ctx, pgDB)
		if checkErr != nil {
			log.Fatalf("verify tables after create skip: %v", checkErr)
		}
		if len(missing) > 0 {
			log.Fatalf("cannot continue: missing tables and no create privilege: %s", strings.Join(missing, ", "))
		}
		log.Println("all required tables already exist; continuing without create privilege")
	}

	// Disable FK enforcement so we can load in bulk without strict ordering
	if _, err := pgDB.ExecContext(ctx, "SET session_replication_role = 'replica'"); err != nil {
		if isInsufficientPrivilege(err) {
			log.Printf("disable FK skipped (insufficient privilege): %v", err)
		} else {
			log.Fatalf("disable FK: %v", err)
		}
	} else {
		defer func() {
			if _, err := pgDB.ExecContext(ctx, "SET session_replication_role = 'origin'"); err != nil {
				log.Printf("re-enable FK: %v", err)
			}
		}()
	}

	if n, err := migrateUsers(ctx, sourceDB, pgDB); err != nil {
		log.Fatalf("migrate users: %v", err)
	} else {
		log.Printf("%-20s %d rows migrated", "users", n)
	}
	if n, err := migrateCourses(ctx, sourceDB, pgDB); err != nil {
		log.Fatalf("migrate courses: %v", err)
	} else {
		log.Printf("%-20s %d rows migrated", "courses", n)
	}

	raceIDs, raceCount, err := migrateRaces(ctx, sourceDB, pgDB)
	if err != nil {
		log.Fatalf("migrate races: %v", err)
	}
	log.Printf("%-20s %d rows migrated (%d canonical target races)", "races", raceCount, uniqueMappedIDs(raceIDs))

	steps := []struct {
		name string
		fn   func() (int, error)
	}{
		{"horses", func() (int, error) { return migrateHorses(ctx, sourceDB, pgDB, raceIDs) }},
		{"trainers", func() (int, error) { return migrateTrainers(ctx, sourceDB, pgDB) }},
		{"pre_race_runners", func() (int, error) { return migratePreRace(ctx, sourceDB, pgDB, raceIDs) }},
		{"results", func() (int, error) { return migrateResults(ctx, sourceDB, pgDB, raceIDs) }},
		{"intermediary", func() (int, error) { return migrateIntermediary(ctx, sourceDB, pgDB, raceIDs) }},
	}

	for _, s := range steps {
		n, err := s.fn()
		if err != nil {
			log.Fatalf("migrate %s: %v", s.name, err)
		}
		log.Printf("%-20s %d rows migrated", s.name, n)
	}

	resetSequences(ctx, pgDB)
	log.Println("migration complete")
}

// --- helpers ---

func nullInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func nullStr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

func nullFloat(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	return &n.Float64
}

func fmtDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// bulkInsert inserts a batch, skipping rows that already exist (idempotent re-runs).
func bulkInsert[T any](ctx context.Context, pgDB *bun.DB, rows []T) error {
	if len(rows) == 0 {
		return nil
	}
	_, err := pgDB.NewInsert().Model(&rows).On("CONFLICT DO NOTHING").Exec(ctx)
	return err
}

func mappedRaceID(sourceID int, raceIDs map[int]int) (int, error) {
	targetID, ok := raceIDs[sourceID]
	if !ok {
		return 0, fmt.Errorf("source race %d was not migrated", sourceID)
	}
	return targetID, nil
}

func uniqueMappedIDs(raceIDs map[int]int) int {
	unique := make(map[int]struct{}, len(raceIDs))
	for _, targetID := range raceIDs {
		unique[targetID] = struct{}{}
	}
	return len(unique)
}

func sourceTableExists(ctx context.Context, sourceDB *sql.DB, table string) (bool, error) {
	var exists bool
	err := sourceDB.QueryRowContext(ctx,
		`SELECT to_regclass('public.' || $1) IS NOT NULL`, table,
	).Scan(&exists)
	return exists, err
}

// --- per-table migrations ---

func migrateUsers(ctx context.Context, sourceDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := sourceDB.QueryContext(ctx, "SELECT id, username, password FROM users")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var batch []models.User
	total := 0
	for rows.Next() {
		var r models.User
		if err := rows.Scan(&r.ID, &r.Username, &r.Password); err != nil {
			return total, err
		}
		batch = append(batch, r)
		if len(batch) >= batchSize {
			if err := bulkInsert(ctx, pgDB, batch); err != nil {
				return total, err
			}
			total += len(batch)
			batch = batch[:0]
		}
	}
	if err := bulkInsert(ctx, pgDB, batch); err != nil {
		return total, err
	}
	return total + len(batch), rows.Err()
}

func migrateCourses(ctx context.Context, sourceDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := sourceDB.QueryContext(ctx,
		"SELECT course_id, course, direction, is_aw, code FROM courses")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var batch []models.Course
	total := 0
	for rows.Next() {
		var r models.Course
		if err := rows.Scan(&r.CourseID, &r.Course, &r.Direction, &r.IsAW, &r.Code); err != nil {
			return total, err
		}
		batch = append(batch, r)
		if len(batch) >= batchSize {
			if err := bulkInsert(ctx, pgDB, batch); err != nil {
				return total, err
			}
			total += len(batch)
			batch = batch[:0]
		}
	}
	if err := bulkInsert(ctx, pgDB, batch); err != nil {
		return total, err
	}
	return total + len(batch), rows.Err()
}

func migrateHorses(ctx context.Context, sourceDB *sql.DB, pgDB *bun.DB, raceIDs map[int]int) (int, error) {
	rows, err := sourceDB.QueryContext(ctx,
		`SELECT horse_id, horse, last_win_id, highest_win_weight, last_win_weight,
		        last_run_weight, last_win_claim, last_run_claim, highest_win_or
		 FROM horses`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var batch []models.Horse
	total := 0
	for rows.Next() {
		var (
			horseID          int
			horse            string
			lastWinID        sql.NullInt64
			highestWinWeight sql.NullInt64
			lastWinWeight    sql.NullInt64
			lastRunWeight    sql.NullInt64
			lastWinClaim     sql.NullInt64
			lastRunClaim     sql.NullInt64
			highestWinOr     sql.NullInt64
		)
		if err := rows.Scan(&horseID, &horse, &lastWinID, &highestWinWeight, &lastWinWeight,
			&lastRunWeight, &lastWinClaim, &lastRunClaim, &highestWinOr); err != nil {
			return total, err
		}
		mappedLastWinID := nullInt(lastWinID)
		if mappedLastWinID != nil {
			mapped, mapErr := mappedRaceID(*mappedLastWinID, raceIDs)
			if mapErr != nil {
				return total, mapErr
			}
			mappedLastWinID = &mapped
		}
		batch = append(batch, models.Horse{
			HorseID:          horseID,
			Horse:            horse,
			LastWinID:        mappedLastWinID,
			HighestWinWeight: nullInt(highestWinWeight),
			LastWinWeight:    nullInt(lastWinWeight),
			LastRunWeight:    nullInt(lastRunWeight),
			LastWinClaim:     nullInt(lastWinClaim),
			LastRunClaim:     nullInt(lastRunClaim),
			HighestWinOr:     nullInt(highestWinOr),
		})
		if len(batch) >= batchSize {
			if err := bulkInsert(ctx, pgDB, batch); err != nil {
				return total, err
			}
			total += len(batch)
			batch = batch[:0]
		}
	}
	if err := bulkInsert(ctx, pgDB, batch); err != nil {
		return total, err
	}
	return total + len(batch), rows.Err()
}

func migrateTrainers(ctx context.Context, sourceDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := sourceDB.QueryContext(ctx,
		"SELECT trainer_id, trainer, info FROM trainers")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var batch []models.Trainer
	total := 0
	for rows.Next() {
		var (
			trainerID int
			trainer   string
			info      sql.NullString
		)
		if err := rows.Scan(&trainerID, &trainer, &info); err != nil {
			return total, err
		}
		batch = append(batch, models.Trainer{
			TrainerID: trainerID,
			Trainer:   trainer,
			Info:      nullStr(info),
		})
		if len(batch) >= batchSize {
			if err := bulkInsert(ctx, pgDB, batch); err != nil {
				return total, err
			}
			total += len(batch)
			batch = batch[:0]
		}
	}
	if err := bulkInsert(ctx, pgDB, batch); err != nil {
		return total, err
	}
	return total + len(batch), rows.Err()
}

func migrateRaces(ctx context.Context, sourceDB *sql.DB, pgDB *bun.DB) (map[int]int, int, error) {
	rows, err := sourceDB.QueryContext(ctx,
		`SELECT race_id, course_id, date, time, url, class, distance, going,
		        band_start, band_end, age_restriction, mr, mr2, analysed,
		        pre_done, main_comment, amended
		 FROM races
		 ORDER BY race_id`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	raceIDs := make(map[int]int)
	total := 0
	for rows.Next() {
		var (
			sourceRaceID   int
			courseID       int
			date           time.Time
			rtime          string
			url            string
			class          sql.NullString
			distance       float64
			going          string
			bandStart      sql.NullInt64
			bandEnd        sql.NullInt64
			ageRestriction sql.NullString
			mr             sql.NullInt64
			mr2            sql.NullInt64
			analysed       bool
			preDone        bool
			mainComment    sql.NullString
			amended        bool
		)
		if err := rows.Scan(&sourceRaceID, &courseID, &date, &rtime, &url, &class,
			&distance, &going, &bandStart, &bandEnd, &ageRestriction, &mr, &mr2,
			&analysed, &preDone, &mainComment, &amended); err != nil {
			return raceIDs, total, err
		}
		race := models.Race{
			RaceID:         sourceRaceID,
			CourseID:       courseID,
			Date:           fmtDate(date),
			Time:           rtime,
			URL:            url,
			Class:          nullStr(class),
			Distance:       distance,
			Going:          going,
			BandStart:      nullInt(bandStart),
			BandEnd:        nullInt(bandEnd),
			AgeRestriction: nullStr(ageRestriction),
			Mr:             nullInt(mr),
			Mr2:            nullInt(mr2),
			Analysed:       analysed,
			PreDone:        preDone,
			MainComment:    nullStr(mainComment),
			Amended:        amended,
		}
		err := pgDB.NewInsert().Model(&race).
			On("CONFLICT (course_id, date, time) DO UPDATE").
			Set("url = EXCLUDED.url").
			Set("class = COALESCE(EXCLUDED.class, rc.class)").
			Set("distance = EXCLUDED.distance").
			Set("going = COALESCE(NULLIF(EXCLUDED.going, ''), rc.going)").
			Set("band_start = COALESCE(EXCLUDED.band_start, rc.band_start)").
			Set("band_end = COALESCE(EXCLUDED.band_end, rc.band_end)").
			Set("age_restriction = COALESCE(EXCLUDED.age_restriction, rc.age_restriction)").
			Set("mr = COALESCE(EXCLUDED.mr, rc.mr)").
			Set("mr2 = COALESCE(EXCLUDED.mr2, rc.mr2)").
			Set("analysed = rc.analysed OR EXCLUDED.analysed").
			Set("pre_done = rc.pre_done OR EXCLUDED.pre_done").
			Set("main_comment = COALESCE(EXCLUDED.main_comment, rc.main_comment)").
			Set("amended = rc.amended OR EXCLUDED.amended").
			Returning("race_id").Scan(ctx)
		if err != nil {
			return raceIDs, total, err
		}
		raceIDs[sourceRaceID] = race.RaceID
		total++
	}
	return raceIDs, total, rows.Err()
}

// migratePreRace copies normalized source rows when available and otherwise
// expands the legacy pre_race JSON arrays.
func migratePreRace(ctx context.Context, sourceDB *sql.DB, pgDB *bun.DB, raceIDs map[int]int) (int, error) {
	hasNormalized, err := sourceTableExists(ctx, sourceDB, "pre_race_runners")
	if err != nil {
		return 0, err
	}
	if hasNormalized {
		return migrateNormalizedPreRace(ctx, sourceDB, pgDB, raceIDs)
	}
	rows, err := sourceDB.QueryContext(ctx, `SELECT race_id, runners FROM pre_race`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var sourceRaceID int
		var runners []byte
		if err := rows.Scan(&sourceRaceID, &runners); err != nil {
			return total, err
		}
		raceID, err := mappedRaceID(sourceRaceID, raceIDs)
		if err != nil {
			return total, err
		}
		result, err := pgDB.ExecContext(ctx, `
   INSERT INTO pre_race_runners (race_id, horse_id, runner)
   SELECT ?, (runner->>'horseID')::integer, runner
   FROM jsonb_array_elements(?::jsonb) AS runner
   ON CONFLICT (race_id, horse_id) DO UPDATE SET runner = EXCLUDED.runner`, raceID, string(runners))
		if err != nil {
			return total, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return total, err
		}
		total += int(count)
	}
	return total, rows.Err()
}

func migrateNormalizedPreRace(ctx context.Context, sourceDB *sql.DB, pgDB *bun.DB, raceIDs map[int]int) (int, error) {
	rows, err := sourceDB.QueryContext(ctx,
		`SELECT id, race_id, horse_id, runner FROM pre_race_runners ORDER BY id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	batch := make([]models.PreRaceRunner, 0, batchSize)
	total := 0
	flush := func() error {
		if err := bulkInsert(ctx, pgDB, batch); err != nil {
			return err
		}
		total += len(batch)
		batch = batch[:0]
		return nil
	}
	for rows.Next() {
		var row models.PreRaceRunner
		var sourceRaceID int
		if err := rows.Scan(&row.ID, &sourceRaceID, &row.HorseID, &row.Runner); err != nil {
			return total, err
		}
		raceID, err := mappedRaceID(sourceRaceID, raceIDs)
		if err != nil {
			return total, err
		}
		row.RaceID = raceID
		batch = append(batch, row)
		if len(batch) == batchSize {
			if err := flush(); err != nil {
				return total, err
			}
		}
	}
	if err := flush(); err != nil {
		return total, err
	}
	return total, rows.Err()
}

func migrateResults(ctx context.Context, sourceDB *sql.DB, pgDB *bun.DB, raceIDs map[int]int) (int, error) {
	rows, err := sourceDB.QueryContext(ctx,
		`SELECT id, horse_id, course_id, race_id, age, price, trainer, jockey, number,
		        headgear, placed, pace, official_rat, win_dist, dist_behind_winner,
		        weight_carried, card_weight, claim, rpr, ts,
		        mr_plus_or, mr2_plus_or, wc_mr2_plus_or, wc_mr1_plus_or, tot_rpr,
		        tfr, tfsf, tfsf_minus_or, sec_t, speed_per, comment, analysed
		 FROM results`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var batch []models.Result
	total := 0
	for rows.Next() {
		var (
			id               int
			horseID          int
			courseID         int
			raceID           int
			age              int
			price            string
			trainer          string
			jockey           string
			number           int
			headgear         sql.NullString
			placed           string
			pace             sql.NullString
			officialRat      sql.NullInt64
			winDist          sql.NullFloat64
			distBehindWinner sql.NullFloat64
			weightCarried    int
			cardWeight       int
			claim            sql.NullInt64
			rpr              sql.NullInt64
			ts               sql.NullInt64
			mrPlusOr         sql.NullInt64
			mr2PlusOr        sql.NullInt64
			wCmr2PlusOr      sql.NullInt64
			wCmr1PlusOr      sql.NullInt64
			totRPR           sql.NullInt64
			tfr              sql.NullString
			tfsf             sql.NullInt64
			tfsfMinusOr      sql.NullInt64
			secT             sql.NullFloat64
			speedPer         sql.NullFloat64
			comment          sql.NullString
			analysed         bool
		)
		if err := rows.Scan(
			&id, &horseID, &courseID, &raceID, &age, &price, &trainer, &jockey, &number,
			&headgear, &placed, &pace, &officialRat, &winDist, &distBehindWinner,
			&weightCarried, &cardWeight, &claim, &rpr, &ts,
			&mrPlusOr, &mr2PlusOr, &wCmr2PlusOr, &wCmr1PlusOr, &totRPR,
			&tfr, &tfsf, &tfsfMinusOr, &secT, &speedPer, &comment, &analysed,
		); err != nil {
			return total, err
		}
		targetRaceID, err := mappedRaceID(raceID, raceIDs)
		if err != nil {
			return total, err
		}
		batch = append(batch, models.Result{
			ID:               id,
			HorseID:          horseID,
			CourseID:         courseID,
			RaceID:           targetRaceID,
			Age:              age,
			Price:            price,
			Trainer:          trainer,
			Jockey:           jockey,
			Number:           number,
			Headgear:         nullStr(headgear),
			Placed:           placed,
			Pace:             nullStr(pace),
			OfficialRat:      nullInt(officialRat),
			WinDist:          nullFloat(winDist),
			DistBehindWinner: nullFloat(distBehindWinner),
			WeightCarried:    weightCarried,
			CardWeight:       cardWeight,
			Claim:            nullInt(claim),
			RPR:              nullInt(rpr),
			TS:               nullInt(ts),
			MrPlusOr:         nullInt(mrPlusOr),
			Mr2PlusOr:        nullInt(mr2PlusOr),
			WCmr2PlusOr:      nullInt(wCmr2PlusOr),
			WCmr1PlusOr:      nullInt(wCmr1PlusOr),
			TotRPR:           nullInt(totRPR),
			Tfr:              nullStr(tfr),
			Tfsf:             nullInt(tfsf),
			TfsfMinusOr:      nullInt(tfsfMinusOr),
			SecT:             nullFloat(secT),
			SpeedPer:         nullFloat(speedPer),
			Comment:          nullStr(comment),
			Analysed:         analysed,
		})
		if len(batch) >= batchSize {
			if err := bulkInsert(ctx, pgDB, batch); err != nil {
				return total, err
			}
			total += len(batch)
			batch = batch[:0]
		}
	}
	if err := bulkInsert(ctx, pgDB, batch); err != nil {
		return total, err
	}
	return total + len(batch), rows.Err()
}

func migrateIntermediary(ctx context.Context, sourceDB *sql.DB, pgDB *bun.DB, raceIDs map[int]int) (int, error) {
	rows, err := sourceDB.QueryContext(ctx,
		"SELECT id, horse_id, race_id, mr_plus_or, tfr FROM intermediary")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var batch []models.Intermediary
	total := 0
	for rows.Next() {
		var (
			id       int
			horseID  int
			raceID   int
			mrPlusOr sql.NullInt64
			tfr      sql.NullString
		)
		if err := rows.Scan(&id, &horseID, &raceID, &mrPlusOr, &tfr); err != nil {
			return total, err
		}
		targetRaceID, err := mappedRaceID(raceID, raceIDs)
		if err != nil {
			return total, err
		}
		batch = append(batch, models.Intermediary{
			ID:       id,
			HorseID:  horseID,
			RaceID:   targetRaceID,
			MrPlusOr: nullInt(mrPlusOr),
			Tfr:      nullStr(tfr),
		})
		if len(batch) >= batchSize {
			if err := bulkInsert(ctx, pgDB, batch); err != nil {
				return total, err
			}
			total += len(batch)
			batch = batch[:0]
		}
	}
	if err := bulkInsert(ctx, pgDB, batch); err != nil {
		return total, err
	}
	return total + len(batch), rows.Err()
}

// resetSequences advances each PG sequence to MAX(id) so new inserts don't conflict.
func resetSequences(ctx context.Context, pgDB *bun.DB) {
	seqs := []struct{ seq, table, col string }{
		{"users_id_seq", "users", "id"},
		{"courses_course_id_seq", "courses", "course_id"},
		{"horses_horse_id_seq", "horses", "horse_id"},
		{"trainers_trainer_id_seq", "trainers", "trainer_id"},
		{"races_race_id_seq", "races", "race_id"},
		{"pre_race_runners_id_seq", "pre_race_runners", "id"},
		{"results_id_seq", "results", "id"},
		{"intermediary_id_seq", "intermediary", "id"},
	}
	for _, s := range seqs {
		q := fmt.Sprintf(
			"SELECT setval('%s', COALESCE((SELECT MAX(%s) FROM %s), 1))",
			s.seq, s.col, s.table,
		)
		if _, err := pgDB.ExecContext(ctx, q); err != nil {
			log.Printf("reset seq %s: %v", s.seq, err)
		}
	}
	log.Println("sequences reset")
}

func isInsufficientPrivilege(err error) bool {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		// SQLSTATE 42501: insufficient_privilege
		return pgErr.Field('C') == "42501"
	}
	return strings.Contains(strings.ToLower(err.Error()), "permission denied")
}

func missingTables(ctx context.Context, pgDB *bun.DB) ([]string, error) {
	required := []string{
		"users",
		"courses",
		"horses",
		"trainers",
		"races",
		"pre_race_runners",
		"results",
		"intermediary",
	}

	missing := make([]string, 0, len(required))
	for _, table := range required {
		var reg sql.NullString
		if err := pgDB.NewRaw(`SELECT to_regclass(?)`, "public."+table).Scan(ctx, &reg); err != nil {
			return nil, err
		}
		if !reg.Valid || reg.String == "" {
			missing = append(missing, table)
		}
	}
	return missing, nil
}
