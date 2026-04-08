package server

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/rolivape/hiverod-mcp-go/hivemcp"

	"github.com/rolivape/planrod/internal/search"
	"github.com/rolivape/planrod/internal/sessions"
	"github.com/rolivape/planrod/internal/types"
)

// --- Todos (5 tools) ---

func (s *Server) toolCreateTodo() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_create_todo",
		Description: "Create a new todo item with optional priority, deadline, and cross-service references.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title":            map[string]interface{}{"type": "string", "description": "Todo title"},
				"content":          map[string]interface{}{"type": "string", "description": "Detailed description"},
				"priority":         map[string]interface{}{"type": "string", "enum": []string{"low", "medium", "high", "urgent"}},
				"deadline":         map[string]interface{}{"type": "string", "description": "ISO 8601 timestamp"},
				"related_entities": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"related_specs":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"investigation_id": map[string]interface{}{"type": "integer"},
				"session_id":       map[string]interface{}{"type": "integer"},
				"_options": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"cross_service_mode": map[string]interface{}{"type": "string", "enum": []string{"lite", "strict"}},
					},
				},
			},
			"required": []string{"title"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			title := getString(args, "title")
			if title == "" {
				return errorResult("title is required"), nil
			}

			// Cross-service validation
			override := getCrossServiceOverride(args)
			entities := getStringSlice(args, "related_entities")
			specs := getStringSlice(args, "related_specs")
			if len(entities) > 0 {
				if _, err := s.cross.ValidateEntities(ctx, entities, override); err != nil {
					return errorResult(err.Error()), nil
				}
			}
			if len(specs) > 0 {
				if _, err := s.cross.ValidateSpecs(ctx, specs, override); err != nil {
					return errorResult(err.Error()), nil
				}
			}

			t := &types.Todo{
				Title:           title,
				Content:         getString(args, "content"),
				Priority:        getString(args, "priority"),
				RelatedEntities: entities,
				RelatedSpecs:    specs,
			}

			if dl := getString(args, "deadline"); dl != "" {
				parsed, err := time.Parse(time.RFC3339, dl)
				if err != nil {
					parsed, err = time.Parse("2006-01-02", dl)
				}
				if err == nil {
					t.Deadline = &parsed
				}
			}

			invID := getInt(args, "investigation_id", 0)
			if invID > 0 {
				id := int64(invID)
				t.InvestigationID = &id
			}
			sessID := getInt(args, "session_id", 0)
			if sessID > 0 {
				id := int64(sessID)
				t.SessionID = &id
			}

			created, err := s.store.CreateTodo(ctx, t)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			s.embedAsync(ctx, "todo", created.ID, created.Title+" "+created.Content)

			result := jsonResult(created)
			result.Meta = map[string]interface{}{
				hivemcp.KeyLatencyMs: time.Since(start).Milliseconds(),
			}
			return result, nil
		},
	}
}

func (s *Server) toolListTodos() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_list_todos",
		Description: "List todos, optionally filtered by status, priority, related entity, or investigation.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status":           map[string]interface{}{"type": "string", "enum": []string{"pending", "in_progress", "done", "cancelled"}},
				"priority":         map[string]interface{}{"type": "string", "enum": []string{"low", "medium", "high", "urgent"}},
				"related_entity":   map[string]interface{}{"type": "string"},
				"investigation_id": map[string]interface{}{"type": "integer"},
				"limit":            map[string]interface{}{"type": "integer", "default": 20, "maximum": 100},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			todos, err := s.store.ListTodos(ctx,
				getString(args, "status"),
				getString(args, "priority"),
				getString(args, "related_entity"),
				int64(getInt(args, "investigation_id", 0)),
				getInt(args, "limit", 20))
			if err != nil {
				return errorResult(err.Error()), nil
			}

			result := jsonResult(todos)
			result.Meta = map[string]interface{}{
				hivemcp.KeyLatencyMs:   time.Since(start).Milliseconds(),
				hivemcp.KeyResultCount: len(todos),
			}
			return result, nil
		},
	}
}

