package search

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/rolivape/planrod/internal/embed"
	"github.com/rolivape/planrod/internal/types"
)

type Engine struct {
	DB       *sql.DB
	Embedder *embed.Manager
	WFts     float64
	WVec     float64
}

type SearchOpts struct {
	Mode  string   // "hybrid", "fts", "vec"
	Limit int
	Types []string // filter: "todo", "decision", "investigation", "session"
}

type SearchMeta struct {
	Mode     string
	Degraded bool
	Reason   string
}

func (e *Engine) Search(ctx context.Context, query string, opts SearchOpts) ([]types.SearchResult, SearchMeta, error) {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	if opts.Limit > 50 {
		opts.Limit = 50
	}
	if opts.Mode == "" {
		opts.Mode = "hybrid"
	}

	meta := SearchMeta{Mode: opts.Mode}

	switch opts.Mode {
	case "fts":
		results, err := e.searchFTS(ctx, query, opts)
		return results, meta, err

	case "vec":
		if !e.Embedder.IsAvailable() {
			remaining := e.Embedder.CooldownRemaining()
			return nil, meta, fmt.Errorf("embedder unavailable, retry in %d seconds or use mode=fts", remaining)
		}
		results, err := e.searchVec(ctx, query, opts)
		return results, meta, err

	case "hybrid":
		ftsResults, err := e.searchFTS(ctx, query, opts)
		if err != nil {
			return nil, meta, err
		}

		if !e.Embedder.IsAvailable() {
			meta.Degraded = true
			meta.Reason = "embedder_unavailable"
			meta.Mode = "fts"
			return ftsResults, meta, nil
		}

		vecResults, err := e.searchVec(ctx, query, opts)
		if err != nil {
			meta.Degraded = true
			meta.Reason = "embedder_unavailable"
			meta.Mode = "fts"
			return ftsResults, meta, nil
		}

		ftsRanked := make([]rankedItem, len(ftsResults))
		for i, r := range ftsResults {
			ftsRanked[i] = rankedItem{RefType: r.RefType, RefID: r.RefID, Rank: i + 1}
		}
		vecRanked := make([]rankedItem, len(vecResults))
		for i, r := range vecResults {
			vecRanked[i] = rankedItem{RefType: r.RefType, RefID: r.RefID, Rank: i + 1}
		}

		scored := RRF(ftsRanked, vecRanked, e.WFts, e.WVec, opts.Limit)

		resultMap := make(map[string]types.SearchResult)
		for _, r := range ftsResults {
			resultMap[fmt.Sprintf("%s:%d", r.RefType, r.RefID)] = r
		}
		for _, r := range vecResults {
			key := fmt.Sprintf("%s:%d", r.RefType, r.RefID)
			if _, exists := resultMap[key]; !exists {
				resultMap[key] = r
			}
		}

		var merged []types.SearchResult
		for _, s := range scored {
			key := fmt.Sprintf("%s:%d", s.RefType, s.RefID)
			if r, ok := resultMap[key]; ok {
				r.Score = s.Score
				merged = append(merged, r)
			}
		}
		return merged, meta, nil

	default:
		return nil, meta, fmt.Errorf("unknown search mode %q", opts.Mode)
	}
}

func (e *Engine) typeAllowed(t string, types []string) bool {
	if len(types) == 0 {
		return true
	}
	for _, allowed := range types {
		if allowed == t {
			return true
		}
	}
	return false
}

