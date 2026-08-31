package models

// RoleTag is a curated talent-card tag that can be recommended for one or more project roles.
type RoleTag struct {
	ID      int    `db:"id" json:"id"`
	Text    string `db:"tag_text" json:"text"`
	Emoji   string `db:"emoji" json:"emoji"`
	Display string `db:"display_text" json:"displayText"`
}
