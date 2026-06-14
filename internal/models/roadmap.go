package models

import "time"

// Roadmap represents one public platform update timeline entry.
type Roadmap struct {
	ID        int       `db:"id"`
	Date      time.Time `db:"date"`
	Title     string    `db:"title"`
	Content   string    `db:"content"`
	Link      *string   `db:"link"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}
