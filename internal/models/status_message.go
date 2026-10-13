package models

import "time"

// StatusMessage is an immutable archive, independent of the delivery queue.
type StatusMessage struct {
	ID                    int64                  `db:"id" json:"id"`
	Type                  string                 `db:"type" json:"type"`
	Payload               *string                `db:"payload" json:"-"`
	Params                map[string]interface{} `db:"-" json:"params"`
	ReadAt                *time.Time             `db:"read_at" json:"readAt"`
	FullscreenDisplayed   *bool                  `db:"fullscreen_displayed" json:"fullscreenDisplayed"`
	FullscreenDisplayedAt *time.Time             `db:"fullscreen_displayed_at" json:"fullscreenDisplayedAt"`
	CreatedAt             time.Time              `db:"created_at" json:"createdAt"`
}
