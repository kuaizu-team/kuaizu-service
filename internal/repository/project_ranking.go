package repository

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
)

const (
	projectGeoTierCount = 5
	projectPoolMinSize  = 5
)

type projectRankCandidate struct {
	ID              int     `db:"id"`
	CreatorID       int     `db:"creator_id"`
	SchoolID        int     `db:"school_id"`
	Province        *string `db:"province"`
	City            *string `db:"city"`
	District        *string `db:"district"`
	MajorID         *int    `db:"major_id"`
	MajorClassID    *int    `db:"major_class_id"`
	LikeCount       int     `db:"like_count"`
	FavoriteCount   int     `db:"favorite_count"`
	ShareCount      int     `db:"share_count"`
	SearchScore     int     `db:"search_score"`
	EventCategory   int     `db:"event_category"`
	EventRelations  int     `db:"event_relations"`
	EventMatches    int     `db:"event_matches"`
	EventCharacters int     `db:"event_characters"`

	Tier       int    `db:"-"`
	MajorMatch int    `db:"-"`
	Heat       int    `db:"-"`
	RandomRank uint64 `db:"-"`
}

func (r *ProjectRepository) listRankedProjectIDs(ctx context.Context, whereClause string, whereArgs []interface{}, params ListParams, searchSQL *degradedSearchSQL, eventFilterSQL *projectEventFilterSQL) ([]int, error) {
	searchScoreSelect := "0 AS search_score"
	queryArgs := make([]interface{}, 0, len(whereArgs))
	if searchSQL != nil {
		searchScoreSelect = searchSQL.Score + " AS search_score"
		queryArgs = append(queryArgs, searchSQL.ScoreArgs...)
	}
	eventScoreSelect := "0 AS event_category, 0 AS event_relations, 0 AS event_matches, 0 AS event_characters"
	if eventFilterSQL != nil {
		eventScoreSelect = eventFilterSQL.SelectScores()
		queryArgs = append(queryArgs, eventFilterSQL.SelectArgs()...)
	}
	queryArgs = append(queryArgs, whereArgs...)
	query := fmt.Sprintf(`
		SELECT p.id, p.creator_id, p.school_id,
			s.province, s.city, s.district,
			u.major_id, m.class_id AS major_class_id,
			COALESCE(pl.like_count, 0) AS like_count,
			COALESCE(pf.favorite_count, 0) AS favorite_count,
			COALESCE(ps.share_count, 0) AS share_count,
			%s,
			%s
		FROM project p
		LEFT JOIN school s ON s.id = p.school_id
		LEFT JOIN `+"`user`"+` u ON u.id = p.creator_id
		LEFT JOIN major m ON m.id = u.major_id
		LEFT JOIN (SELECT project_id, COUNT(*) AS like_count FROM project_like GROUP BY project_id) pl ON pl.project_id = p.id
		LEFT JOIN (SELECT project_id, COUNT(*) AS favorite_count FROM project_favorite GROUP BY project_id) pf ON pf.project_id = p.id
		LEFT JOIN (SELECT project_id, COUNT(*) AS share_count FROM project_share GROUP BY project_id) ps ON ps.project_id = p.id
		WHERE %s`, searchScoreSelect, eventScoreSelect, whereClause)

	var candidates []projectRankCandidate
	if err := r.db.SelectContext(ctx, &candidates, query, queryArgs...); err != nil {
		return nil, fmt.Errorf("query project ranking candidates: %w", err)
	}
	ranked := rankProjectCandidates(candidates, params)
	ids := make([]int, len(ranked))
	for i := range ranked {
		ids[i] = ranked[i].ID
	}
	return ids, nil
}

