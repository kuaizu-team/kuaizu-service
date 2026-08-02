package db

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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

	// Keep API instances from exhausting MySQL under burst traffic and recycle
	// long-lived connections before infrastructure-side idle limits do.
	db.SetMaxOpenConns(envInt("DB_MAX_OPEN_CONNS", 50))
	db.SetMaxIdleConns(envInt("DB_MAX_IDLE_CONNS", 10))
	db.SetConnMaxLifetime(time.Duration(envInt("DB_CONN_MAX_LIFETIME_MINUTES", 30)) * time.Minute)
	db.SetConnMaxIdleTime(time.Duration(envInt("DB_CONN_MAX_IDLE_TIME_MINUTES", 5)) * time.Minute)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
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
