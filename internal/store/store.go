package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rolivape/planrod/internal/types"
)

type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{DB: db}
}

// --- JSON helpers ---

func marshalJSON(v interface{}) string {
	if v == nil {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func unmarshalStrings(s string) []string {
	if s == "" || s == "null" {
		return nil
	}
	var out []string
	json.Unmarshal([]byte(s), &out)
	return out
}

func unmarshalInt64s(s string) []int64 {
	if s == "" || s == "null" {
		return nil
	}
	var out []int64
	json.Unmarshal([]byte(s), &out)
	return out
}

// --- Todos CRUD ---

func (s *Store) CreateTodo(ctx context.Context, t *types.Todo) (*types.Todo, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO todos (title, content, status, priority, deadline, related_entities, related_specs, investigation_id, session_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Title, t.Content, coalesce(t.Status, "pending"), t.Priority, t.Deadline,
		marshalJSON(t.RelatedEntities), marshalJSON(t.RelatedSpecs),
		t.InvestigationID, t.SessionID)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetTodo(ctx, id)
}

func (s *Store) GetTodo(ctx context.Context, id int64) (*types.Todo, error) {
	var t types.Todo
	var entities, specs string
	var deadline, completedAt sql.NullTime
	var invID, sessID sql.NullInt64
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, title, COALESCE(content,''), status, COALESCE(priority,''), deadline,
		        related_entities, related_specs, investigation_id, session_id,
		        created_at, updated_at, completed_at
		 FROM todos WHERE id = ?`, id).
		Scan(&t.ID, &t.Title, &t.Content, &t.Status, &t.Priority, &deadline,
			&entities, &specs, &invID, &sessID,
			&t.CreatedAt, &t.UpdatedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("todo with id %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	if deadline.Valid {
		t.Deadline = &deadline.Time
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.Time
	}
	if invID.Valid {
		t.InvestigationID = &invID.Int64
	}
	if sessID.Valid {
		t.SessionID = &sessID.Int64
	}
	t.RelatedEntities = unmarshalStrings(entities)
	t.RelatedSpecs = unmarshalStrings(specs)
	return &t, nil
}

func (s *Store) ListTodos(ctx context.Context, status, priority, relatedEntity string, investigationID int64, limit int) ([]types.Todo, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := "SELECT id, title, COALESCE(content,''), status, COALESCE(priority,''), deadline, related_entities, related_specs, investigation_id, session_id, created_at, updated_at, completed_at FROM todos WHERE 1=1"
	var args []interface{}
	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	}
	if priority != "" {
		q += " AND priority = ?"
		args = append(args, priority)
	}
	if relatedEntity != "" {
		q += " AND related_entities LIKE ?"
		args = append(args, "%"+relatedEntity+"%")
	}
	if investigationID > 0 {
		q += " AND investigation_id = ?"
		args = append(args, investigationID)
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []types.Todo
	for rows.Next() {
		var t types.Todo
		var entities, specs string
		var deadline, completedAt sql.NullTime
		var invID, sessID sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Title, &t.Content, &t.Status, &t.Priority, &deadline,
			&entities, &specs, &invID, &sessID, &t.CreatedAt, &t.UpdatedAt, &completedAt); err != nil {
			return nil, err
		}
		if deadline.Valid {
			t.Deadline = &deadline.Time
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.Time
		}
		if invID.Valid {
			t.InvestigationID = &invID.Int64
		}
		if sessID.Valid {
			t.SessionID = &sessID.Int64
		}
		t.RelatedEntities = unmarshalStrings(entities)
		t.RelatedSpecs = unmarshalStrings(specs)
		todos = append(todos, t)
	}
	return todos, rows.Err()
}

func (s *Store) UpdateTodo(ctx context.Context, id int64, title, content, priority *string, deadline *time.Time, relatedEntities []string) (*types.Todo, error) {
	old, err := s.GetTodo(ctx, id)
	if err != nil {
		return nil, err
	}

	sets := []string{"updated_at = CURRENT_TIMESTAMP"}
	var args []interface{}

	if title != nil && *title != old.Title {
		s.recordTodoHistory(ctx, id, "title", old.Title, *title, "mcp")
		sets = append(sets, "title = ?")
		args = append(args, *title)
	}
	if content != nil && *content != old.Content {
		s.recordTodoHistory(ctx, id, "content", old.Content, *content, "mcp")
		sets = append(sets, "content = ?")
		args = append(args, *content)
	}
	if priority != nil && *priority != old.Priority {
		s.recordTodoHistory(ctx, id, "priority", old.Priority, *priority, "mcp")
		sets = append(sets, "priority = ?")
		args = append(args, *priority)
	}
	if deadline != nil {
		sets = append(sets, "deadline = ?")
		args = append(args, deadline)
	}
	if relatedEntities != nil {
		sets = append(sets, "related_entities = ?")
		args = append(args, marshalJSON(relatedEntities))
	}

	args = append(args, id)
	_, err = s.DB.ExecContext(ctx,
		fmt.Sprintf("UPDATE todos SET %s WHERE id = ?", strings.Join(sets, ", ")), args...)
	if err != nil {
		return nil, err
	}
	return s.GetTodo(ctx, id)
}

func (s *Store) CompleteTodo(ctx context.Context, id int64) (*types.Todo, error) {
	old, err := s.GetTodo(ctx, id)
	if err != nil {
		return nil, err
	}
	if old.Status == "done" {
		return nil, fmt.Errorf("todo %d is already done", id)
	}

	s.recordTodoHistory(ctx, id, "status", old.Status, "done", "mcp")

	_, err = s.DB.ExecContext(ctx,
		"UPDATE todos SET status = 'done', completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	return s.GetTodo(ctx, id)
}

func (s *Store) recordTodoHistory(ctx context.Context, todoID int64, field, oldVal, newVal, source string) {
	s.DB.ExecContext(ctx,
		"INSERT INTO todos_history (todo_id, field, old_value, new_value, source) VALUES (?, ?, ?, ?, ?)",
		todoID, field, oldVal, newVal, source)
}

func (s *Store) GetTodoHistory(ctx context.Context, todoID int64) ([]types.HistoryEntry, error) {
	rows, err := s.DB.QueryContext(ctx,
		"SELECT id, todo_id, field, COALESCE(old_value,''), COALESCE(new_value,''), changed_at, COALESCE(source,'') FROM todos_history WHERE todo_id = ? ORDER BY changed_at DESC", todoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []types.HistoryEntry
	for rows.Next() {
		var h types.HistoryEntry
		if err := rows.Scan(&h.ID, &h.RefID, &h.Field, &h.OldValue, &h.NewValue, &h.ChangedAt, &h.Source); err != nil {
			return nil, err
		}
		entries = append(entries, h)
	}
	return entries, rows.Err()
}

// --- Decisions CRUD ---

func (s *Store) RecordDecision(ctx context.Context, d *types.Decision) (*types.Decision, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO decisions (title, choice, why, alternatives, category, related_entities, related_specs, invalidates_decision_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.Title, d.Choice, d.Why, marshalJSON(d.Alternatives), d.Category,
		marshalJSON(d.RelatedEntities), marshalJSON(d.RelatedSpecs), d.InvalidatesDecisionID)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetDecision(ctx, id)
}

func (s *Store) GetDecision(ctx context.Context, id int64) (*types.Decision, error) {
	var d types.Decision
	var alts, entities, specs string
	var invID sql.NullInt64
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, title, choice, COALESCE(why,''), COALESCE(alternatives,'[]'), COALESCE(category,''),
		        related_entities, related_specs, invalidates_decision_id, created_at
		 FROM decisions WHERE id = ?`, id).
		Scan(&d.ID, &d.Title, &d.Choice, &d.Why, &alts, &d.Category,
			&entities, &specs, &invID, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("decision with id %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	if invID.Valid {
		d.InvalidatesDecisionID = &invID.Int64
	}
	d.Alternatives = unmarshalStrings(alts)
	d.RelatedEntities = unmarshalStrings(entities)
	d.RelatedSpecs = unmarshalStrings(specs)
	return &d, nil
}

func (s *Store) SearchDecisions(ctx context.Context, category, relatedEntity string, since string, limit int) ([]types.Decision, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := "SELECT id, title, choice, COALESCE(why,''), COALESCE(alternatives,'[]'), COALESCE(category,''), related_entities, related_specs, invalidates_decision_id, created_at FROM decisions WHERE 1=1"
	var args []interface{}
	if category != "" {
		q += " AND category = ?"
		args = append(args, category)
	}
	if relatedEntity != "" {
		q += " AND related_entities LIKE ?"
		args = append(args, "%"+relatedEntity+"%")
	}
	if since != "" {
		q += " AND created_at >= ?"
		args = append(args, since)
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decisions []types.Decision
	for rows.Next() {
		var d types.Decision
		var alts, entities, specs string
		var invID sql.NullInt64
		if err := rows.Scan(&d.ID, &d.Title, &d.Choice, &d.Why, &alts, &d.Category,
			&entities, &specs, &invID, &d.CreatedAt); err != nil {
			return nil, err
		}
		if invID.Valid {
			d.InvalidatesDecisionID = &invID.Int64
		}
		d.Alternatives = unmarshalStrings(alts)
		d.RelatedEntities = unmarshalStrings(entities)
		d.RelatedSpecs = unmarshalStrings(specs)
		decisions = append(decisions, d)
	}
	return decisions, rows.Err()
}

// --- Investigations CRUD ---

func (s *Store) OpenInvestigation(ctx context.Context, inv *types.Investigation) (*types.Investigation, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO investigations (name, hypothesis, status, related_entities, related_specs)
		 VALUES (?, ?, 'open', ?, ?)`,
		inv.Name, inv.Hypothesis, marshalJSON(inv.RelatedEntities), marshalJSON(inv.RelatedSpecs))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("investigation %q already exists", inv.Name)
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetInvestigation(ctx, id)
}

func (s *Store) GetInvestigation(ctx context.Context, id int64) (*types.Investigation, error) {
	var inv types.Investigation
	var evidence, entities, specs string
	var closedAt sql.NullTime
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, name, hypothesis, status, COALESCE(conclusion,''), COALESCE(evidence,'[]'),
		        related_entities, related_specs, opened_at, closed_at
		 FROM investigations WHERE id = ?`, id).
		Scan(&inv.ID, &inv.Name, &inv.Hypothesis, &inv.Status, &inv.Conclusion, &evidence,
			&entities, &specs, &inv.OpenedAt, &closedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("investigation with id %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	if closedAt.Valid {
		inv.ClosedAt = &closedAt.Time
	}
	inv.Evidence = unmarshalStrings(evidence)
	inv.RelatedEntities = unmarshalStrings(entities)
	inv.RelatedSpecs = unmarshalStrings(specs)
	return &inv, nil
}

func (s *Store) UpdateInvestigation(ctx context.Context, id int64, addEvidence string, status string) (*types.Investigation, error) {
	old, err := s.GetInvestigation(ctx, id)
	if err != nil {
		return nil, err
	}

	if addEvidence != "" {
		ev := old.Evidence
		ev = append(ev, addEvidence)
		s.recordInvestigationHistory(ctx, id, "evidence", marshalJSON(old.Evidence), marshalJSON(ev), "mcp")
		s.DB.ExecContext(ctx, "UPDATE investigations SET evidence = ? WHERE id = ?", marshalJSON(ev), id)
	}

	if status != "" && status != old.Status {
		s.recordInvestigationHistory(ctx, id, "status", old.Status, status, "mcp")
		s.DB.ExecContext(ctx, "UPDATE investigations SET status = ? WHERE id = ?", status, id)
	}

	return s.GetInvestigation(ctx, id)
}

func (s *Store) CloseInvestigation(ctx context.Context, id int64, conclusion, status string) (*types.Investigation, error) {
	old, err := s.GetInvestigation(ctx, id)
	if err != nil {
		return nil, err
	}
	if status == "" {
		status = "closed"
	}
	s.recordInvestigationHistory(ctx, id, "status", old.Status, status, "mcp")
	s.recordInvestigationHistory(ctx, id, "conclusion", old.Conclusion, conclusion, "mcp")

	_, err = s.DB.ExecContext(ctx,
		"UPDATE investigations SET status = ?, conclusion = ?, closed_at = CURRENT_TIMESTAMP WHERE id = ?",
		status, conclusion, id)
	if err != nil {
		return nil, err
	}
	return s.GetInvestigation(ctx, id)
}

func (s *Store) ListInvestigations(ctx context.Context, status, relatedEntity string, limit int) ([]types.Investigation, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := "SELECT id, name, hypothesis, status, COALESCE(conclusion,''), COALESCE(evidence,'[]'), related_entities, related_specs, opened_at, closed_at FROM investigations WHERE 1=1"
	var args []interface{}
	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	}
	if relatedEntity != "" {
		q += " AND related_entities LIKE ?"
		args = append(args, "%"+relatedEntity+"%")
	}
	q += " ORDER BY opened_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var investigations []types.Investigation
	for rows.Next() {
		var inv types.Investigation
		var evidence, entities, specs string
		var closedAt sql.NullTime
		if err := rows.Scan(&inv.ID, &inv.Name, &inv.Hypothesis, &inv.Status, &inv.Conclusion, &evidence,
			&entities, &specs, &inv.OpenedAt, &closedAt); err != nil {
			return nil, err
		}
		if closedAt.Valid {
			inv.ClosedAt = &closedAt.Time
		}
		inv.Evidence = unmarshalStrings(evidence)
		inv.RelatedEntities = unmarshalStrings(entities)
		inv.RelatedSpecs = unmarshalStrings(specs)
		investigations = append(investigations, inv)
	}
	return investigations, rows.Err()
}

func (s *Store) recordInvestigationHistory(ctx context.Context, invID int64, field, oldVal, newVal, source string) {
	s.DB.ExecContext(ctx,
		"INSERT INTO investigations_history (investigation_id, field, old_value, new_value, source) VALUES (?, ?, ?, ?, ?)",
		invID, field, oldVal, newVal, source)
}

func (s *Store) GetInvestigationHistory(ctx context.Context, invID int64) ([]types.HistoryEntry, error) {
	rows, err := s.DB.QueryContext(ctx,
		"SELECT id, investigation_id, field, COALESCE(old_value,''), COALESCE(new_value,''), changed_at, COALESCE(source,'') FROM investigations_history WHERE investigation_id = ? ORDER BY changed_at DESC", invID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []types.HistoryEntry
	for rows.Next() {
		var h types.HistoryEntry
		if err := rows.Scan(&h.ID, &h.RefID, &h.Field, &h.OldValue, &h.NewValue, &h.ChangedAt, &h.Source); err != nil {
			return nil, err
		}
		entries = append(entries, h)
	}
	return entries, rows.Err()
}

// --- Sessions CRUD ---

func (s *Store) LogSession(ctx context.Context, sess *types.Session) (*types.Session, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions (title, summary, summary_path, decisions_made, todos_opened, todos_closed, related_entities, related_specs, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.Title, sess.Summary, sess.SummaryPath,
		marshalJSON(sess.DecisionsMade), marshalJSON(sess.TodosOpened), marshalJSON(sess.TodosClosed),
		marshalJSON(sess.RelatedEntities), marshalJSON(sess.RelatedSpecs), sess.Source)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetSession(ctx, id)
}

func (s *Store) GetSession(ctx context.Context, id int64) (*types.Session, error) {
	var sess types.Session
	var decMade, todosOpen, todosClosed, entities, specs string
	var summaryPath sql.NullString
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, COALESCE(title,''), summary, summary_path, COALESCE(decisions_made,'[]'),
		        COALESCE(todos_opened,'[]'), COALESCE(todos_closed,'[]'),
		        related_entities, related_specs, COALESCE(source,''), created_at
		 FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.Title, &sess.Summary, &summaryPath, &decMade,
			&todosOpen, &todosClosed, &entities, &specs, &sess.Source, &sess.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session with id %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	if summaryPath.Valid {
		sess.SummaryPath = summaryPath.String
	}
	sess.DecisionsMade = unmarshalInt64s(decMade)
	sess.TodosOpened = unmarshalInt64s(todosOpen)
	sess.TodosClosed = unmarshalInt64s(todosClosed)
	sess.RelatedEntities = unmarshalStrings(entities)
	sess.RelatedSpecs = unmarshalStrings(specs)
	return &sess, nil
}

func (s *Store) ListSessions(ctx context.Context, source string, limit int) ([]types.Session, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := "SELECT id, COALESCE(title,''), summary, summary_path, COALESCE(decisions_made,'[]'), COALESCE(todos_opened,'[]'), COALESCE(todos_closed,'[]'), related_entities, related_specs, COALESCE(source,''), created_at FROM sessions WHERE 1=1"
	var args []interface{}
	if source != "" {
		q += " AND source = ?"
		args = append(args, source)
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []types.Session
	for rows.Next() {
		var sess types.Session
		var decMade, todosOpen, todosClosed, entities, specs string
		var summaryPath sql.NullString
		if err := rows.Scan(&sess.ID, &sess.Title, &sess.Summary, &summaryPath, &decMade,
			&todosOpen, &todosClosed, &entities, &specs, &sess.Source, &sess.CreatedAt); err != nil {
			return nil, err
		}
		if summaryPath.Valid {
			sess.SummaryPath = summaryPath.String
		}
		sess.DecisionsMade = unmarshalInt64s(decMade)
		sess.TodosOpened = unmarshalInt64s(todosOpen)
		sess.TodosClosed = unmarshalInt64s(todosClosed)
		sess.RelatedEntities = unmarshalStrings(entities)
		sess.RelatedSpecs = unmarshalStrings(specs)
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// --- DB helpers ---

func (s *Store) DBSize(ctx context.Context) (int64, error) {
	var pageCount, pageSize int64
	if err := s.DB.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, err
	}
	if err := s.DB.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, err
	}
	return pageCount * pageSize, nil
}

func coalesce(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