func (s *Server) toolGetTodo() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_get_todo",
		Description: "Get a todo by ID, including its full change history.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{"type": "integer", "description": "Todo ID"},
			},
			"required": []string{"id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			id := int64(getInt(args, "id", 0))
			todo, err := s.store.GetTodo(ctx, id)
			if err != nil {
				return errorResult(err.Error()), nil
			}
			history, _ := s.store.GetTodoHistory(ctx, id)
			data := map[string]interface{}{
				"todo":    todo,
				"history": history,
			}
			result := jsonResult(data)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

func (s *Server) toolUpdateTodo() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_update_todo",
		Description: "Update a todo's title, content, priority, deadline, or related entities. Use plan_complete_todo to mark as done.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":               map[string]interface{}{"type": "integer"},
				"title":            map[string]interface{}{"type": "string"},
				"content":          map[string]interface{}{"type": "string"},
				"priority":         map[string]interface{}{"type": "string", "enum": []string{"low", "medium", "high", "urgent"}},
				"deadline":         map[string]interface{}{"type": "string"},
				"related_entities": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			},
			"required": []string{"id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			id := int64(getInt(args, "id", 0))

			var title, content, priority *string
			if v := getString(args, "title"); v != "" {
				title = &v
			}
			if v, ok := args["content"].(string); ok {
				content = &v
			}
			if v := getString(args, "priority"); v != "" {
				priority = &v
			}

			var deadline *time.Time
			if dl := getString(args, "deadline"); dl != "" {
				parsed, err := time.Parse(time.RFC3339, dl)
				if err != nil {
					parsed, err = time.Parse("2006-01-02", dl)
				}
				if err == nil {
					deadline = &parsed
				}
			}

			entities := getStringSlice(args, "related_entities")

			updated, err := s.store.UpdateTodo(ctx, id, title, content, priority, deadline, entities)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			result := jsonResult(updated)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

func (s *Server) toolCompleteTodo() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_complete_todo",
		Description: "Mark a todo as done. Sets status='done' and completed_at=now, records change in history.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{"type": "integer", "description": "Todo ID to complete"},
			},
			"required": []string{"id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			id := int64(getInt(args, "id", 0))
			todo, err := s.store.CompleteTodo(ctx, id)
			if err != nil {
				return errorResult(err.Error()), nil
			}
			result := jsonResult(todo)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

// --- Decisions (3 tools) ---

func (s *Server) toolRecordDecision() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_record_decision",
		Description: "Record a decision with choice, reasoning, alternatives, and optional cross-service references.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title":                   map[string]interface{}{"type": "string"},
				"choice":                  map[string]interface{}{"type": "string", "description": "The decision itself"},
				"why":                     map[string]interface{}{"type": "string"},
				"alternatives":            map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"category":                map[string]interface{}{"type": "string"},
				"related_entities":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"related_specs":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"invalidates_decision_id": map[string]interface{}{"type": "integer"},
				"_options": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"cross_service_mode": map[string]interface{}{"type": "string", "enum": []string{"lite", "strict"}},
					},
				},
			},
			"required": []string{"title", "choice"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()

			override := getCrossServiceOverride(args)
			entities := getStringSlice(args, "related_entities")
			specs := getStringSlice(args, "related_specs")
			if len(entities) > 0 {
				if _, err := s.cross.ValidateEntities(ctx, entities, override); err != nil {
					return errorResult(err.Error()), nil
				}
			}
			if len(specs) > 0 {
				if _, err := s.cross.ValidateSpecs(ctx, specs, override); err != nil {
					return errorResult(err.Error()), nil
				}
			}

			d := &types.Decision{
				Title:           getString(args, "title"),
				Choice:          getString(args, "choice"),
				Why:             getString(args, "why"),
				Alternatives:    getStringSlice(args, "alternatives"),
				Category:        getString(args, "category"),
				RelatedEntities: entities,
				RelatedSpecs:    specs,
			}
			invID := getInt(args, "invalidates_decision_id", 0)
			if invID > 0 {
				id := int64(invID)
				d.InvalidatesDecisionID = &id
			}

			created, err := s.store.RecordDecision(ctx, d)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			s.embedAsync(ctx, "decision", created.ID, created.Title+" "+created.Choice+" "+created.Why)

			result := jsonResult(created)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

func (s *Server) toolSearchDecisions() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_search_decisions",
		Description: "Search decisions using hybrid search (FTS + vector) on title/choice/why, optionally filtered by category or entity.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":          map[string]interface{}{"type": "string"},
				"category":       map[string]interface{}{"type": "string"},
				"related_entity": map[string]interface{}{"type": "string"},
				"since":          map[string]interface{}{"type": "string", "description": "ISO 8601 timestamp"},
				"limit":          map[string]interface{}{"type": "integer", "default": 20},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			query := getString(args, "query")

			// If there's a text query, use search engine for hybrid search
			if query != "" {
				results, meta, err := s.search.Search(ctx, query, search.SearchOpts{
					Mode:  "hybrid",
					Limit: getInt(args, "limit", 20),
					Types: []string{"decision"},
				})
				if err != nil {
					return errorResult(err.Error()), nil
				}
				result := jsonResult(results)
				result.Meta = map[string]interface{}{
					hivemcp.KeyLatencyMs:   time.Since(start).Milliseconds(),
					hivemcp.KeyResultCount: len(results),
					hivemcp.KeySearchMode:  meta.Mode,
				}
				if meta.Degraded {
					result.Meta["degraded"] = true
				}
				return result, nil
			}

			// Otherwise, use store for filtered listing
			decisions, err := s.store.SearchDecisions(ctx,
				getString(args, "category"),
				getString(args, "related_entity"),
				getString(args, "since"),
				getInt(args, "limit", 20))
			if err != nil {
				return errorResult(err.Error()), nil
			}
			result := jsonResult(decisions)
			result.Meta = map[string]interface{}{
				hivemcp.KeyLatencyMs:   time.Since(start).Milliseconds(),
				hivemcp.KeyResultCount: len(decisions),
			}
			return result, nil
		},
	}
}

