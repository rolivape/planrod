package search

import (
	"testing"
)

func TestRRF(t *testing.T) {
	fts := []rankedItem{
		{RefType: "todo", RefID: 1, Rank: 1},
		{RefType: "decision", RefID: 2, Rank: 2},
		{RefType: "todo", RefID: 3, Rank: 3},
	}
	vec := []rankedItem{
		{RefType: "decision", RefID: 2, Rank: 1},
		{RefType: "todo", RefID: 1, Rank: 2},
		{RefType: "investigation", RefID: 4, Rank: 3},
	}

	results := RRF(fts, vec, 0.4, 0.6, 10)

	if len(results) == 0 {
		t.Fatal("expected results")
	}

	// decision#2 appears in both lists so should score highest
	if results[0].RefType != "decision" || results[0].RefID != 2 {
		t.Errorf("expected decision#2 first, got %s#%d", results[0].RefType, results[0].RefID)
	}

	// todo#1 also appears in both
	if results[1].RefType != "todo" || results[1].RefID != 1 {
		t.Errorf("expected todo#1 second, got %s#%d", results[1].RefType, results[1].RefID)
	}

	// Verify score is positive
	for _, r := range results {
		if r.Score <= 0 {
			t.Errorf("expected positive score, got %f for %s#%d", r.Score, r.RefType, r.RefID)
		}
	}
}

func TestRRFLimit(t *testing.T) {
	fts := []rankedItem{
		{RefType: "todo", RefID: 1, Rank: 1},
		{RefType: "todo", RefID: 2, Rank: 2},
		{RefType: "todo", RefID: 3, Rank: 3},
	}

	results := RRF(fts, nil, 0.4, 0.6, 2)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}