func rankProjectCandidates(candidates []projectRankCandidate, params ListParams) []projectRankCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	seed := params.RandomSeed
	for i := range candidates {
		candidate := &candidates[i]
		candidate.Heat = projectCompositeFavorites(*candidate)
		candidate.Tier = projectPromotedTier(projectGeoTier(*candidate, params), candidate.Heat)
		candidate.MajorMatch = projectMajorMatch(*candidate, params)
		candidate.RandomRank = seededProjectRank(seed, candidate.ID)
	}

	pools := make([][]projectRankCandidate, projectGeoTierCount)
	for _, candidate := range candidates {
		pools[candidate.Tier-1] = append(pools[candidate.Tier-1], candidate)
	}
	for i := range pools {
		sortProjectPool(pools[i])
	}

	// A borrowed item belongs to the higher display pool for this seed only.
	for tier := 0; tier < projectGeoTierCount-1; tier++ {
		need := projectPoolMinSize - len(pools[tier])
		if need <= 0 || len(pools[tier+1]) == 0 {
			continue
		}
		if need > len(pools[tier+1]) {
			need = len(pools[tier+1])
		}
		borrowed := append([]projectRankCandidate(nil), pools[tier+1][:need]...)
		pools[tier+1] = pools[tier+1][need:]
		for i := range borrowed {
			borrowed[i].Tier = tier + 1
		}
		pools[tier] = append(pools[tier], borrowed...)
		sortProjectPool(pools[tier])
	}

	// A singleton cannot visibly shuffle by itself. Exchange it with a seeded
	// item from the next non-empty pool, accepting this small temporary tier
	// relaxation as requested.
	shuffleSingletonProjectPools(pools, seed)

	ordered := flattenProjectPools(pools)
	ordered = demoteAdjacentProjectOwners(ordered)
	ordered = avoidAdjacentProjectOwners(ordered)
	if params.Keyword != nil && strings.TrimSpace(*params.Keyword) != "" {
		sort.SliceStable(ordered, func(i, j int) bool {
			return ordered[i].SearchScore > ordered[j].SearchScore
		})
	}
	if len(params.EventIDs) > 0 {
		sort.SliceStable(ordered, func(i, j int) bool {
			left, right := ordered[i], ordered[j]
			if left.EventCategory != right.EventCategory {
				return left.EventCategory > right.EventCategory
			}
			if left.EventRelations != right.EventRelations {
				return left.EventRelations > right.EventRelations
			}
			if left.EventMatches != right.EventMatches {
				return left.EventMatches > right.EventMatches
			}
			return left.EventCharacters > right.EventCharacters
		})
	}
	return ordered
}

func shuffleSingletonProjectPools(pools [][]projectRankCandidate, seed string) {
	refreshSequence, hasRefreshSequence := projectRefreshSequence(seed)
	for tier := 0; tier < projectGeoTierCount-1; tier++ {
		if len(pools[tier]) != 1 {
			continue
		}
		for lower := tier + 1; lower < projectGeoTierCount; lower++ {
			if len(pools[lower]) == 0 {
				continue
			}
			if hasRefreshSequence && len(pools[lower]) == 1 && refreshSequence%2 == 0 {
				break
			}
			index := int(seededProjectRank(fmt.Sprintf("%s:singleton:%d", seed, tier), pools[tier][0].ID) % uint64(len(pools[lower])))
			if hasRefreshSequence && len(pools[lower]) > 1 {
				index = refreshSequence % len(pools[lower])
			}
			pools[tier][0], pools[lower][index] = pools[lower][index], pools[tier][0]
			pools[tier][0].Tier = tier + 1
			pools[lower][index].Tier = lower + 1
			break
		}
	}
}

func projectRefreshSequence(seed string) (int, bool) {
	prefix, _, ok := strings.Cut(seed, ":")
	if !ok || len(prefix) < 2 || prefix[0] != 'r' {
		return 0, false
	}
	sequence, err := strconv.Atoi(prefix[1:])
	return sequence, err == nil && sequence >= 0
}

func projectCompositeFavorites(candidate projectRankCandidate) int {
	return candidate.FavoriteCount + candidate.LikeCount/5 + candidate.ShareCount/10
}

func projectPromotedTier(baseTier, compositeFavorites int) int {
	tier := baseTier - compositeFavorites/10
	if tier < 1 {
		return 1
	}
	return tier
}

func projectGeoTier(candidate projectRankCandidate, params ListParams) int {
	if params.ViewerUserID == nil || params.UserSchoolID == nil || *params.UserSchoolID == 0 {
		return 1 // anonymous/missing-profile requests are one fully random pool
	}
	if candidate.SchoolID == *params.UserSchoolID {
		return 1
	}
	if sameString(candidate.District, params.UserSchoolDistrict) &&
		sameString(candidate.City, params.UserSchoolCity) &&
		sameString(candidate.Province, params.UserSchoolProvince) {
		return 2
	}
	if sameString(candidate.City, params.UserSchoolCity) && sameString(candidate.Province, params.UserSchoolProvince) {
		return 3
	}
	if sameString(candidate.Province, params.UserSchoolProvince) {
		return 4
	}
	return 5
}

