package db

import (
	"context"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// New creates a new MySQL database connection
func New(ctx context.Context) (*sqlx.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	db, err := sqlx.ConnectContext(ctx, "mysql", normalizeMySQLDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func normalizeMySQLDSN(dsn string) string {
	if !strings.Contains(dsn, "?") {
		return dsn + "?parseTime=true&loc=Asia%2FShanghai"
	}
	params := dsn[strings.Index(dsn, "?")+1:]
	if !strings.Contains(params, "parseTime=") {
		dsn += "&parseTime=true"
	}
	if !strings.Contains(params, "loc=") {
		dsn += "&loc=Asia%2FShanghai"
	}
	return dsn
}
