package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	appdb "github.com/kuaizu-team/kuaizu-service/internal/db"
)

// Opt-in, read-only regression against the configured MySQL database.
func TestTalentSearchMySQL(t *testing.T) {
	if os.Getenv("KUAIZU_SEARCH_MYSQL_TEST") != "1" {
		t.Skip("set KUAIZU_SEARCH_MYSQL_TEST=1 and DATABASE_URL to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		config, err := godotenv.Read("../../.env")
		if err != nil {
			t.Fatal("cannot load database configuration")
		}
		t.Setenv("DATABASE_URL", config["DATABASE_URL"])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := appdb.New(ctx)
	if err != nil {
		var sqlErr *mysql.MySQLError
		if errors.As(err, &sqlErr) {
			t.Fatalf("MySQL connection error %d", sqlErr.Number)
		}
		t.Fatalf("database connection failed (%T)", errors.Unwrap(err))
	}
	defer db.Close()
	// Pin the session so SET NAMES also applies to the prepared LIKE parameters.
	db.SetMaxOpenConns(1)
	repo := NewTalentProfileRepository(db)
	sortBy := "school_priority"
	schoolID, majorID := 1, 1
	for _, charset := range []string{"utf8mb4", "binary"} {
		t.Run(charset, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, "SET NAMES "+charset); err != nil {
				t.Fatal(err)
			}
			for _, keyword := range []string{"", "123", "计", "计算机", "大学", "软件工程", "设计", "%_!"} {
				t.Run(keyword, func(t *testing.T) {
					for _, params := range []TalentProfileListParams{
						{Page: 1, Size: 10, Keyword: &keyword},
						{Page: 1, Size: 1, Keyword: &keyword, SortBy: &sortBy, UserSchoolID: &schoolID, UserMajorID: &majorID, RandomSeed: "search-regression"},
						{Page: 2, Size: 1, Keyword: &keyword, SortBy: &sortBy, SchoolID: &schoolID, MajorID: &majorID, RandomSeed: "search-regression"},
					} {
						profiles, total, err := repo.List(ctx, params)
						if err != nil {
							t.Fatal(err)
						}
						if len(profiles) > params.Size || int64(len(profiles)) > total {
							t.Fatalf("invalid pagination: rows=%d total=%d", len(profiles), total)
						}
					}
				})
			}
		})
	}
	// The optional fixture check is only for the isolated synthetic database.
	if os.Getenv("KUAIZU_SEARCH_FIXTURE_TEST") == "1" {
		keyword := "甲乙丙"
		profiles, total, err := repo.List(ctx, TalentProfileListParams{Page: 1, Size: 10, Keyword: &keyword, SortBy: &sortBy, RandomSeed: "fixture"})
		if err != nil {
			t.Fatal(err)
		}
		if total != 4 || len(profiles) != 4 {
			t.Fatalf("ranking rows=%d total=%d, want 4", len(profiles), total)
		}
		for i, p := range profiles {
			if p.ID != i+1 {
				t.Fatalf("rank %d: id=%d, want %d", i, p.ID, i+1)
			}
		}
		for _, keyword := range []string{"计算机", "测试大学", "设计"} {
			rows, _, err := repo.List(ctx, TalentProfileListParams{Page: 1, Size: 10, Keyword: &keyword})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range rows {
				found = found || row.ID == 1
			}
			if !found {
				t.Fatalf("expected fixture profile for %q", keyword)
			}
		}
	}
}