func (s *Server) toolGetDecision() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_get_decision",
		Description: "Get a decision by ID, including any decision it invalidates or was invalidated by.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{"type": "integer"},
			},
			"required": []string{"id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			id := int64(getInt(args, "id", 0))
			d, err := s.store.GetDecision(ctx, id)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			data := map[string]interface{}{"decision": d}
			if d.InvalidatesDecisionID != nil {
				old, err := s.store.GetDecision(ctx, *d.InvalidatesDecisionID)
				if err == nil {
					data["invalidates"] = old
				}
			}

			result := jsonResult(data)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

// --- Investigations (4 tools) ---

func (s *Server) toolOpenInvestigation() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_open_investigation",
		Description: "Open a new investigation with a hypothesis and optional cross-service references.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":             map[string]interface{}{"type": "string"},
				"hypothesis":       map[string]interface{}{"type": "string"},
				"related_entities": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"_options": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"cross_service_mode": map[string]interface{}{"type": "string", "enum": []string{"lite", "strict"}},
					},
				},
			},
			"required": []string{"name", "hypothesis"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()

			override := getCrossServiceOverride(args)
			entities := getStringSlice(args, "related_entities")
			if len(entities) > 0 {
				if _, err := s.cross.ValidateEntities(ctx, entities, override); err != nil {
					return errorResult(err.Error()), nil
				}
			}

			inv := &types.Investigation{
				Name:            getString(args, "name"),
				Hypothesis:      getString(args, "hypothesis"),
				RelatedEntities: entities,
			}

			created, err := s.store.OpenInvestigation(ctx, inv)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			s.embedAsync(ctx, "investigation", created.ID, created.Name+" "+created.Hypothesis)

			result := jsonResult(created)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

func (s *Server) toolUpdateInvestigation() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_update_investigation",
		Description: "Update an investigation: add evidence or change status.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":           map[string]interface{}{"type": "integer"},
				"add_evidence": map[string]interface{}{"type": "string", "description": "Evidence to add to the list"},
				"status":       map[string]interface{}{"type": "string", "enum": []string{"open", "in_progress", "closed", "abandoned"}},
			},
			"required": []string{"id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			id := int64(getInt(args, "id", 0))
			updated, err := s.store.UpdateInvestigation(ctx, id, getString(args, "add_evidence"), getString(args, "status"))
			if err != nil {
				return errorResult(err.Error()), nil
			}
			result := jsonResult(updated)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

func (s *Server) toolCloseInvestigation() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_close_investigation",
		Description: "Close an investigation with a conclusion. Defaults to status 'closed', can also be 'abandoned'.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":         map[string]interface{}{"type": "integer"},
				"conclusion": map[string]interface{}{"type": "string"},
				"status":     map[string]interface{}{"type": "string", "enum": []string{"closed", "abandoned"}, "default": "closed"},
			},
			"required": []string{"id", "conclusion"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			id := int64(getInt(args, "id", 0))
			closed, err := s.store.CloseInvestigation(ctx, id, getString(args, "conclusion"), getString(args, "status"))
			if err != nil {
				return errorResult(err.Error()), nil
			}
			result := jsonResult(closed)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

func (s *Server) toolListInvestigations() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_list_investigations",
		Description: "List investigations, optionally filtered by status or related entity.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status":         map[string]interface{}{"type": "string", "enum": []string{"open", "in_progress", "closed", "abandoned"}},
				"related_entity": map[string]interface{}{"type": "string"},
				"limit":          map[string]interface{}{"type": "integer", "default": 20},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			investigations, err := s.store.ListInvestigations(ctx,
				getString(args, "status"),
				getString(args, "related_entity"),
				getInt(args, "limit", 20))
			if err != nil {
				return errorResult(err.Error()), nil
			}
			result := jsonResult(investigations)
			result.Meta = map[string]interface{}{
				hivemcp.KeyLatencyMs:   time.Since(start).Milliseconds(),
				hivemcp.KeyResultCount: len(investigations),
			}
			return result, nil
		},
	}
}

