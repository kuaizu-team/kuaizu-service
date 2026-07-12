//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	db, err := sql.Open("mysql", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	defer db.Close()

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'event' AND COLUMN_NAME IN ('level', 'summary', 'school_id')`).Scan(&count)
	if err != nil {
		panic(err)
	}
	if count == 3 {
		fmt.Println("event fields already exist")
		return
	}
	if count != 0 {
		panic("event table has a partial level/summary/school_id migration")
	}
	_, err = db.Exec(`ALTER TABLE event
		ADD COLUMN level VARCHAR(20) NULL COMMENT '赛事等级: national/regional/school' AFTER article_url,
		ADD COLUMN summary VARCHAR(255) NULL COMMENT '赛事一句话描述' AFTER level,
		ADD COLUMN school_id INT NULL COMMENT '学校级赛事所属学校ID' AFTER summary,
		ADD INDEX idx_event_level (level),
		ADD INDEX idx_event_school_id (school_id)`)
	if err != nil {
		panic(err)
	}
	fmt.Println("event fields migration applied")
}
