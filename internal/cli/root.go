package cli

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/rolivape/hiverod-mcp-go/clihelp"
	"github.com/rolivape/hiverod-mcp-go/embeddings"
	"github.com/rolivape/hiverod-mcp-go/logging"
	"github.com/rolivape/hiverod-mcp-go/metrics"
	"github.com/rolivape/hiverod-mcp-go/schema"
	hivestore "github.com/rolivape/hiverod-mcp-go/store"

	"github.com/rolivape/planrod/internal/config"
	"github.com/rolivape/planrod/internal/crossservice"
	membed "github.com/rolivape/planrod/internal/embed"
	"github.com/rolivape/planrod/internal/search"
	"github.com/rolivape/planrod/internal/server"
	"github.com/rolivape/planrod/internal/sessions"
	"github.com/rolivape/planrod/internal/store"
	"github.com/rolivape/planrod/internal/types"
)

var migrationsFS embed.FS
var cfg config.Config

func NewRootCmd(migrations embed.FS) *cobra.Command {
	migrationsFS = migrations
	root := clihelp.NewRootCmd("plan", "0.1.0")
	root.Short = "PlanRod — HiveRod Cognitive Planning Service"

	outputFlag := clihelp.AddOutputFlag(root)

	// --- Serve ---
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd.Context())
		},
	}
	serveCmd.Flags().IntVar(&cfg.MCPPort, "mcp-port", 9101, "MCP server port")
	serveCmd.Flags().IntVar(&cfg.MetricsPort, "metrics-port", 9111, "Metrics port")
	serveCmd.Flags().StringVar(&cfg.DBPath, "db", "/opt/plan/data/plan.db", "Database path")
	serveCmd.Flags().StringVar(&cfg.SessionsDir, "sessions-dir", "/opt/plan/sessions", "Sessions directory")
	serveCmd.Flags().StringVar(&cfg.OllamaURL, "ollama-url", "http://localhost:11434", "Ollama URL")
	serveCmd.Flags().StringVar(&cfg.OllamaModel, "ollama-model", "nomic-embed-text", "Ollama model")
	serveCmd.Flags().Float64Var(&cfg.RRFWeightFTS, "rrf-w-fts", 0.4, "RRF weight for FTS")
	serveCmd.Flags().Float64Var(&cfg.RRFWeightVec, "rrf-w-vec", 0.6, "RRF weight for vec")
	serveCmd.Flags().StringVar(&cfg.CrossServiceMode, "cross-service-mode", "lite", "Cross-service mode: none|lite|strict")
	serveCmd.Flags().StringVar(&cfg.MemoryRodURL, "memoryrod-url", "http://localhost:9100/mcp", "MemoryRod MCP URL")
	serveCmd.Flags().StringVar(&cfg.CrossServiceTimeout, "cross-service-timeout", "500ms", "Cross-service timeout")
	root.AddCommand(serveCmd)

	// --- Migrate ---
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run pending SQL migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrate()
		},
	}
	migrateCmd.Flags().StringVar(&cfg.DBPath, "db", "/opt/plan/data/plan.db", "Database path")
	root.AddCommand(migrateCmd)

	// --- Healthcheck ---
	root.AddCommand(&cobra.Command{
		Use:   "healthcheck",
		Short: "Print service health status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHealthcheck()
		},
	})

	// --- Search ---
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search across todos, decisions, investigations, and sessions",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, _ := cmd.Flags().GetString("mode")
			limit, _ := cmd.Flags().GetInt("limit")
			typesStr, _ := cmd.Flags().GetString("type")
			var types []string
			if typesStr != "" {
				types = strings.Split(typesStr, ",")
			}
			return runSearch(strings.Join(args, " "), mode, types, limit, clihelp.OutputFormat(*outputFlag))
		},
	}
	searchCmd.Flags().String("mode", "hybrid", "Search mode: hybrid|fts|vec")
	searchCmd.Flags().Int("limit", 10, "Max results")
	searchCmd.Flags().String("type", "", "Filter by type: todo|decision|investigation|session (comma-separated)")
	root.AddCommand(searchCmd)

	// --- Get ---
	getCmd := &cobra.Command{Use: "get", Short: "Get a resource by ID"}
	getCmd.AddCommand(
		&cobra.Command{
			Use: "todo <id>", Short: "Get todo by ID", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, _ := strconv.ParseInt(args[0], 10, 64)
				return runGetTodo(id, clihelp.OutputFormat(*outputFlag))
			},
		},
		&cobra.Command{
			Use: "decision <id>", Short: "Get decision by ID", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, _ := strconv.ParseInt(args[0], 10, 64)
				return runGetDecision(id, clihelp.OutputFormat(*outputFlag))
			},
		},
		&cobra.Command{
			Use: "investigation <id>", Short: "Get investigation by ID", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, _ := strconv.ParseInt(args[0], 10, 64)
				return runGetInvestigation(id, clihelp.OutputFormat(*outputFlag))
			},
		},
		&cobra.Command{
			Use: "session <id>", Short: "Get session by ID", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, _ := strconv.ParseInt(args[0], 10, 64)
				return runGetSession(id, clihelp.OutputFormat(*outputFlag))
			},
		},
	)
	root.AddCommand(getCmd)

	// --- List ---
	listCmd := &cobra.Command{Use: "list", Short: "List resources"}
	listTodosCmd := &cobra.Command{
		Use: "todos", Short: "List todos",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, _ := cmd.Flags().GetString("status")
			priority, _ := cmd.Flags().GetString("priority")
			limit, _ := cmd.Flags().GetInt("limit")
			return runListTodos(status, priority, limit, clihelp.OutputFormat(*outputFlag))
		},
	}
	listTodosCmd.Flags().String("status", "", "Filter by status")
	listTodosCmd.Flags().String("priority", "", "Filter by priority")
	listTodosCmd.Flags().Int("limit", 20, "Max results")

	listDecisionsCmd := &cobra.Command{
		Use: "decisions", Short: "List decisions",
		RunE: func(cmd *cobra.Command, args []string) error {
			category, _ := cmd.Flags().GetString("category")
			limit, _ := cmd.Flags().GetInt("limit")
			return runListDecisions(category, limit, clihelp.OutputFormat(*outputFlag))
		},
	}
	listDecisionsCmd.Flags().String("category", "", "Filter by category")
	listDecisionsCmd.Flags().Int("limit", 20, "Max results")

	listInvsCmd := &cobra.Command{
		Use: "investigations", Short: "List investigations",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, _ := cmd.Flags().GetString("status")
			limit, _ := cmd.Flags().GetInt("limit")
			return runListInvestigations(status, limit, clihelp.OutputFormat(*outputFlag))
		},
	}
	listInvsCmd.Flags().String("status", "", "Filter by status")
	listInvsCmd.Flags().Int("limit", 20, "Max results")

	listSessionsCmd := &cobra.Command{
		Use: "sessions", Short: "List sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			source, _ := cmd.Flags().GetString("source")
			limit, _ := cmd.Flags().GetInt("limit")
			return runListSessions(source, limit, clihelp.OutputFormat(*outputFlag))
		},
	}
	listSessionsCmd.Flags().String("source", "", "Filter by source")
	listSessionsCmd.Flags().Int("limit", 20, "Max results")

	listCmd.AddCommand(listTodosCmd, listDecisionsCmd, listInvsCmd, listSessionsCmd)
	root.AddCommand(listCmd)

	// --- Add ---
	addCmd := &cobra.Command{Use: "add", Short: "Add a todo or evidence"}
	addTodoCmd := &cobra.Command{
		Use: "todo <title>", Short: "Add a new todo", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			priority, _ := cmd.Flags().GetString("priority")
			deadline, _ := cmd.Flags().GetString("deadline")
			content, _ := cmd.Flags().GetString("content")
			entitiesStr, _ := cmd.Flags().GetString("related-entities")
			var entities []string
			if entitiesStr != "" {
				entities = strings.Split(entitiesStr, ",")
			}
			return runAddTodo(args[0], content, priority, deadline, entities)
		},
	}
	addTodoCmd.Flags().String("priority", "", "Priority: low|medium|high|urgent")
	addTodoCmd.Flags().String("deadline", "", "Deadline (ISO 8601)")
	addTodoCmd.Flags().String("content", "", "Detailed description")
	addTodoCmd.Flags().String("related-entities", "", "Related entities (comma-separated)")

	addEvidenceCmd := &cobra.Command{
		Use: "evidence <investigation-id> <evidence>", Short: "Add evidence to an investigation", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, _ := strconv.ParseInt(args[0], 10, 64)
			return runAddEvidence(id, args[1])
		},
	}

	addCmd.AddCommand(addTodoCmd, addEvidenceCmd)
	root.AddCommand(addCmd)

	// --- Complete ---
	completeCmd := &cobra.Command{Use: "complete", Short: "Complete a resource"}
	completeCmd.AddCommand(&cobra.Command{
		Use: "todo <id>", Short: "Mark a todo as done", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, _ := strconv.ParseInt(args[0], 10, 64)
			return runCompleteTodo(id)
		},
	})
	root.AddCommand(completeCmd)

	// --- Update ---
	updateCmd := &cobra.Command{Use: "update", Short: "Update a resource"}
	updateTodoCmd := &cobra.Command{
		Use: "todo <id>", Short: "Update a todo", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, _ := strconv.ParseInt(args[0], 10, 64)
			priority, _ := cmd.Flags().GetString("priority")
			content, _ := cmd.Flags().GetString("content")
			return runUpdateTodo(id, priority, content)
		},
	}
	updateTodoCmd.Flags().String("priority", "", "New priority")
	updateTodoCmd.Flags().String("content", "", "New content")
	updateCmd.AddCommand(updateTodoCmd)
	root.AddCommand(updateCmd)

	// --- Record ---
	recordCmd := &cobra.Command{Use: "record", Short: "Record a decision"}
	recordDecisionCmd := &cobra.Command{
		Use: "decision <title>", Short: "Record a new decision", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			choice, _ := cmd.Flags().GetString("choice")
			why, _ := cmd.Flags().GetString("why")
			category, _ := cmd.Flags().GetString("category")
			altsStr, _ := cmd.Flags().GetString("alternatives")
			var alts []string
			if altsStr != "" {
				alts = strings.Split(altsStr, ",")
			}
			return runRecordDecision(args[0], choice, why, category, alts)
		},
	}
	recordDecisionCmd.Flags().String("choice", "", "The decision (required)")
	recordDecisionCmd.MarkFlagRequired("choice")
	recordDecisionCmd.Flags().String("why", "", "Reasoning")
	recordDecisionCmd.Flags().String("category", "", "Category")
	recordDecisionCmd.Flags().String("alternatives", "", "Alternatives (comma-separated)")
	recordCmd.AddCommand(recordDecisionCmd)
	root.AddCommand(recordCmd)

	// --- Open ---
	openCmd := &cobra.Command{Use: "open", Short: "Open an investigation"}
	openInvCmd := &cobra.Command{
		Use: "investigation <name>", Short: "Open a new investigation", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hypothesis, _ := cmd.Flags().GetString("hypothesis")
			return runOpenInvestigation(args[0], hypothesis)
		},
	}
	openInvCmd.Flags().String("hypothesis", "", "Hypothesis (required)")
	openInvCmd.MarkFlagRequired("hypothesis")
	openCmd.AddCommand(openInvCmd)
	root.AddCommand(openCmd)

	// --- Close ---
	closeCmd := &cobra.Command{Use: "close", Short: "Close an investigation"}
	closeInvCmd := &cobra.Command{
		Use: "investigation <id>", Short: "Close an investigation", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, _ := strconv.ParseInt(args[0], 10, 64)
			conclusion, _ := cmd.Flags().GetString("conclusion")
			return runCloseInvestigation(id, conclusion)
		},
	}
	closeInvCmd.Flags().String("conclusion", "", "Conclusion (required)")
	closeInvCmd.MarkFlagRequired("conclusion")
	closeCmd.AddCommand(closeInvCmd)
	root.AddCommand(closeCmd)

	// --- Log ---
	logCmd := &cobra.Command{Use: "log", Short: "Log a session"}
	logSessionCmd := &cobra.Command{
		Use: "session <summary>", Short: "Log a session", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, _ := cmd.Flags().GetString("source")
			decisionsStr, _ := cmd.Flags().GetString("decisions")
			todosOpenStr, _ := cmd.Flags().GetString("todos-opened")
			return runLogSession(args[0], source, decisionsStr, todosOpenStr)
		},
	}
	logSessionCmd.Flags().String("source", "manual", "Session source")
	logSessionCmd.Flags().String("decisions", "", "Decision IDs (comma-separated)")
	logSessionCmd.Flags().String("todos-opened", "", "Todo IDs opened (comma-separated)")
	logCmd.AddCommand(logSessionCmd)
	root.AddCommand(logCmd)

	// --- Reembed ---
	reembedCmd := &cobra.Command{
		Use:   "reembed",
		Short: "Re-process all embeddings",
		RunE: func(cmd *cobra.Command, args []string) error {
			refType, _ := cmd.Flags().GetString("ref-type")
			return runReembed(refType)
		},
	}
	reembedCmd.Flags().String("ref-type", "", "Filter by type")
	root.AddCommand(reembedCmd)

	// --- Import-Cortex ---
	importCmd := &cobra.Command{
		Use:   "import-cortex",
		Short: "Import data from Cortex v2 database",
		RunE: func(cmd *cobra.Command, args []string) error {
			cortexDB, _ := cmd.Flags().GetString("cortex-db")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			apply, _ := cmd.Flags().GetBool("apply")
			if !dryRun && !apply {
				dryRun = true
			}
			return runImportCortex(cortexDB, dryRun)
		},
	}
	importCmd.Flags().String("cortex-db", "/tmp/cortex-migration-copy.db", "Path to Cortex v2 database copy")
	importCmd.Flags().StringVar(&cfg.DBPath, "db", "/opt/plan/data/plan.db", "PlanRod database path")
	importCmd.Flags().Bool("dry-run", false, "Show what would be imported")
	importCmd.Flags().Bool("apply", false, "Actually import")
	root.AddCommand(importCmd)

	return root
}