func (e *Engine) searchFTS(ctx context.Context, query string, opts SearchOpts) ([]types.SearchResult, error) {
	ftsQuery := escapeFTS(query)
	var results []types.SearchResult

	if e.typeAllowed("todo", opts.Types) {
		rows, err := e.DB.QueryContext(ctx, `
			SELECT 'todo' AS ref_type, t.id, t.title, COALESCE(t.content,''), bm25(fts_todos)
			FROM fts_todos
			JOIN todos t ON t.id = fts_todos.rowid
			WHERE fts_todos MATCH ?
			ORDER BY bm25(fts_todos)
			LIMIT ?`, ftsQuery, opts.Limit)
		if err == nil {
			for rows.Next() {
				var r types.SearchResult
				rows.Scan(&r.RefType, &r.RefID, &r.NameOrTitle, &r.Snippet, &r.Score)
				results = append(results, r)
			}
			rows.Close()
		}
	}

	if e.typeAllowed("decision", opts.Types) {
		rows, err := e.DB.QueryContext(ctx, `
			SELECT 'decision' AS ref_type, d.id, d.title, d.choice, bm25(fts_decisions)
			FROM fts_decisions
			JOIN decisions d ON d.id = fts_decisions.rowid
			WHERE fts_decisions MATCH ?
			ORDER BY bm25(fts_decisions)
			LIMIT ?`, ftsQuery, opts.Limit)
		if err == nil {
			for rows.Next() {
				var r types.SearchResult
				rows.Scan(&r.RefType, &r.RefID, &r.NameOrTitle, &r.Snippet, &r.Score)
				results = append(results, r)
			}
			rows.Close()
		}
	}

	if e.typeAllowed("investigation", opts.Types) {
		rows, err := e.DB.QueryContext(ctx, `
			SELECT 'investigation' AS ref_type, i.id, i.name, i.hypothesis, bm25(fts_investigations)
			FROM fts_investigations
			JOIN investigations i ON i.id = fts_investigations.rowid
			WHERE fts_investigations MATCH ?
			ORDER BY bm25(fts_investigations)
			LIMIT ?`, ftsQuery, opts.Limit)
		if err == nil {
			for rows.Next() {
				var r types.SearchResult
				rows.Scan(&r.RefType, &r.RefID, &r.NameOrTitle, &r.Snippet, &r.Score)
				results = append(results, r)
			}
			rows.Close()
		}
	}

	if e.typeAllowed("session", opts.Types) {
		rows, err := e.DB.QueryContext(ctx, `
			SELECT 'session' AS ref_type, s.id, COALESCE(s.title,''), substr(s.summary, 1, 200), bm25(fts_sessions)
			FROM fts_sessions
			JOIN sessions s ON s.id = fts_sessions.rowid
			WHERE fts_sessions MATCH ?
			ORDER BY bm25(fts_sessions)
			LIMIT ?`, ftsQuery, opts.Limit)
		if err == nil {
			for rows.Next() {
				var r types.SearchResult
				rows.Scan(&r.RefType, &r.RefID, &r.NameOrTitle, &r.Snippet, &r.Score)
				results = append(results, r)
			}
			rows.Close()
		}
	}

	return results, nil
}

func (e *Engine) searchVec(ctx context.Context, query string, opts SearchOpts) ([]types.SearchResult, error) {
	queryVec, err := e.Embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	vecBytes := serializeFloat32(queryVec)
	k := opts.Limit
	if k < 50 {
		k = 50
	}

	rows, err := e.DB.QueryContext(ctx, `
		SELECT em.ref_type, em.ref_id, ve.distance
		FROM vec_embeddings ve
		JOIN embeddings_meta em ON em.rowid = ve.rowid
		WHERE ve.embedding MATCH ? AND ve.k = ?
		ORDER BY ve.distance`, vecBytes, k)
	if err != nil {
		return nil, fmt.Errorf("vec search: %w", err)
	}

	type rawResult struct {
		RefType  string
		RefID    int64
		Distance float64
	}
	var raw []rawResult
	for rows.Next() {
		var rr rawResult
		rows.Scan(&rr.RefType, &rr.RefID, &rr.Distance)
		if !e.typeAllowed(rr.RefType, opts.Types) {
			continue
		}
		raw = append(raw, rr)
	}
	rows.Close()

	var results []types.SearchResult
	for _, rr := range raw {
		var r types.SearchResult
		r.RefType = rr.RefType
		r.RefID = rr.RefID
		r.Score = 1.0 - rr.Distance
		r.NameOrTitle, r.Snippet = e.resolveItem(ctx, r.RefType, r.RefID)
		results = append(results, r)
		if len(results) >= opts.Limit {
			break
		}
	}
	return results, nil
}

func (e *Engine) resolveItem(ctx context.Context, refType string, refID int64) (name, snippet string) {
	switch refType {
	case "todo":
		e.DB.QueryRowContext(ctx, "SELECT title, COALESCE(content,'') FROM todos WHERE id = ?", refID).Scan(&name, &snippet)
	case "decision":
		e.DB.QueryRowContext(ctx, "SELECT title, choice FROM decisions WHERE id = ?", refID).Scan(&name, &snippet)
	case "investigation":
		e.DB.QueryRowContext(ctx, "SELECT name, hypothesis FROM investigations WHERE id = ?", refID).Scan(&name, &snippet)
	case "session":
		e.DB.QueryRowContext(ctx, "SELECT COALESCE(title,''), substr(summary, 1, 200) FROM sessions WHERE id = ?", refID).Scan(&name, &snippet)
	}
	return
}

func escapeFTS(query string) string {
	words := strings.Fields(query)
	if len(words) == 0 {
		return "\"\""
	}
	escaped := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.NewReplacer("\"", "", "(", "", ")", "", "*", "", ":", "").Replace(w)
		if w != "" {
			escaped = append(escaped, "\""+w+"\"")
		}
	}
	return strings.Join(escaped, " OR ")
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
