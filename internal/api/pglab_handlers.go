package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/indexlab/indexlab/internal/planexplainer"
	"github.com/indexlab/indexlab/internal/postgreslab"
	"github.com/jackc/pgx/v5/pgconn"
)

// PostgresLabService wires postgreslab.Lab + planexplainer behind HTTP routes.
type PostgresLabService struct {
	lab *postgreslab.Lab
}

const (
	labCodeUnavailable      = "PGLAB_UNAVAILABLE"
	labCodeBadRequest       = "PGLAB_BAD_REQUEST"
	labCodeReadOnly         = "PGLAB_READ_ONLY"
	labCodeDBError          = "PGLAB_DB_ERROR"
	labCodeMethodNotAllowed = "PGLAB_METHOD_NOT_ALLOWED"
)

type labError struct {
	Error  string `json:"error"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
	Action string `json:"action,omitempty"`
}

// NewPostgresLabService constructs a service that delegates to the given Lab.
func NewPostgresLabService(lab *postgreslab.Lab) *PostgresLabService {
	return &PostgresLabService{lab: lab}
}

// Register attaches Postgres lab routes to the mux.
func (s *PostgresLabService) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/pglab/setup", s.handleSetup)
	mux.HandleFunc("/api/pglab/seed", s.handleSeed)
	mux.HandleFunc("/api/pglab/status", s.handleStatus)
	mux.HandleFunc("/api/pglab/query", s.handleQuery)
	mux.HandleFunc("/api/pglab/explain", s.handleExplain)
	mux.HandleFunc("/api/pglab/indexes", s.handleIndexes)
	mux.HandleFunc("/api/pglab/index", s.handleIndex)
	mux.HandleFunc("/api/pglab/compare", s.handleCompare)
	mux.HandleFunc("/api/pglab/recommend", s.handleRecommend)
}

func (s *PostgresLabService) handleSetup(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.lab.Setup(ctx); err != nil {
		writeLabErrorFrom(w, "setup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *PostgresLabService) handleSeed(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Rows     int  `json:"rows"`
		Truncate bool `json:"truncate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeLabError(w, http.StatusBadRequest, labCodeBadRequest, "invalid request body", "send valid JSON: {rows?: number, truncate?: boolean}", "Check body schema and retry.")
		return
	}
	if body.Rows <= 0 {
		body.Rows = 100000
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := s.lab.Seed(ctx, body.Rows, body.Truncate); err != nil {
		writeLabErrorFrom(w, "seed failed", err)
		return
	}
	count, _ := s.lab.RowCount(ctx)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "rows": count})
}

func (s *PostgresLabService) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	exists, err := s.lab.DemoTableExists(ctx)
	if err != nil {
		writeLabErrorFrom(w, "status check failed", err)
		return
	}
	if !exists {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"configured": true,
			"connected":  true,
			"ready":      false,
			"reason":     "users_demo table is not present",
			"nextAction": "setup",
			"rows":       0,
			"indexes":    []postgreslab.IndexInfo{},
		})
		return
	}

	count, err := s.lab.RowCount(ctx)
	if err != nil {
		writeLabErrorFrom(w, "status check failed", err)
		return
	}
	indexes, err := s.lab.ListIndexes(ctx, "users_demo")
	if err != nil {
		writeLabErrorFrom(w, "status check failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured": true,
		"connected":  true,
		"ready":      true,
		"rows":       count,
		"indexes":    indexes,
	})
}

