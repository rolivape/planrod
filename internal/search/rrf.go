package search

import (
	"fmt"
	"sort"
)

const defaultK = 60

type rankedItem struct {
	RefType string
	RefID   int64
	Rank    int
}

type scoredItem struct {
	RefType string
	RefID   int64
	Score   float64
}

func RRF(ftsResults, vecResults []rankedItem, wFTS, wVec float64, limit int) []scoredItem {
	scores := make(map[string]float64)
	items := make(map[string]rankedItem)

	for _, r := range ftsResults {
		key := itemKey(r.RefType, r.RefID)
		scores[key] += wFTS * (1.0 / float64(defaultK+r.Rank))
		items[key] = r
	}
	for _, r := range vecResults {
		key := itemKey(r.RefType, r.RefID)
		scores[key] += wVec * (1.0 / float64(defaultK+r.Rank))
		if _, exists := items[key]; !exists {
			items[key] = r
		}
	}

	var results []scoredItem
	for key, score := range scores {
		item := items[key]
		results = append(results, scoredItem{
			RefType: item.RefType,
			RefID:   item.RefID,
			Score:   score,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func itemKey(refType string, refID int64) string {
	return fmt.Sprintf("%s:%d", refType, refID)
}
