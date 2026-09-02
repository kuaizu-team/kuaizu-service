package repository

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
)

type selectedProjectEvent struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

type projectEventFilterSQL struct {
	Predicate             string
	PredicateArgs         []interface{}
	Category              string
	CategoryArgs          []interface{}
	RelationCount         string
	RelationCountArgs     []interface{}
	MatchedEventCount     string
	MatchedEventArgs      []interface{}
	MatchedCharacterCount string
	MatchedCharacterArgs  []interface{}
}

func (r *ProjectRepository) listSelectedEvents(ctx context.Context, eventIDs []int) ([]selectedProjectEvent, error) {
	uniqueIDs := uniquePositiveInts(eventIDs)
	query, args, err := sqlx.In("SELECT id, name FROM event WHERE id IN (?)", uniqueIDs)
	if err != nil {
		return nil, fmt.Errorf("build selected event query: %w", err)
	}
	var rows []selectedProjectEvent
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("query selected events: %w", err)
	}
	byID := make(map[int]selectedProjectEvent, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Name) != "" {
			byID[row.ID] = row
		}
	}
	result := make([]selectedProjectEvent, 0, len(byID))
	for _, id := range uniqueIDs {
		if event, ok := byID[id]; ok {
			result = append(result, event)
		}
	}
	return result, nil
}

func uniquePositiveInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func buildProjectEventFilterSQL(events []selectedProjectEvent) projectEventFilterSQL {
	ids := make([]int, 0, len(events))
	names := make([]string, 0, len(events))
	for _, event := range events {
		name := strings.TrimSpace(event.Name)
		if event.ID > 0 && name != "" {
			ids = append(ids, event.ID)
			names = append(names, name)
		}
	}

	relationExists, relationExistsArgs := selectedEventRelationSQL("EXISTS", ids)
	relationCount, relationCountArgs := selectedEventRelationSQL("COUNT", ids)
	characters := uniqueSearchFragments(strings.Join(names, " "), 1)
	pairs := uniqueEventNameFragments(names, 2)

	characterCheck, characterArgs := projectEventPatternChecks(characters)
	pairCheck, pairArgs := projectEventPatternChecks(pairs)
	fullCheck, fullArgs := projectEventPatternChecks(names)

	matchedEventParts := make([]string, 0, len(names))
	matchedEventArgs := make([]interface{}, 0)
	for _, name := range names {
		check, args := projectEventPatternChecks(uniqueSearchFragments(name, 1))
		matchedEventParts = append(matchedEventParts, "IF("+check+", 1, 0)")
		matchedEventArgs = append(matchedEventArgs, args...)
	}
	matchedCharacterParts := make([]string, 0, len(characters))
	matchedCharacterArgs := make([]interface{}, 0)
	for _, character := range characters {
		check, args := projectEventPatternChecks([]string{character})
		matchedCharacterParts = append(matchedCharacterParts, "IF("+check+", 1, 0)")
		matchedCharacterArgs = append(matchedCharacterArgs, args...)
	}

	category := fmt.Sprintf(`CASE
		WHEN %s THEN 4
		WHEN %s THEN 3
		WHEN %s THEN 2
		WHEN %s THEN 1
		ELSE 0
	END`, relationExists, fullCheck, pairCheck, characterCheck)
	categoryArgs := append([]interface{}{}, relationExistsArgs...)
	categoryArgs = append(categoryArgs, fullArgs...)
	categoryArgs = append(categoryArgs, pairArgs...)
	categoryArgs = append(categoryArgs, characterArgs...)

	predicateArgs := append([]interface{}{}, relationExistsArgs...)
	predicateArgs = append(predicateArgs, characterArgs...)
	return projectEventFilterSQL{
		Predicate:             "(" + relationExists + " OR " + characterCheck + ")",
		PredicateArgs:         predicateArgs,
		Category:              category,
		CategoryArgs:          categoryArgs,
		RelationCount:         relationCount,
		RelationCountArgs:     relationCountArgs,
		MatchedEventCount:     strings.Join(matchedEventParts, " + "),
		MatchedEventArgs:      matchedEventArgs,
		MatchedCharacterCount: strings.Join(matchedCharacterParts, " + "),
		MatchedCharacterArgs:  matchedCharacterArgs,
	}
}

