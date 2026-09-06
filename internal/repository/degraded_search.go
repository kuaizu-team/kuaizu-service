package repository

import (
	"fmt"
	"strings"
	"unicode"
)

type degradedSearchSQL struct {
	Predicate     string
	PredicateArgs []interface{}
	Score         string
	ScoreArgs     []interface{}
}

func buildDegradedSearchSQL(documentExpr, keyword string) degradedSearchSQL {
	return buildDegradedSearchSQLWithMatcher(keyword, func(pattern string) (string, []interface{}) {
		return fmt.Sprintf("%s LIKE ? ESCAPE '!'", documentExpr), []interface{}{pattern}
	})
}

func buildDegradedSearchSQLWithMatcher(keyword string, matcher func(string) (string, []interface{})) degradedSearchSQL {
	keyword = strings.TrimSpace(keyword)
	characters := uniqueSearchFragments(keyword, 1)
	pairs := uniqueSearchFragments(keyword, 2)

	characterChecks, characterArgs := matchChecks(matcher, characters)
	fullPattern := talentKeywordLikePattern(keyword)
	fullCheck, fullArgs := matcher(fullPattern)

	pairCheck := "0 = 1"
	pairArgs := []interface{}{}
	if len(pairs) > 0 {
		pairChecks, args := matchChecks(matcher, pairs)
		pairCheck = "(" + strings.Join(pairChecks, " OR ") + ")"
		pairArgs = args
	}

	matchCountParts := make([]string, len(characterChecks))
	for i, check := range characterChecks {
		matchCountParts[i] = "IF(" + check + ", 1, 0)"
	}
	matchCount := strings.Join(matchCountParts, " + ")
	if matchCount == "" {
		matchCount = "0"
	}

	score := fmt.Sprintf(`CASE
		WHEN %s THEN 4
		WHEN %s THEN 3
		WHEN (%s) >= 2 THEN 2
		WHEN (%s) >= 1 THEN 1
		ELSE 0
	END`, fullCheck, pairCheck, matchCount, matchCount)
	scoreArgs := append([]interface{}{}, fullArgs...)
	scoreArgs = append(scoreArgs, pairArgs...)
	scoreArgs = append(scoreArgs, characterArgs...)
	scoreArgs = append(scoreArgs, characterArgs...)

	return degradedSearchSQL{
		Predicate:     "(" + strings.Join(characterChecks, " OR ") + ")",
		PredicateArgs: characterArgs,
		Score:         score,
		ScoreArgs:     scoreArgs,
	}
}

func uniqueSearchFragments(keyword string, width int) []string {
	runes := []rune(keyword)
	seen := map[string]struct{}{}
	fragments := make([]string, 0, len(runes))
	for i := 0; i+width <= len(runes); i++ {
		fragmentRunes := runes[i : i+width]
		skip := false
		for _, current := range fragmentRunes {
			if unicode.IsSpace(current) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		fragment := string(fragmentRunes)
		if _, exists := seen[fragment]; exists {
			continue
		}
		seen[fragment] = struct{}{}
		fragments = append(fragments, fragment)
	}
	return fragments
}

func matchChecks(matcher func(string) (string, []interface{}), fragments []string) ([]string, []interface{}) {
	checks := make([]string, 0, len(fragments))
	args := make([]interface{}, 0, len(fragments))
	for _, fragment := range fragments {
		check, checkArgs := matcher(talentKeywordLikePattern(fragment))
		checks = append(checks, check)
		args = append(args, checkArgs...)
	}
	return checks, args
}