// --- Shared helpers ---

func openDB() (*sql.DB, error) {
	if cfg.DBPath == "" {
		cfg.DBPath = "/opt/plan/data/plan.db"
	}
	return hivestore.OpenDB(cfg.DBPath)
}

func initEmbedder() *membed.Manager {
	if cfg.OllamaURL == "" {
		cfg.OllamaURL = "http://localhost:11434"
	}
	if cfg.OllamaModel == "" {
		cfg.OllamaModel = "nomic-embed-text"
	}
	client, err := embeddings.NewOllamaClient(embeddings.OllamaConfig{
		BaseURL: cfg.OllamaURL,
		Model:   cfg.OllamaModel,
	})
	if err != nil {
		slog.Warn("embedder unavailable", "error", err)
		return membed.NewManager(nil)
	}
	return membed.NewManager(client)
}

// --- Command implementations ---

func runServe(ctx context.Context) error {
	logger := logging.NewLogger("planrod")

	db, err := openDB()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	migrator := &schema.Migrator{DB: db, MigrationsFS: migrationsFS, Logger: logger}
	if err := migrator.Apply(context.Background()); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	sessDir := cfg.SessionsDir
	if sessDir == "" {
		sessDir = "/opt/plan/sessions"
	}
	sessMgr := &sessions.Manager{Dir: sessDir}
	if err := sessMgr.Init(); err != nil {
		logger.Warn("sessions git init failed", "error", err)
	}

	em := initEmbedder()
	if em.IsAvailable() {
		logger.Info("embedder ready", "model_version", em.ModelVersion())
	} else {
		logger.Warn("embedder unavailable, running in FTS-only mode")
	}

	timeout, _ := time.ParseDuration(cfg.CrossServiceTimeout)
	if timeout == 0 {
		timeout = 500 * time.Millisecond
	}
	cs := crossservice.NewClient(cfg.CrossServiceMode, cfg.MemoryRodURL, timeout, logger)

	metricsReg := metrics.NewRegistry("planrod")
	st := store.New(db)
	eng := &search.Engine{
		DB:       db,
		Embedder: em,
		WFts:     cfg.RRFWeightFTS,
		WVec:     cfg.RRFWeightVec,
	}
	if eng.WFts == 0 {
		eng.WFts = 0.4
	}
	if eng.WVec == 0 {
		eng.WVec = 0.6
	}

	srv, err := server.New(cfg, st, eng, sessMgr, em, cs, metricsReg, logger)
	if err != nil {
		return err
	}

	sctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(sctx)
}

