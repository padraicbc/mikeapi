// cmd/migrate/main.go
// Migrates data from a remote MySQL rpData database into the local PostgreSQL database.
//
// Usage:
//
//	MYSQL_DSN="user:pass@tcp(host:3306)/rpData?parseTime=true" \
//	RPPASS="pgpass" \
//	go run ./cmd/migrate
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/uptrace/bun"

	"github.com/padraicbc/mikeapi/config"
	bundb "github.com/padraicbc/mikeapi/db"
	"github.com/padraicbc/mikeapi/models"
)

const batchSize = 500

func main() {
	ctx := context.Background()

	cfg := config.Load()

	// --- MySQL ---
	if cfg.MySQLDSN == "" {
		log.Fatal("MYSQL_DSN required, e.g.: user:pass@tcp(host:3306)/rpData?parseTime=true")
	}
	myDB, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer myDB.Close()
	myDB.SetMaxOpenConns(4)
	if err := myDB.PingContext(ctx); err != nil {
		log.Fatalf("ping mysql: %v", err)
	}
	log.Println("connected to MySQL")

	// --- PostgreSQL ---
	pgDB := bundb.Setup(cfg)
	defer pgDB.Close()
	log.Println("connected to PostgreSQL")

	// Create tables (idempotent)
	if err := bundb.CreateTables(ctx, pgDB); err != nil {
		log.Fatalf("create tables: %v", err)
	}

	// Disable FK enforcement so we can load in bulk without strict ordering
	if _, err := pgDB.ExecContext(ctx, "SET session_replication_role = 'replica'"); err != nil {
		log.Fatalf("disable FK: %v", err)
	}
	defer func() {
		if _, err := pgDB.ExecContext(ctx, "SET session_replication_role = 'origin'"); err != nil {
			log.Printf("re-enable FK: %v", err)
		}
	}()

	steps := []struct {
		name string
		fn   func() (int, error)
	}{
		{"users", func() (int, error) { return migrateUsers(ctx, myDB, pgDB) }},
		{"courses", func() (int, error) { return migrateCourses(ctx, myDB, pgDB) }},
		{"horses", func() (int, error) { return migrateHorses(ctx, myDB, pgDB) }},
		{"trainers", func() (int, error) { return migrateTrainers(ctx, myDB, pgDB) }},
		{"races", func() (int, error) { return migrateRaces(ctx, myDB, pgDB) }},
		{"pre_race", func() (int, error) { return migratePreRace(ctx, myDB, pgDB) }},
		{"results", func() (int, error) { return migrateResults(ctx, myDB, pgDB) }},
		{"intermediary", func() (int, error) { return migrateIntermediary(ctx, myDB, pgDB) }},
	}

	for _, s := range steps {
		n, err := s.fn()
		if err != nil {
			log.Fatalf("migrate %s: %v", s.name, err)
		}
		log.Printf("%-15s  %d rows migrated", s.name, n)
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

// --- per-table migrations ---

func migrateUsers(ctx context.Context, myDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := myDB.QueryContext(ctx, "SELECT id, username, password FROM users")
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

func migrateCourses(ctx context.Context, myDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := myDB.QueryContext(ctx,
		"SELECT courseID, course, direction, isAw, code FROM courses")
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

func migrateHorses(ctx context.Context, myDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := myDB.QueryContext(ctx,
		`SELECT horseID, horse, lastWinID, highestWinWeight, lastWinWeight,
		        lastRunWeight, lastWinClaim, lastRunClaim, highestWinOr
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
		batch = append(batch, models.Horse{
			HorseID:          horseID,
			Horse:            horse,
			LastWinID:        nullInt(lastWinID),
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

func migrateTrainers(ctx context.Context, myDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := myDB.QueryContext(ctx,
		"SELECT trainerID, trainer, info FROM trainers")
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

func migrateRaces(ctx context.Context, myDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := myDB.QueryContext(ctx,
		`SELECT raceID, courseID, date, time, url, class, distance, going,
		        mr, mr2, analysed, preDone, mainComment, amended
		 FROM races`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var batch []models.Race
	total := 0
	for rows.Next() {
		var (
			raceID      int
			courseID    int
			date        time.Time
			rtime       string
			url         string
			class       sql.NullString
			distance    float64
			going       string
			mr          sql.NullInt64
			mr2         sql.NullInt64
			analysed    bool
			preDone     bool
			mainComment sql.NullString
			amended     bool
		)
		if err := rows.Scan(&raceID, &courseID, &date, &rtime, &url, &class,
			&distance, &going, &mr, &mr2, &analysed, &preDone, &mainComment, &amended); err != nil {
			return total, err
		}
		batch = append(batch, models.Race{
			RaceID:      raceID,
			CourseID:    courseID,
			Date:        fmtDate(date),
			Time:        rtime,
			URL:         url,
			Class:       nullStr(class),
			Distance:    distance,
			Going:       going,
			Mr:          nullInt(mr),
			Mr2:         nullInt(mr2),
			Analysed:    analysed,
			PreDone:     preDone,
			MainComment: nullStr(mainComment),
			Amended:     amended,
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

func migratePreRace(ctx context.Context, myDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := myDB.QueryContext(ctx,
		`SELECT id, runners, course, courseID, date, time, raceID,
		        direction, distance, class, url
		 FROM preRace`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var batch []models.PreRace
	total := 0
	for rows.Next() {
		var (
			id        int
			runners   []byte
			course    string
			courseID  int
			date      time.Time
			rtime     string
			raceID    int
			direction string
			distance  float64
			class     string
			url       string
		)
		if err := rows.Scan(&id, &runners, &course, &courseID, &date, &rtime,
			&raceID, &direction, &distance, &class, &url); err != nil {
			return total, err
		}
		batch = append(batch, models.PreRace{
			ID:        id,
			Runners:   json.RawMessage(runners),
			Course:    course,
			CourseID:  courseID,
			Date:      fmtDate(date),
			Time:      rtime,
			RaceID:    raceID,
			Direction: direction,
			Distance:  distance,
			Class:     class,
			URL:       url,
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

func migrateResults(ctx context.Context, myDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := myDB.QueryContext(ctx,
		`SELECT id, horseID, courseID, raceID, age, price, trainer, jockey, number,
		        headgear, placed, pace, officialRat, winDist, distBehindWinner,
		        weightCarried, cardWeight, claim, rpr, ts,
		        mrPlusOr, mr2PlusOr, wCmr2PlusOr, wCmr1PlusOr, totRPR,
		        tfr, tfsf, tfsfMinusOr, secT, speedPer, comment, analysed
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
		batch = append(batch, models.Result{
			ID:               id,
			HorseID:          horseID,
			CourseID:         courseID,
			RaceID:           raceID,
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

func migrateIntermediary(ctx context.Context, myDB *sql.DB, pgDB *bun.DB) (int, error) {
	rows, err := myDB.QueryContext(ctx,
		"SELECT id, horseID, raceID, mrPlusOr, tfr FROM intermediary")
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
		batch = append(batch, models.Intermediary{
			ID:       id,
			HorseID:  horseID,
			RaceID:   raceID,
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
		{"pre_race_id_seq", "pre_race", "id"},
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
