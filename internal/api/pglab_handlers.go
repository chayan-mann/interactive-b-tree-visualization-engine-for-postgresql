package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/indexlab/indexlab/internal/planexplainer"
	"github.com/indexlab/indexlab/internal/postgreslab"
)

// PostgresLabService wires postgreslab.Lab + planexplainer behind HTTP routes.
type PostgresLabService struct {
	lab *postgreslab.Lab
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
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.lab.Setup(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *PostgresLabService) handleSeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Rows     int  `json:"rows"`
		Truncate bool `json:"truncate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Rows <= 0 {
		body.Rows = 100000
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := s.lab.Seed(ctx, body.Rows, body.Truncate); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	count, _ := s.lab.RowCount(ctx)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "rows": count})
}

func (s *PostgresLabService) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	count, err := s.lab.RowCount(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	indexes, err := s.lab.ListIndexes(ctx, "users_demo")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rows":    count,
		"indexes": indexes,
	})
}

func (s *PostgresLabService) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, err := s.lab.RunQuery(ctx, body.SQL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *PostgresLabService) handleExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	raw, err := s.lab.ExplainAnalyze(ctx, body.SQL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	report, err := planexplainer.Parse(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"raw":    raw,
		"report": report,
	})
}

func (s *PostgresLabService) handleIndexes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	indexes, err := s.lab.ListIndexes(ctx, "users_demo")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.lab.CreateIndex(ctx, spec); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case http.MethodDelete:
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			http.Error(w, "name query parameter required", http.StatusBadRequest)
			return
		}
		if err := s.lab.DropIndex(ctx, name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *PostgresLabService) handleCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		SQL   string                  `json:"sql"`
		Index postgreslab.IndexSpec   `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	before, after, err := s.lab.CompareExplain(ctx, body.SQL, body.Index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	beforeReport, err := planexplainer.Parse(before)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	afterReport, err := planexplainer.Parse(after)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"before":     beforeReport,
		"after":      afterReport,
		"summary":    planexplainer.CompareReports(beforeReport, afterReport),
		"rawBefore":  before,
		"rawAfter":   after,
	})
}

func (s *PostgresLabService) handleRecommend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	recs := planexplainer.RecommendIndexes(body.SQL)
	writeJSON(w, http.StatusOK, map[string]interface{}{"recommendations": recs})
}
