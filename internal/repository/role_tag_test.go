package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestRoleTagLikePatternEscapesWildcards(t *testing.T) {
	tests := []struct {
		name    string
		keyword string
		prefix  bool
		want    string
	}{
		{name: "prefix", keyword: "UI_100%", prefix: true, want: "UI!_100!%%"},
		{name: "substring", keyword: "设计", prefix: false, want: "%设计%"},
		{name: "escape marker", keyword: "A!B", prefix: false, want: "%A!!B%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roleTagLikePattern(tt.keyword, tt.prefix); got != tt.want {
				t.Fatalf("roleTagLikePattern(%q, %v) = %q, want %q", tt.keyword, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestSearchRoleTagsUsesRoleFilterAndPrefixFirst(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()
	repo := NewRoleTagRepository(sqlx.NewDb(db, "sqlmock"))

	mock.ExpectQuery("SELECT id, tag_text, emoji, display_text").
		WithArgs("TECH_LEADER", "架构%", "TECH_LEADER", "%架构%", "架构%", 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tag_text", "emoji", "display_text"}).
			AddRow(1, "架构设计", "🏛️", "🏛️ 架构设计"))

	tags, err := repo.Search(context.Background(), "架构", "TECH_LEADER", 10)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(tags) != 1 || tags[0].Display != "🏛️ 架构设计" {
		t.Fatalf("unexpected tags: %#v", tags)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
