package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"

	hivestore "github.com/rolivape/hiverod-mcp-go/store"
	"github.com/rolivape/planrod/internal/types"
)

func init() {
	sqlite_vec.Auto()
}

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "planrod-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	path := tmpFile.Name()

	db, err := hivestore.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}

	// Run migrations inline
	migrations := []string{
		`CREATE TABLE investigations (
			id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, hypothesis TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open', conclusion TEXT, evidence TEXT,
			related_entities TEXT, related_specs TEXT,
			opened_at DATETIME DEFAULT CURRENT_TIMESTAMP, closed_at DATETIME);`,
		`CREATE TABLE sessions (
			id INTEGER PRIMARY KEY, title TEXT, summary TEXT NOT NULL, summary_path TEXT,
			decisions_made TEXT, todos_opened TEXT, todos_closed TEXT,
			related_entities TEXT, related_specs TEXT, source TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
		`CREATE TABLE todos (
			id INTEGER PRIMARY KEY, title TEXT NOT NULL, content TEXT,
			status TEXT NOT NULL DEFAULT 'pending', priority TEXT, deadline DATETIME,
			related_entities TEXT, related_specs TEXT,
			investigation_id INTEGER REFERENCES investigations(id) ON DELETE SET NULL,
			session_id INTEGER REFERENCES sessions(id) ON DELETE SET NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, completed_at DATETIME);`,
		`CREATE TABLE decisions (
			id INTEGER PRIMARY KEY, title TEXT NOT NULL, choice TEXT NOT NULL,
			why TEXT, alternatives TEXT, category TEXT,
			related_entities TEXT, related_specs TEXT,
			invalidates_decision_id INTEGER REFERENCES decisions(id) ON DELETE SET NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
		`CREATE TABLE todos_history (
			id INTEGER PRIMARY KEY, todo_id INTEGER NOT NULL, field TEXT NOT NULL,
			old_value TEXT, new_value TEXT,
			changed_at DATETIME DEFAULT CURRENT_TIMESTAMP, source TEXT);`,
		`CREATE TABLE investigations_history (
			id INTEGER PRIMARY KEY, investigation_id INTEGER NOT NULL, field TEXT NOT NULL,
			old_value TEXT, new_value TEXT,
			changed_at DATETIME DEFAULT CURRENT_TIMESTAMP, source TEXT);`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			t.Fatalf("migration failed: %v\nSQL: %s", err, m)
		}
	}

	return db, func() {
		db.Close()
		os.Remove(path)
	}
}

func TestTodoCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	st := New(db)
	ctx := context.Background()

	// Create
	todo, err := st.CreateTodo(ctx, &types.Todo{
		Title:           "Test todo",
		Content:         "Some content",
		Priority:        "high",
		RelatedEntities: []string{"frigate"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if todo.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if todo.Status != "pending" {
		t.Errorf("expected pending, got %s", todo.Status)
	}

	// Get
	got, err := st.GetTodo(ctx, todo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Test todo" {
		t.Errorf("expected 'Test todo', got %q", got.Title)
	}
	if len(got.RelatedEntities) != 1 || got.RelatedEntities[0] != "frigate" {
		t.Errorf("expected [frigate], got %v", got.RelatedEntities)
	}

	// Complete
	completed, err := st.CompleteTodo(ctx, todo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "done" {
		t.Errorf("expected done, got %s", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}

	// History
	history, err := st.GetTodoHistory(ctx, todo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}

	// List
	todos, err := st.ListTodos(ctx, "done", "", "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 1 {
		t.Errorf("expected 1 done todo, got %d", len(todos))
	}
}

func TestDecisionCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	st := New(db)
	ctx := context.Background()

	d, err := st.RecordDecision(ctx, &types.Decision{
		Title:        "Use SQLite",
		Choice:       "SQLite with WAL",
		Why:          "Consistency",
		Category:     "architecture",
		Alternatives: []string{"PostgreSQL", "Redis"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	got, err := st.GetDecision(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Choice != "SQLite with WAL" {
		t.Errorf("expected 'SQLite with WAL', got %q", got.Choice)
	}
	if len(got.Alternatives) != 2 {
		t.Errorf("expected 2 alternatives, got %d", len(got.Alternatives))
	}
}

func TestInvestigationCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	st := New(db)
	ctx := context.Background()

	inv, err := st.OpenInvestigation(ctx, &types.Investigation{
		Name:       "test-inv",
		Hypothesis: "Something is broken",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != "open" {
		t.Errorf("expected open, got %s", inv.Status)
	}

	// Add evidence
	updated, err := st.UpdateInvestigation(ctx, inv.ID, "Found error in logs", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Evidence) != 1 {
		t.Errorf("expected 1 evidence, got %d", len(updated.Evidence))
	}

	// Close
	closed, err := st.CloseInvestigation(ctx, inv.ID, "Fixed it", "closed")
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != "closed" {
		t.Errorf("expected closed, got %s", closed.Status)
	}
	if closed.Conclusion != "Fixed it" {
		t.Errorf("expected 'Fixed it', got %q", closed.Conclusion)
	}

	// Duplicate name should fail
	_, err = st.OpenInvestigation(ctx, &types.Investigation{
		Name:       "test-inv",
		Hypothesis: "Another one",
	})
	if err == nil {
		t.Error("expected duplicate name error")
	}
}

func TestFKConstraint(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	st := New(db)
	ctx := context.Background()

	// Try to create todo with non-existent investigation_id
	invID := int64(999)
	_, err := st.CreateTodo(ctx, &types.Todo{
		Title:           "FK test",
		InvestigationID: &invID,
	})
	if err == nil {
		t.Error("expected FK constraint error")
	}
}
