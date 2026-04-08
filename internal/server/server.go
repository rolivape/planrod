package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/rolivape/hiverod-mcp-go/hivemcp"
	"github.com/rolivape/hiverod-mcp-go/metrics"

	"github.com/rolivape/planrod/internal/config"
	"github.com/rolivape/planrod/internal/crossservice"
	"github.com/rolivape/planrod/internal/embed"
	"github.com/rolivape/planrod/internal/search"
	"github.com/rolivape/planrod/internal/sessions"
	"github.com/rolivape/planrod/internal/store"
)

type Server struct {
	cfg      config.Config
	store    *store.Store
	search   *search.Engine
	sessions *sessions.Manager
	embedder *embed.Manager
	cross    *crossservice.Client
	metrics  *metrics.Registry
	logger   *slog.Logger
	mcpSrv   *hivemcp.Server
}

func New(cfg config.Config, st *store.Store, eng *search.Engine, sess *sessions.Manager, em *embed.Manager, cs *crossservice.Client, mr *metrics.Registry, logger *slog.Logger) (*Server, error) {
	s := &Server{
		cfg:      cfg,
		store:    st,
		search:   eng,
		sessions: sess,
		embedder: em,
		cross:    cs,
		metrics:  mr,
		logger:   logger,
	}

	mcpSrv, err := hivemcp.NewServer(hivemcp.Config{
		ServiceName:     "planrod",
		ServiceVersion:  "0.1.0",
		ListenAddr:      fmt.Sprintf(":%d", cfg.MCPPort),
		Logger:          logger,
		MetricsEnabled:  true,
		MetricsAddr:     fmt.Sprintf(":%d", cfg.MetricsPort),
		MetricsRegistry: mr,
	})
	if err != nil {
		return nil, err
	}
	s.mcpSrv = mcpSrv

	if err := s.registerTools(); err != nil {
		return nil, err
	}

	mcpSrv.HandleFunc("/healthz", s.healthzHandler)

	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	go s.updateDBMetrics(ctx)
	return s.mcpSrv.Run(ctx)
}

func (s *Server) updateDBMetrics(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if size, err := s.store.DBSize(ctx); err == nil {
				s.metrics.DBSize.Set(float64(size))
			}
		}
	}
}

func (s *Server) registerTools() error {
	return s.mcpSrv.RegisterTools(
		// Todos (5)
		s.toolCreateTodo(),
		s.toolListTodos(),
		s.toolGetTodo(),
		s.toolUpdateTodo(),
		s.toolCompleteTodo(),
		// Decisions (3)
		s.toolRecordDecision(),
		s.toolSearchDecisions(),
		s.toolGetDecision(),
		// Investigations (4)
		s.toolOpenInvestigation(),
		s.toolUpdateInvestigation(),
		s.toolCloseInvestigation(),
		s.toolListInvestigations(),
		// Sessions (2)
		s.toolLogSession(),
		s.toolGetSession(),
		// Cross-cutting (2)
		s.toolPlanSearch(),
		s.toolPlanHealth(),
	)
}

func (s *Server) healthzHandler(w http.ResponseWriter, r *http.Request) {
	result := map[string]interface{}{
		"status":  "ok",
		"db":      "ok",
		"service": "planrod",
		"version": "0.1.0",
	}

	if err := s.store.DB.Ping(); err != nil {
		result["status"] = "down"
		result["db"] = "error: " + err.Error()
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(result)
		return
	}

	if s.sessions.IsValid() {
		result["sessions_repo"] = "ok"
	} else {
		result["sessions_repo"] = "not initialized"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.embedder.IsHealthy(ctx); err != nil {
		result["status"] = "degraded"
		result["embedder"] = "unhealthy"
	} else {
		result["embedder"] = "healthy"
		result["embedder_version"] = s.embedder.ModelVersion()
	}

	if s.cross.Mode != "none" {
		ctx2, cancel2 := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel2()
		if s.cross.IsReachable(ctx2) {
			result["memoryrod"] = "reachable"
		} else {
			result["memoryrod"] = "unreachable"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func getString(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return v
}

func getInt(args map[string]interface{}, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	}
	return def
}

func getStringSlice(args map[string]interface{}, key string) []string {
	v, ok := args[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		var result []string
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return val
	}
	return nil
}

func getInt64Slice(args map[string]interface{}, key string) []int64 {
	v, ok := args[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		var result []int64
		for _, item := range val {
			switch n := item.(type) {
			case float64:
				result = append(result, int64(n))
			case json.Number:
				i, _ := n.Int64()
				result = append(result, i)
			}
		}
		return result
	}
	return nil
}

func getCrossServiceOverride(args map[string]interface{}) string {
	opts, ok := args["_options"].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := opts["cross_service_mode"].(string)
	return v
}

func textResult(text string) hivemcp.ToolResult {
	return hivemcp.ToolResult{
		Content: []hivemcp.ContentBlock{{Type: "text", Text: text}},
	}
}

func jsonResult(data interface{}) hivemcp.ToolResult {
	b, _ := json.MarshalIndent(data, "", "  ")
	return hivemcp.ToolResult{
		Content: []hivemcp.ContentBlock{{Type: "text", Text: string(b)}},
	}
}

func errorResult(msg string) hivemcp.ToolResult {
	return hivemcp.ToolResult{
		Content: []hivemcp.ContentBlock{{Type: "text", Text: msg}},
		IsError: true,
	}
}
