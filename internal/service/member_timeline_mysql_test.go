package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
)

// Opt-in against a disposable local MySQL server, never the application's DSN.
// Creates and drops only a uniquely named test database.
func TestMemberTimelineMySQL(t *testing.T) {
	dsn := os.Getenv("KUAIZU_TIMELINE_TEST_DSN")
	if dsn == "" {
		t.Skip("set KUAIZU_TIMELINE_TEST_DSN to a disposable local MySQL server")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "tcp", cfg.Net)
	host, _, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", host)
	require.Empty(t, cfg.DBName, "use a server DSN without an existing database")
	admin, err := sqlx.Connect("mysql", dsn)
	require.NoError(t, err)
	defer admin.Close()
	name := fmt.Sprintf("kuaizu_timeline_test_%d", time.Now().UnixNano())
	_, err = admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4")
	require.NoError(t, err)
	defer func() { _, err := admin.Exec("DROP DATABASE " + name); require.NoError(t, err) }()
	cfg.DBName, cfg.ParseTime = name, true
	db, err := sqlx.Connect("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	defer db.Close()
	for _, query := range []string{
		"CREATE TABLE user (id BIGINT UNSIGNED PRIMARY KEY, nickname VARCHAR(60)) ENGINE=InnoDB",
		"CREATE TABLE project_members (id BIGINT UNSIGNED PRIMARY KEY, project_id BIGINT UNSIGNED, user_id BIGINT UNSIGNED, UNIQUE KEY (project_id,user_id)) ENGINE=InnoDB",
		"INSERT INTO user VALUES (7,'A'),(8,'B')",
		"INSERT INTO project_members VALUES (1,42,7),(2,42,8)",
	} {
		_, err = db.Exec(query)
		require.NoError(t, err)
	}
	migration, err := os.ReadFile("../../sql/migration_project_member_timeline.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Run("cross member locks coexist and still prevent removal", func(t *testing.T) {
		a, err := db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer a.Rollback()
		b, err := db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer b.Rollback()
		_, err = a.ExecContext(ctx, "SET SESSION innodb_lock_wait_timeout=1")
		require.NoError(t, err)
		require.NoError(t, lockTimelineMember(ctx, a, 42, 7))
		require.NoError(t, lockTimelineMember(ctx, b, 42, 8))
		// Deterministically recreate the former cross-lock dependency.
		require.NoError(t, lockTimelineMember(ctx, a, 42, 8))
		require.NoError(t, lockTimelineMember(ctx, b, 42, 7))
		removal, err := db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer removal.Rollback()
		_, err = removal.ExecContext(ctx, "SET SESSION innodb_lock_wait_timeout=1")
		require.NoError(t, err)
		_, err = removal.ExecContext(ctx, "DELETE FROM project_members WHERE project_id=42 AND user_id=7")
		var sqlErr *mysql.MySQLError
		require.ErrorAs(t, err, &sqlErr)
		require.EqualValues(t, 1205, sqlErr.Number)
		require.NoError(t, a.Commit())
		require.NoError(t, b.Commit())
		_, err = removal.ExecContext(ctx, "DELETE FROM project_members WHERE project_id=42 AND user_id=7")
		require.NoError(t, err)
		// Roll back the removal to retain both members for the service test.
	})

	t.Run("members concurrently create and edit mutually linked nodes", func(t *testing.T) {
		svc := &ProjectService{repo: repository.New(db)}
		ids := [2]int{}
		for round := 0; round < 2; round++ {
			start := make(chan struct{})
			errors := make(chan error, 2)
			for i := 0; i < 2; i++ {
				go func(i int) {
					<-start
					id, err := svc.SaveMemberTimeline(ctx, 42, 7+i, ids[i], MemberTimelineInput{
						Title: "协作进度", RelatedUserIDs: []int{8 - i},
					})
					ids[i] = id
					errors <- err
				}(i)
			}
			close(start)
			results := make([]error, 2)
			for i := 0; i < 2; i++ {
				results[i] = <-errors
			}
			for _, err := range results {
				require.NoError(t, err)
			}
		}
		nodes, err := svc.ListMemberTimeline(ctx, 42, 7, 0)
		require.NoError(t, err)
		require.Len(t, nodes, 2)
		for _, node := range nodes {
			require.Len(t, node.RelatedMembers, 1)
			require.Equal(t, 15-node.UserID, node.RelatedMembers[0].UserID)
		}
	})
}