func runMigrate() error {
	logger := logging.NewLogger("planrod")
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	migrator := &schema.Migrator{DB: db, MigrationsFS: migrationsFS, Logger: logger}
	return migrator.Apply(context.Background())
}

func runHealthcheck() error {
	fmt.Println("Checking database...")
	db, err := openDB()
	if err != nil {
		fmt.Printf("DB: ERROR - %v\n", err)
		return err
	}
	defer db.Close()
	fmt.Println("DB: OK")

	fmt.Println("Checking embedder...")
	em := initEmbedder()
	if em.IsAvailable() {
		fmt.Printf("Embedder: OK (%s)\n", em.ModelVersion())
	} else {
		fmt.Println("Embedder: UNAVAILABLE")
	}

	sessDir := cfg.SessionsDir
	if sessDir == "" {
		sessDir = "/opt/plan/sessions"
	}
	sessMgr := &sessions.Manager{Dir: sessDir}
	fmt.Print("Sessions repo: ")
	if sessMgr.IsValid() {
		fmt.Println("OK")
	} else {
		fmt.Println("NOT INITIALIZED")
	}
	return nil
}

func runSearch(query, mode string, types []string, limit int, format clihelp.OutputFormat) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	em := initEmbedder()
	eng := &search.Engine{DB: db, Embedder: em, WFts: 0.4, WVec: 0.6}

	results, meta, err := eng.Search(context.Background(), query, search.SearchOpts{
		Mode: mode, Limit: limit, Types: types,
	})
	if err != nil {
		return err
	}

	if meta.Degraded {
		fmt.Fprintf(os.Stderr, "⚠ Degraded: %s (mode=%s)\n", meta.Reason, meta.Mode)
	}

	clihelp.Print(results, format)
	return nil
}