// --- Sessions (2 tools) ---

func (s *Server) toolLogSession() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_log_session",
		Description: "Log a session snapshot. If summary >2500 chars, saves to filesystem with git commit.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"summary":          map[string]interface{}{"type": "string"},
				"title":            map[string]interface{}{"type": "string"},
				"decisions_made":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "integer"}},
				"todos_opened":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "integer"}},
				"todos_closed":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "integer"}},
				"related_entities": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"related_specs":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"source":           map[string]interface{}{"type": "string", "description": "claude-ai | claude-code | grok | manual"},
			},
			"required": []string{"summary"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			summary := getString(args, "summary")
			if summary == "" {
				return errorResult("summary is required"), nil
			}

			sess := &types.Session{
				Title:           getString(args, "title"),
				Summary:         summary,
				DecisionsMade:   getInt64Slice(args, "decisions_made"),
				TodosOpened:     getInt64Slice(args, "todos_opened"),
				TodosClosed:     getInt64Slice(args, "todos_closed"),
				RelatedEntities: getStringSlice(args, "related_entities"),
				RelatedSpecs:    getStringSlice(args, "related_specs"),
				Source:          getString(args, "source"),
			}

			// Save to DB first to get the ID
			created, err := s.store.LogSession(ctx, sess)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			// If summary is long, save to filesystem
			if len(summary) > sessions.SummaryThreshold {
				path, err := s.sessions.WriteSummary(created.ID, summary)
				if err != nil {
					s.logger.Warn("failed to write session to filesystem", "error", err, "session_id", created.ID)
				} else {
					// Update DB with path and truncated summary
					truncated := summary[:500] + "..."
					s.store.DB.ExecContext(ctx,
						"UPDATE sessions SET summary = ?, summary_path = ? WHERE id = ?",
						truncated, path, created.ID)
					created.SummaryPath = path
					created.Summary = truncated
				}
			}

			s.embedAsync(ctx, "session", created.ID, fmt.Sprintf("%s %s", created.Title, summary))

			result := jsonResult(created)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

func (s *Server) toolGetSession() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_get_session",
		Description: "Get a session by ID. If summary was saved to filesystem, expands the full content.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{"type": "integer"},
			},
			"required": []string{"id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			id := int64(getInt(args, "id", 0))
			sess, err := s.store.GetSession(ctx, id)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			if sess.SummaryPath != "" {
				fullSummary, err := s.sessions.ReadSummary(sess.SummaryPath)
				if err == nil {
					sess.Summary = fullSummary
				}
			}

			result := jsonResult(sess)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

// --- Cross-cutting (2 tools) ---

func (s *Server) toolPlanSearch() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_search",
		Description: "Search across todos, decisions, investigations, and sessions using hybrid (FTS5 + vector) search with RRF fusion.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string", "description": "Search query"},
				"types": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string", "enum": []string{"todo", "decision", "investigation", "session"}},
				},
				"mode":  map[string]interface{}{"type": "string", "enum": []string{"hybrid", "fts", "vec"}, "default": "hybrid"},
				"limit": map[string]interface{}{"type": "integer", "default": 10, "maximum": 50},
			},
			"required": []string{"query"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			query := getString(args, "query")
			if query == "" {
				return errorResult("query is required"), nil
			}

			opts := search.SearchOpts{
				Mode:  getString(args, "mode"),
				Limit: getInt(args, "limit", 10),
				Types: getStringSlice(args, "types"),
			}

			results, meta, err := s.search.Search(ctx, query, opts)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			result := jsonResult(results)
			result.Meta = map[string]interface{}{
				hivemcp.KeyLatencyMs:   time.Since(start).Milliseconds(),
				hivemcp.KeySearchMode:  meta.Mode,
				hivemcp.KeyResultCount: len(results),
			}
			if meta.Degraded {
				result.Meta["degraded"] = true
				result.Meta["reason"] = meta.Reason
			}
			return result, nil
		},
	}
}