func (s *PostgresLabService) handleQuery(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeLabError(w, http.StatusBadRequest, labCodeBadRequest, "invalid request body", "send valid JSON: {sql}", "Check body schema and retry.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, err := s.lab.RunQuery(ctx, body.SQL)
	if err != nil {
		writeLabErrorFrom(w, "query failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *PostgresLabService) handleExplain(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeLabError(w, http.StatusBadRequest, labCodeBadRequest, "invalid request body", "send valid JSON: {sql}", "Check body schema and retry.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	raw, err := s.lab.ExplainAnalyze(ctx, body.SQL)
	if err != nil {
		writeLabErrorFrom(w, "explain failed", err)
		return
	}
	report, err := planexplainer.Parse(raw)
	if err != nil {
		writeLabError(w, http.StatusInternalServerError, labCodeDBError, "parse explain output failed", err.Error(), "Retry with a valid EXPLAIN-compatible SQL query.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"raw":    raw,
		"report": report,
	})
}

func (s *PostgresLabService) handleIndexes(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	indexes, err := s.lab.ListIndexes(ctx, "users_demo")
	if err != nil {
		writeLabErrorFrom(w, "list indexes failed", err)
		return
	}
	writeJSON(w, http.StatusOK, indexes)
}

func (s *PostgresLabService) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	switch r.Method {
	case http.MethodPost:
		var spec postgreslab.IndexSpec
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			writeLabError(w, http.StatusBadRequest, labCodeBadRequest, "invalid request body", "send valid JSON with name/columns", "Check request payload.")
			return
		}
		if err := s.lab.CreateIndex(ctx, spec); err != nil {
			writeLabErrorFrom(w, "create index failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case http.MethodDelete:
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeLabError(w, http.StatusBadRequest, labCodeBadRequest, "missing name parameter", "provide the query parameter name", "Pass name=<index-name> in the query string.")
			return
		}
		if err := s.lab.DropIndex(ctx, name); err != nil {
			writeLabErrorFrom(w, "drop index failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeLabError(w, http.StatusMethodNotAllowed, labCodeMethodNotAllowed, "method not allowed", "only POST or DELETE is allowed", "Use POST for create or DELETE with name query param.")
	}
}

func (s *PostgresLabService) handleCompare(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body struct {
		SQL   string                `json:"sql"`
		Index postgreslab.IndexSpec `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeLabError(w, http.StatusBadRequest, labCodeBadRequest, "invalid request body", "send valid JSON with sql and index", "Check body schema and retry.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	before, after, err := s.lab.CompareExplain(ctx, body.SQL, body.Index)
	if err != nil {
		writeLabErrorFrom(w, "compare failed", err)
		return
	}
	beforeReport, err := planexplainer.Parse(before)
	if err != nil {
		writeLabError(w, http.StatusInternalServerError, labCodeDBError, "parse explain output failed", err.Error(), "Retry compare with a valid SQL query.")
		return
	}
	afterReport, err := planexplainer.Parse(after)
	if err != nil {
		writeLabError(w, http.StatusInternalServerError, labCodeDBError, "parse explain output failed", err.Error(), "Retry compare with a valid SQL query.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"before":    beforeReport,
		"after":     afterReport,
		"summary":   planexplainer.CompareReports(beforeReport, afterReport),
		"rawBefore": before,
		"rawAfter":  after,
	})
}

func (s *PostgresLabService) handleRecommend(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeLabError(w, http.StatusBadRequest, labCodeBadRequest, "invalid request body", "send valid JSON: {sql}", "Check body schema and retry.")
		return
	}
	recs := planexplainer.RecommendIndexes(body.SQL)
	writeJSON(w, http.StatusOK, map[string]interface{}{"recommendations": recs})
}

func requireMethod(w http.ResponseWriter, r *http.Request, expected string) bool {
	if r.Method != expected {
		writeLabError(w, http.StatusMethodNotAllowed, labCodeMethodNotAllowed, "method not allowed", "only "+expected+" is allowed", "Use the documented HTTP method.")
		return false
	}
	return true
}

func writeLabError(w http.ResponseWriter, status int, code string, errorMsg string, reason string, action string) {
	resp := labError{
		Error:  errorMsg,
		Code:   code,
		Reason: reason,
	}
	if action != "" {
		resp.Action = action
	}
	writeJSON(w, status, resp)
}

func writeLabErrorFrom(w http.ResponseWriter, errorMsg string, err error) {
	status, code, action := classifyLabError(err)
	reason := err.Error()
	if code == labCodeReadOnly {
		action = "Use SELECT, WITH, or EXPLAIN queries only."
	}
	writeLabError(w, status, code, errorMsg, reason, action)
}

func classifyLabError(err error) (status int, code string, action string) {
	if errors.Is(err, postgreslab.ErrReadOnlyQuery) {
		return http.StatusBadRequest, labCodeReadOnly, "Move write operations to /api/pglab/setup, /api/pglab/seed, or /api/pglab/index."
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		codeText := string(pgErr.Code)
		switch {
		case strings.HasPrefix(codeText, "08"):
			return http.StatusServiceUnavailable, labCodeUnavailable, "Check DSN and PostgreSQL availability."
		case strings.HasPrefix(codeText, "57"):
			return http.StatusServiceUnavailable, labCodeUnavailable, "Retry after PostgreSQL service recovers."
		case strings.HasPrefix(codeText, "42") || strings.HasPrefix(codeText, "0A") || strings.HasPrefix(codeText, "22"):
			return http.StatusBadRequest, labCodeBadRequest, "Fix SQL or request payload."
		default:
			return http.StatusInternalServerError, labCodeDBError, "Check PostgreSQL runtime."
		}
	}

	return http.StatusInternalServerError, labCodeDBError, "Check PostgreSQL runtime and request payload."
}