func runGetTodo(id int64, format clihelp.OutputFormat) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)
	todo, err := st.GetTodo(context.Background(), id)
	if err != nil {
		return err
	}
	history, _ := st.GetTodoHistory(context.Background(), id)
	clihelp.Print(map[string]interface{}{"todo": todo, "history": history}, format)
	return nil
}

func runGetDecision(id int64, format clihelp.OutputFormat) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)
	d, err := st.GetDecision(context.Background(), id)
	if err != nil {
		return err
	}
	clihelp.Print(d, format)
	return nil
}

func runGetInvestigation(id int64, format clihelp.OutputFormat) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)
	inv, err := st.GetInvestigation(context.Background(), id)
	if err != nil {
		return err
	}
	history, _ := st.GetInvestigationHistory(context.Background(), id)
	clihelp.Print(map[string]interface{}{"investigation": inv, "history": history}, format)
	return nil
}

func runGetSession(id int64, format clihelp.OutputFormat) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)
	sess, err := st.GetSession(context.Background(), id)
	if err != nil {
		return err
	}
	if sess.SummaryPath != "" {
		sessDir := cfg.SessionsDir
		if sessDir == "" {
			sessDir = "/opt/plan/sessions"
		}
		mgr := &sessions.Manager{Dir: sessDir}
		if full, err := mgr.ReadSummary(sess.SummaryPath); err == nil {
			sess.Summary = full
		}
	}
	clihelp.Print(sess, format)
	return nil
}

