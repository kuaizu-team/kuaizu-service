package repository

import (
	"reflect"
	"strings"
	"testing"
)

func TestUniqueSearchFragmentsUsesUnicodeCharactersAndSkipsWhitespace(t *testing.T) {
	if got, want := uniqueSearchFragments("4 6级46", 1), []string{"4", "6", "级"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("single-character fragments = %#v, want %#v", got, want)
	}
	if got, want := uniqueSearchFragments("46级46", 2), []string{"46", "6级", "级4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("two-character fragments = %#v, want %#v", got, want)
	}
}

func TestBuildDegradedSearchSQLContainsAllFourLevels(t *testing.T) {
	search := buildDegradedSearchSQL("search_document", "46级")
	for _, want := range []string{"THEN 4", "THEN 3", "THEN 2", "THEN 1"} {
		if !strings.Contains(search.Score, want) {
			t.Fatalf("score missing %q: %s", want, search.Score)
		}
	}
	if got, want := search.PredicateArgs, []interface{}{"%4%", "%6%", "%级%"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("predicate args = %#v, want %#v", got, want)
	}
	wantPrefix := []interface{}{"%46级%", "%46%", "%6级%"}
	if !reflect.DeepEqual(search.ScoreArgs[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("score args prefix = %#v, want %#v", search.ScoreArgs[:len(wantPrefix)], wantPrefix)
	}
}