func projectMajorMatch(candidate projectRankCandidate, params ListParams) int {
	if params.ViewerUserID == nil || params.UserMajorID == nil {
		return 0
	}
	if candidate.MajorID != nil && *candidate.MajorID == *params.UserMajorID {
		return 0
	}
	if candidate.MajorClassID != nil && params.UserMajorClassID != nil && *candidate.MajorClassID == *params.UserMajorClassID {
		return 1
	}
	return 2
}

func sameString(left, right *string) bool {
	return left != nil && right != nil && *left != "" && *left == *right
}

func seededProjectRank(seed string, id int) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s:%d", seed, id)
	return h.Sum64()
}

func sortProjectPool(pool []projectRankCandidate) {
	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].MajorMatch != pool[j].MajorMatch {
			return pool[i].MajorMatch < pool[j].MajorMatch
		}
		if pool[i].RandomRank != pool[j].RandomRank {
			return pool[i].RandomRank < pool[j].RandomRank
		}
		return pool[i].ID < pool[j].ID
	})
}

func flattenProjectPools(pools [][]projectRankCandidate) []projectRankCandidate {
	var result []projectRankCandidate
	for _, pool := range pools {
		result = append(result, pool...)
	}
	return result
}

func demoteAdjacentProjectOwners(items []projectRankCandidate) []projectRankCandidate {
	limit := len(items) * projectGeoTierCount
	for attempt := 0; attempt < limit; attempt++ {
		changed := false
		for i := 1; i < len(items); i++ {
			if items[i-1].CreatorID != items[i].CreatorID {
				continue
			}
			move := i
			delta := 1
			if items[i-1].Tier == items[i].Tier {
				delta = 2
				if items[i-1].Heat < items[i].Heat {
					move = i - 1
				}
			}
			newTier := items[move].Tier + delta
			if newTier > projectGeoTierCount {
				newTier = projectGeoTierCount
			}
			if newTier == items[move].Tier {
				continue
			}
			items[move].Tier = newTier
			sort.SliceStable(items, func(a, b int) bool {
				if items[a].Tier != items[b].Tier {
					return items[a].Tier < items[b].Tier
				}
				if items[a].MajorMatch != items[b].MajorMatch {
					return items[a].MajorMatch < items[b].MajorMatch
				}
				return items[a].RandomRank < items[b].RandomRank
			})
			changed = true
			break
		}
		if !changed {
			break
		}
	}
	return items
}

// Keep the ranked order whenever possible, but look ahead when selecting the
// next item so a prolific creator is not left as an unavoidable adjacent tail.
func avoidAdjacentProjectOwners(items []projectRankCandidate) []projectRankCandidate {
	remaining := append([]projectRankCandidate(nil), items...)
	counts := make(map[int]int, len(items))
	for _, item := range remaining {
		counts[item.CreatorID]++
	}
	result := make([]projectRankCandidate, 0, len(items))
	lastCreator := 0
	for len(remaining) > 0 {
		chosen := -1
		for i := range remaining {
			if remaining[i].CreatorID == lastCreator {
				continue
			}
			counts[remaining[i].CreatorID]--
			if projectOwnerTailFeasible(counts, len(remaining)-1, remaining[i].CreatorID) {
				chosen = i
				break
			}
			counts[remaining[i].CreatorID]++
		}
		if chosen < 0 {
			for i := range remaining {
				if remaining[i].CreatorID != lastCreator {
					chosen = i
					counts[remaining[i].CreatorID]--
					break
				}
			}
		}
		if chosen < 0 { // mathematically impossible: only the previous creator remains
			chosen = 0
			counts[remaining[0].CreatorID]--
		}
		item := remaining[chosen]
		result = append(result, item)
		lastCreator = item.CreatorID
		remaining = append(remaining[:chosen], remaining[chosen+1:]...)
	}
	return result
}

func projectOwnerTailFeasible(counts map[int]int, remaining, blockedCreator int) bool {
	for creator, count := range counts {
		limit := (remaining + 1) / 2
		if creator == blockedCreator {
			limit = remaining / 2
		}
		if count > limit {
			return false
		}
	}
	return true
}