func runListTodos(status, priority string, limit int, format clihelp.OutputFormat) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)
	todos, err := st.ListTodos(context.Background(), status, priority, "", 0, limit)
	if err != nil {
		return err
	}
	clihelp.Print(todos, format)
	return nil
}

func runListDecisions(category string, limit int, format clihelp.OutputFormat) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)
	decisions, err := st.SearchDecisions(context.Background(), category, "", "", limit)
	if err != nil {
		return err
	}
	clihelp.Print(decisions, format)
	return nil
}

func runListInvestigations(status string, limit int, format clihelp.OutputFormat) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)
	investigations, err := st.ListInvestigations(context.Background(), status, "", limit)
	if err != nil {
		return err
	}
	clihelp.Print(investigations, format)
	return nil
}

func runListSessions(source string, limit int, format clihelp.OutputFormat) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)
	sessions, err := st.ListSessions(context.Background(), source, limit)
	if err != nil {
		return err
	}
	clihelp.Print(sessions, format)
	return nil
}

func runAddTodo(title, content, priority, deadline string, entities []string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)

	t := &types.Todo{
		Title:           title,
		Content:         content,
		Priority:        priority,
		RelatedEntities: entities,
	}
	if deadline != "" {
		parsed, err := time.Parse(time.RFC3339, deadline)
		if err != nil {
			parsed, err = time.Parse("2006-01-02", deadline)
		}
		if err == nil {
			t.Deadline = &parsed
		}
	}

	created, err := st.CreateTodo(context.Background(), t)
	if err != nil {
		return err
	}
	fmt.Printf("Created todo #%d: %s\n", created.ID, created.Title)
	return nil
}

func runCompleteTodo(id int64) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)
	todo, err := st.CompleteTodo(context.Background(), id)
	if err != nil {
		return err
	}
	fmt.Printf("Completed todo #%d: %s\n", todo.ID, todo.Title)
	return nil
}

func runUpdateTodo(id int64, priority, content string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)

	var p, c *string
	if priority != "" {
		p = &priority
	}
	if content != "" {
		c = &content
	}

	updated, err := st.UpdateTodo(context.Background(), id, nil, c, p, nil, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Updated todo #%d: %s\n", updated.ID, updated.Title)
	return nil
}