func (s *Server) toolPlanHealth() hivemcp.ToolDefinition {
	return hivemcp.ToolDefinition{
		Name:        "plan_health",
		Description: "Return service health status: database, embedder, sessions repo, and MemoryRod connectivity.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (hivemcp.ToolResult, error) {
			start := time.Now()
			health := map[string]interface{}{
				"status":  "ok",
				"db":      "ok",
				"service": "planrod",
				"version": "0.1.0",
			}

			if err := s.store.DB.Ping(); err != nil {
				health["status"] = "down"
				health["db"] = "error: " + err.Error()
			}

			if s.sessions.IsValid() {
				health["sessions_repo"] = "ok"
			} else {
				health["sessions_repo"] = "not initialized"
			}

			if err := s.embedder.IsHealthy(ctx); err != nil {
				health["status"] = "degraded"
				health["embedder"] = "unhealthy"
			} else {
				health["embedder"] = "healthy"
			}

			if s.cross.Mode != "none" {
				if s.cross.IsReachable(ctx) {
					health["memoryrod"] = "reachable"
				} else {
					health["memoryrod"] = "unreachable"
				}
			}

			result := jsonResult(health)
			result.Meta = map[string]interface{}{hivemcp.KeyLatencyMs: time.Since(start).Milliseconds()}
			return result, nil
		},
	}
}

// --- Async embedding helper ---

func (s *Server) embedAsync(ctx context.Context, refType string, refID int64, text string) {
	if !s.embedder.IsAvailable() {
		return
	}
	go func() {
		vec, err := s.embedder.Embed(context.Background(), text)
		if err != nil {
			s.logger.Warn("embedding failed", "ref_type", refType, "ref_id", refID, "error", err)
			return
		}

		vecBytes := serializeFloat32(vec)

		// Upsert embedding
		tx, err := s.store.DB.Begin()
		if err != nil {
			return
		}
		defer tx.Rollback()

		// Check if exists
		var existingRowid int64
		err = tx.QueryRow("SELECT rowid FROM embeddings_meta WHERE ref_type = ? AND ref_id = ?", refType, refID).Scan(&existingRowid)
		if err == nil {
			// Update
			tx.Exec("UPDATE vec_embeddings SET embedding = ? WHERE rowid = ?", vecBytes, existingRowid)
			tx.Exec("UPDATE embeddings_meta SET model_version = ?, created_at = CURRENT_TIMESTAMP WHERE rowid = ?", s.embedder.ModelVersion(), existingRowid)
		} else {
			// Insert
			res, err := tx.Exec("INSERT INTO vec_embeddings(embedding) VALUES (?)", vecBytes)
			if err != nil {
				return
			}
			rowid, _ := res.LastInsertId()
			tx.Exec("INSERT INTO embeddings_meta (rowid, ref_type, ref_id, model_version) VALUES (?, ?, ?, ?)",
				rowid, refType, refID, s.embedder.ModelVersion())
		}
		tx.Commit()
	}()
}

func serializeFloat32(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		bits := math.Float32bits(f)
		buf[i*4] = byte(bits)
		buf[i*4+1] = byte(bits >> 8)
		buf[i*4+2] = byte(bits >> 16)
		buf[i*4+3] = byte(bits >> 24)
	}
	return buf
}
