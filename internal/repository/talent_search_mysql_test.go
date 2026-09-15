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
	repo := NewTalentProfileRepository(db)
	for _, keyword := range []string{"", "计", "计算机", "大学", "软件工程", "设计"} {
		t.Run(keyword, func(t *testing.T) {
			profiles, total, err := repo.List(ctx, TalentProfileListParams{Page: 1, Size: 10, Keyword: &keyword})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("rows=%d total=%d", len(profiles), total)
		})
	}
}