func runRecordDecision(title, choice, why, category string, alternatives []string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)

	d := &types.Decision{
		Title:        title,
		Choice:       choice,
		Why:          why,
		Category:     category,
		Alternatives: alternatives,
	}

	created, err := st.RecordDecision(context.Background(), d)
	if err != nil {
		return err
	}
	fmt.Printf("Recorded decision #%d: %s\n", created.ID, created.Title)
	return nil
}

func runOpenInvestigation(name, hypothesis string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)

	inv := &types.Investigation{
		Name:       name,
		Hypothesis: hypothesis,
	}

	created, err := st.OpenInvestigation(context.Background(), inv)
	if err != nil {
		return err
	}
	fmt.Printf("Opened investigation #%d: %s\n", created.ID, created.Name)
	return nil
}

func runAddEvidence(id int64, evidence string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)

	updated, err := st.UpdateInvestigation(context.Background(), id, evidence, "")
	if err != nil {
		return err
	}
	fmt.Printf("Added evidence to investigation #%d: %s\n", updated.ID, updated.Name)
	return nil
}

func runCloseInvestigation(id int64, conclusion string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)

	closed, err := st.CloseInvestigation(context.Background(), id, conclusion, "closed")
	if err != nil {
		return err
	}
	fmt.Printf("Closed investigation #%d: %s\n", closed.ID, closed.Name)
	return nil
}

func runLogSession(summary, source, decisionsStr, todosOpenStr string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db)

	sess := &types.Session{
		Summary: summary,
		Source:  source,
	}

	if decisionsStr != "" {
		for _, s := range strings.Split(decisionsStr, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				sess.DecisionsMade = append(sess.DecisionsMade, id)
			}
		}
	}
	if todosOpenStr != "" {
		for _, s := range strings.Split(todosOpenStr, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				sess.TodosOpened = append(sess.TodosOpened, id)
			}
		}
	}

	created, err := st.LogSession(context.Background(), sess)
	if err != nil {
		return err
	}
	fmt.Printf("Logged session #%d\n", created.ID)
	return nil
}

func runReembed(refType string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	em := initEmbedder()
	if !em.IsAvailable() {
		return fmt.Errorf("embedder unavailable, cannot reembed")
	}

	st := store.New(db)
	ctx := context.Background()
	var count int

	reembedOne := func(rType string, id int64, text string) {
		vec, err := em.Embed(ctx, text)
		if err != nil {
			fmt.Printf("  SKIP %s#%d: %v\n", rType, id, err)
			return
		}
		vecBytes := serializeFloat32CLI(vec)

		tx, err := db.Begin()
		if err != nil {
			return
		}
		defer tx.Rollback()

		var existingRowid int64
		err = tx.QueryRow("SELECT rowid FROM embeddings_meta WHERE ref_type = ? AND ref_id = ?", rType, id).Scan(&existingRowid)
		if err == nil {
			tx.Exec("UPDATE vec_embeddings SET embedding = ? WHERE rowid = ?", vecBytes, existingRowid)
			tx.Exec("UPDATE embeddings_meta SET model_version = ?, created_at = CURRENT_TIMESTAMP WHERE rowid = ?", em.ModelVersion(), existingRowid)
		} else {
			res, err := tx.Exec("INSERT INTO vec_embeddings(embedding) VALUES (?)", vecBytes)
			if err != nil {
				return
			}
			rowid, _ := res.LastInsertId()
			tx.Exec("INSERT INTO embeddings_meta (rowid, ref_type, ref_id, model_version) VALUES (?, ?, ?, ?)",
				rowid, rType, id, em.ModelVersion())
		}
		tx.Commit()
		count++
	}

	if refType == "" || refType == "todo" {
		todos, _ := st.ListTodos(ctx, "", "", "", 0, 1000)
		for _, t := range todos {
			reembedOne("todo", t.ID, t.Title+" "+t.Content)
		}
	}
	if refType == "" || refType == "decision" {
		decisions, _ := st.SearchDecisions(ctx, "", "", "", 1000)
		for _, d := range decisions {
			reembedOne("decision", d.ID, d.Title+" "+d.Choice+" "+d.Why)
		}
	}
	if refType == "" || refType == "investigation" {
		invs, _ := st.ListInvestigations(ctx, "", "", 1000)
		for _, inv := range invs {
			reembedOne("investigation", inv.ID, inv.Name+" "+inv.Hypothesis)
		}
	}
	if refType == "" || refType == "session" {
		sessions, _ := st.ListSessions(ctx, "", 1000)
		for _, sess := range sessions {
			reembedOne("session", sess.ID, sess.Title+" "+sess.Summary)
		}
	}

	fmt.Printf("Re-embedded %d items\n", count)
	return nil
}