func uniqueEventNameFragments(names []string, width int) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, name := range names {
		for _, fragment := range uniqueSearchFragments(name, width) {
			if _, ok := seen[fragment]; ok {
				continue
			}
			seen[fragment] = struct{}{}
			result = append(result, fragment)
		}
	}
	return result
}

func selectedEventRelationSQL(mode string, ids []int) (string, []interface{}) {
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := strings.Join(placeholders, ",")
	if mode == "COUNT" {
		return "(SELECT COUNT(DISTINCT selected_pe.event_id) FROM project_event selected_pe WHERE selected_pe.project_id = p.id AND selected_pe.event_id IN (" + inClause + "))", args
	}
	return "EXISTS (SELECT 1 FROM project_event selected_pe WHERE selected_pe.project_id = p.id AND selected_pe.event_id IN (" + inClause + "))", args
}

func projectEventPatternChecks(fragments []string) (string, []interface{}) {
	patterns := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		if fragment != "" {
			patterns = append(patterns, regexp.QuoteMeta(fragment))
		}
	}
	if len(patterns) == 0 {
		return "0 = 1", nil
	}
	return projectEventSearchMatcher(strings.Join(patterns, "|"))
}

func projectEventSearchMatcher(pattern string) (string, []interface{}) {
	const clause = `(p.name REGEXP ?
		OR p.description REGEXP ?
		OR EXISTS (SELECT 1 FROM project_tag_relation event_search_ptr
			JOIN project_tag event_search_tag ON event_search_tag.id = event_search_ptr.tag_id
			WHERE event_search_ptr.project_id = p.id AND event_search_tag.status = 1
				AND event_search_tag.name REGEXP ?)
		OR EXISTS (SELECT 1 FROM project_milestones event_search_milestone
			WHERE event_search_milestone.project_id = p.id AND (
				event_search_milestone.title REGEXP ?
				OR event_search_milestone.description REGEXP ?
				OR event_search_milestone.detail_description REGEXP ?))
		OR EXISTS (SELECT 1 FROM project_event event_search_pe
			JOIN event event_search_event ON event_search_event.id = event_search_pe.event_id
			WHERE event_search_pe.project_id = p.id AND event_search_event.name REGEXP ?)
		OR EXISTS (SELECT 1 FROM project_event event_timeline_pe
			JOIN event_timeline_node event_search_node ON event_search_node.event_id = event_timeline_pe.event_id
			WHERE event_timeline_pe.project_id = p.id AND (
				event_search_node.title REGEXP ?
				OR event_search_node.description REGEXP ?)))`
	return clause, []interface{}{pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern}
}

func (filter projectEventFilterSQL) OrderBy() string {
	return filter.Category + " DESC, " + filter.RelationCount + " DESC, " + filter.MatchedEventCount + " DESC, " + filter.MatchedCharacterCount + " DESC"
}

func (filter projectEventFilterSQL) OrderArgs() []interface{} {
	args := append([]interface{}{}, filter.CategoryArgs...)
	args = append(args, filter.RelationCountArgs...)
	args = append(args, filter.MatchedEventArgs...)
	args = append(args, filter.MatchedCharacterArgs...)
	return args
}

func (filter projectEventFilterSQL) SelectScores() string {
	return filter.Category + " AS event_category, " + filter.RelationCount + " AS event_relations, " + filter.MatchedEventCount + " AS event_matches, " + filter.MatchedCharacterCount + " AS event_characters"
}

func (filter projectEventFilterSQL) SelectArgs() []interface{} {
	return filter.OrderArgs()
}
