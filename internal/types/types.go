package types

import "time"

type Todo struct {
	ID               int64      `json:"id"`
	Title            string     `json:"title"`
	Content          string     `json:"content,omitempty"`
	Status           string     `json:"status"`
	Priority         string     `json:"priority,omitempty"`
	Deadline         *time.Time `json:"deadline,omitempty"`
	RelatedEntities  []string   `json:"related_entities,omitempty"`
	RelatedSpecs     []string   `json:"related_specs,omitempty"`
	InvestigationID  *int64     `json:"investigation_id,omitempty"`
	SessionID        *int64     `json:"session_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

type Decision struct {
	ID                     int64     `json:"id"`
	Title                  string    `json:"title"`
	Choice                 string    `json:"choice"`
	Why                    string    `json:"why,omitempty"`
	Alternatives           []string  `json:"alternatives,omitempty"`
	Category               string    `json:"category,omitempty"`
	RelatedEntities        []string  `json:"related_entities,omitempty"`
	RelatedSpecs           []string  `json:"related_specs,omitempty"`
	InvalidatesDecisionID  *int64    `json:"invalidates_decision_id,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
}

type Investigation struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Hypothesis      string     `json:"hypothesis"`
	Status          string     `json:"status"`
	Conclusion      string     `json:"conclusion,omitempty"`
	Evidence        []string   `json:"evidence,omitempty"`
	RelatedEntities []string   `json:"related_entities,omitempty"`
	RelatedSpecs    []string   `json:"related_specs,omitempty"`
	OpenedAt        time.Time  `json:"opened_at"`
	ClosedAt        *time.Time `json:"closed_at,omitempty"`
}

type Session struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title,omitempty"`
	Summary         string    `json:"summary"`
	SummaryPath     string    `json:"summary_path,omitempty"`
	DecisionsMade   []int64   `json:"decisions_made,omitempty"`
	TodosOpened     []int64   `json:"todos_opened,omitempty"`
	TodosClosed     []int64   `json:"todos_closed,omitempty"`
	RelatedEntities []string  `json:"related_entities,omitempty"`
	RelatedSpecs    []string  `json:"related_specs,omitempty"`
	Source          string    `json:"source,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type HistoryEntry struct {
	ID        int64     `json:"id"`
	RefID     int64     `json:"ref_id"`
	Field     string    `json:"field"`
	OldValue  string    `json:"old_value,omitempty"`
	NewValue  string    `json:"new_value,omitempty"`
	ChangedAt time.Time `json:"changed_at"`
	Source    string    `json:"source,omitempty"`
}

type SearchResult struct {
	RefType     string  `json:"ref_type"`
	RefID       int64   `json:"ref_id"`
	NameOrTitle string  `json:"name_or_title"`
	Snippet     string  `json:"snippet"`
	Score       float64 `json:"score"`
}