func runImportCortex(cortexDBPath string, dryRun bool) error {
	if cfg.DBPath == "" {
		cfg.DBPath = "/opt/plan/data/plan.db"
	}

	// Open Cortex DB
	cortexDB, err := sql.Open("sqlite3", cortexDBPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open cortex db: %w", err)
	}
	defer cortexDB.Close()

	// Open PlanRod DB
	planDB, err := openDB()
	if err != nil {
		return fmt.Errorf("open plan db: %w", err)
	}
	defer planDB.Close()

	// Run migrations first
	logger := logging.NewLogger("planrod")
	migrator := &schema.Migrator{DB: planDB, MigrationsFS: migrationsFS, Logger: logger}
	if err := migrator.Apply(context.Background()); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	st := store.New(planDB)
	ctx := context.Background()

	var stats struct {
		todos, decisions, investigations, sessions, orphans int
		orphanList                                          []string
	}

	// Import todos from notes with category='todo'
	rows, err := cortexDB.QueryContext(ctx,
		"SELECT COALESCE(content,'') FROM notes WHERE category = 'todo' ORDER BY id")
	if err == nil {
		for rows.Next() {
			var content string
			rows.Scan(&content)
			title := firstLine(content)
			if title == "" {
				continue
			}
			stats.todos++
			if !dryRun {
				st.CreateTodo(ctx, &types.Todo{Title: title, Content: content, Status: "pending"})
			}
		}
		rows.Close()
	}

	// Import decisions from notes with category='decision'
	rows, err = cortexDB.QueryContext(ctx,
		"SELECT COALESCE(content,'') FROM notes WHERE category = 'decision' ORDER BY id")
	if err == nil {
		for rows.Next() {
			var content string
			rows.Scan(&content)
			title := firstLine(content)
			if title == "" {
				continue
			}
			stats.decisions++
			if !dryRun {
				st.RecordDecision(ctx, &types.Decision{Title: title, Choice: content})
			}
		}
		rows.Close()
	}

	// Import investigations from notes with category='investigation'
	rows, err = cortexDB.QueryContext(ctx,
		"SELECT COALESCE(content,'') FROM notes WHERE category = 'investigation' ORDER BY id")
	if err == nil {
		for rows.Next() {
			var content string
			rows.Scan(&content)
			name := firstLine(content)
			if name == "" {
				continue
			}
			stats.investigations++
			if !dryRun {
				st.OpenInvestigation(ctx, &types.Investigation{Name: name, Hypothesis: content})
			}
		}
		rows.Close()
	}

	// Import sessions from notes with category='session'
	rows, err = cortexDB.QueryContext(ctx,
		"SELECT COALESCE(content,'') FROM notes WHERE category = 'session' ORDER BY id")
	if err == nil {
		for rows.Next() {
			var content string
			rows.Scan(&content)
			if content == "" {
				continue
			}
			stats.sessions++
			if !dryRun {
				st.LogSession(ctx, &types.Session{Summary: content, Source: "import"})
			}
		}
		rows.Close()
	}

	// Check for orphans (categories not matching the above)
	rows, err = cortexDB.QueryContext(ctx,
		"SELECT COALESCE(category,''), COALESCE(content,'') FROM notes WHERE category NOT IN ('todo','decision','investigation','session','spec','spec-completion') ORDER BY id")
	if err == nil {
		for rows.Next() {
			var cat, content string
			rows.Scan(&cat, &content)
			stats.orphans++
			firstLine := firstLine(content)
			stats.orphanList = append(stats.orphanList, fmt.Sprintf("[%s] %s", cat, firstLine))
		}
		rows.Close()
	}

	// Import from dedicated Cortex tables
	// Todos table
	rows, err = cortexDB.QueryContext(ctx, "SELECT COALESCE(title,''), COALESCE(priority,''), COALESCE(status,'pending') FROM todos ORDER BY id")
	if err == nil {
		for rows.Next() {
			var title, priority, status string
			rows.Scan(&title, &priority, &status)
			if title == "" {
				continue
			}
			stats.todos++
			if !dryRun {
				st.CreateTodo(ctx, &types.Todo{Title: title, Priority: priority, Status: status})
			}
		}
		rows.Close()
	}

	// Decisions table
	rows, err = cortexDB.QueryContext(ctx, "SELECT COALESCE(title,''), COALESCE(choice,''), COALESCE(why,''), COALESCE(alternatives,''), COALESCE(status,'active') FROM decisions ORDER BY id")
	if err == nil {
		for rows.Next() {
			var title, choice, why, alts, status string
			rows.Scan(&title, &choice, &why, &alts, &status)
			if title == "" {
				continue
			}
			_ = status // Cortex decisions have status but PlanRod decisions are append-only
			d := &types.Decision{Title: title, Choice: choice, Why: why}
			if alts != "" {
				d.Alternatives = unmarshalStringsImport(alts)
			}
			stats.decisions++
			if !dryRun {
				st.RecordDecision(ctx, d)
			}
		}
		rows.Close()
	}

	// Investigations table
	rows, err = cortexDB.QueryContext(ctx, "SELECT COALESCE(name,''), COALESCE(hypothesis,''), COALESCE(status,'open'), COALESCE(evidence,''), COALESCE(conclusion,'') FROM investigations ORDER BY id")
	if err == nil {
		for rows.Next() {
			var name, hypothesis, status, evidence, conclusion string
			rows.Scan(&name, &hypothesis, &status, &evidence, &conclusion)
			if name == "" {
				continue
			}
			stats.investigations++
			if !dryRun {
				inv, err := st.OpenInvestigation(ctx, &types.Investigation{Name: name, Hypothesis: hypothesis})
				if err != nil {
					continue
				}
				if evidence != "" {
					st.UpdateInvestigation(ctx, inv.ID, evidence, "")
				}
				if status == "closed" || status == "abandoned" {
					st.CloseInvestigation(ctx, inv.ID, conclusion, status)
				} else if status != "open" {
					st.UpdateInvestigation(ctx, inv.ID, "", status)
				}
			}
		}
		rows.Close()
	}

	// Sessions table
	rows, err = cortexDB.QueryContext(ctx, "SELECT COALESCE(title,''), COALESCE(summary,''), COALESCE(source,'') FROM sessions ORDER BY created_at")
	if err == nil {
		for rows.Next() {
			var title, summary, source string
			rows.Scan(&title, &summary, &source)
			if summary == "" {
				continue
			}
			stats.sessions++
			if !dryRun {
				st.LogSession(ctx, &types.Session{Title: title, Summary: summary, Source: source})
			}
		}
		rows.Close()
	}

	// Generate embeddings if applying
	if !dryRun {
		em := initEmbedder()
		if em.IsAvailable() {
			fmt.Println("Generating embeddings...")
			// Trigger reembed for all items
			runReembed("")
		} else {
			fmt.Println("⚠ Embedder unavailable, skipping embedding generation")
		}
	}

	// Generate report
	report := fmt.Sprintf(`# PlanRod Import Report

**Date:** %s
**Source:** %s
**Mode:** %s

## Statistics

| Type | Count |
|------|-------|
| Todos | %d |
| Decisions | %d |
| Investigations | %d |
| Sessions | %d |
| Orphans | %d |

## Orphans (not imported)
`, time.Now().Format(time.RFC3339), cortexDBPath, modeStr(dryRun),
		stats.todos, stats.decisions, stats.investigations, stats.sessions, stats.orphans)

	for _, o := range stats.orphanList {
		report += fmt.Sprintf("- %s\n", o)
	}

	if dryRun {
		fmt.Println("=== DRY RUN ===")
		fmt.Println(report)
	} else {
		if err := os.WriteFile("/opt/plan/import-report.md", []byte(report), 0644); err != nil {
			fmt.Printf("Warning: could not write import report: %v\n", err)
		} else {
			fmt.Println("Import report written to /opt/plan/import-report.md")
		}
	}

	fmt.Printf("Todos: %d, Decisions: %d, Investigations: %d, Sessions: %d, Orphans: %d\n",
		stats.todos, stats.decisions, stats.investigations, stats.sessions, stats.orphans)

	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimPrefix(s, "# ")
	s = strings.TrimPrefix(s, "## ")
	if len(s) > 200 {
		s = s[:200]
	}
	return strings.TrimSpace(s)
}

func unmarshalStringsImport(s string) []string {
	if s == "" || s == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		// Try as comma-separated
		for _, p := range strings.Split(s, ",") {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

func modeStr(dryRun bool) string {
	if dryRun {
		return "dry-run"
	}
	return "apply"
}

func serializeFloat32CLI(v []float32) []byte {
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
