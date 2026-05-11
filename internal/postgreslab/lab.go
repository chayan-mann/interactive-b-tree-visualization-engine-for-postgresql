// Package postgreslab connects to a PostgreSQL instance, manages a demo
// `users_demo` table, seeds rows, and captures EXPLAIN (ANALYZE, ..., FORMAT
// JSON) output so the rest of the app can compare query plans.
package postgreslab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Lab manages a single PostgreSQL connection pool and exposes high-level
// operations the API layer can call.
type Lab struct {
	pool *pgxpool.Pool
}

// New connects to the given DSN. Returns an error if the connection cannot
// be established within a short timeout.
func New(dsn string) (*Lab, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Lab{pool: pool}, nil
}

// Close releases the connection pool.
func (l *Lab) Close() { l.pool.Close() }

// Setup creates the demo schema. Safe to call repeatedly.
func (l *Lab) Setup(ctx context.Context) error {
	_, err := l.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS users_demo (
    id BIGSERIAL PRIMARY KEY,
    username TEXT NOT NULL,
    age INT NOT NULL,
    city TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);`)
	return err
}

// RowCount returns the number of rows in users_demo.
func (l *Lab) RowCount(ctx context.Context) (int64, error) {
	var n int64
	err := l.pool.QueryRow(ctx, `SELECT count(*) FROM users_demo`).Scan(&n)
	return n, err
}

// Seed inserts `rows` synthetic rows using a single set-based INSERT so
// generating a million is fast. If `truncate` is true the table is cleared
// first.
func (l *Lab) Seed(ctx context.Context, rows int, truncate bool) error {
	if rows <= 0 {
		return errors.New("rows must be positive")
	}
	if err := l.Setup(ctx); err != nil {
		return err
	}
	if truncate {
		if _, err := l.pool.Exec(ctx, `TRUNCATE users_demo RESTART IDENTITY`); err != nil {
			return err
		}
	}
	const stmt = `
INSERT INTO users_demo (username, age, city, created_at)
SELECT
  'user_' || g::text,
  18 + (random() * 60)::int,
  (ARRAY['Mumbai','Delhi','Bangalore','Hyderabad','Chennai','Kolkata','Pune','Jaipur','Ahmedabad','Surat'])[1 + (random() * 9)::int],
  NOW() - (random() * INTERVAL '720 days')
FROM generate_series(1, $1) AS g;`
	_, err := l.pool.Exec(ctx, stmt, rows)
	return err
}

// IndexSpec describes an index to create.
type IndexSpec struct {
	Name     string   `json:"name"`
	Table    string   `json:"table"`
	Columns  []string `json:"columns"`
	Include  []string `json:"include,omitempty"`
	Unique   bool     `json:"unique,omitempty"`
}

// CreateIndex builds and executes a CREATE INDEX statement. Identifiers are
// validated to allow only simple ASCII names to keep the demo safe.
func (l *Lab) CreateIndex(ctx context.Context, spec IndexSpec) error {
	if err := validateIdent(spec.Name); err != nil {
		return fmt.Errorf("name: %w", err)
	}
	table := spec.Table
	if table == "" {
		table = "users_demo"
	}
	if err := validateIdent(table); err != nil {
		return fmt.Errorf("table: %w", err)
	}
	if len(spec.Columns) == 0 {
		return errors.New("at least one column is required")
	}
	for _, c := range spec.Columns {
		if err := validateIdent(c); err != nil {
			return fmt.Errorf("column %q: %w", c, err)
		}
	}
	for _, c := range spec.Include {
		if err := validateIdent(c); err != nil {
			return fmt.Errorf("include %q: %w", c, err)
		}
	}
	unique := ""
	if spec.Unique {
		unique = "UNIQUE "
	}
	include := ""
	if len(spec.Include) > 0 {
		include = " INCLUDE (" + strings.Join(spec.Include, ", ") + ")"
	}
	sql := fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS %s ON %s (%s)%s",
		unique, spec.Name, table, strings.Join(spec.Columns, ", "), include)
	_, err := l.pool.Exec(ctx, sql)
	return err
}

// DropIndex removes the named index if it exists.
func (l *Lab) DropIndex(ctx context.Context, name string) error {
	if err := validateIdent(name); err != nil {
		return err
	}
	_, err := l.pool.Exec(ctx, fmt.Sprintf("DROP INDEX IF EXISTS %s", name))
	return err
}

// IndexInfo describes a single index returned by ListIndexes.
type IndexInfo struct {
	Name       string `json:"name"`
	Table      string `json:"table"`
	Definition string `json:"definition"`
	SizeBytes  int64  `json:"sizeBytes"`
}

// ListIndexes enumerates indexes on the demo table.
func (l *Lab) ListIndexes(ctx context.Context, table string) ([]IndexInfo, error) {
	if table == "" {
		table = "users_demo"
	}
	if err := validateIdent(table); err != nil {
		return nil, err
	}
	rows, err := l.pool.Query(ctx, `
SELECT i.indexname,
       i.tablename,
       i.indexdef,
       pg_relation_size(format('%I.%I', i.schemaname, i.indexname))
FROM pg_indexes i
WHERE i.tablename = $1
ORDER BY i.indexname`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IndexInfo
	for rows.Next() {
		var info IndexInfo
		if err := rows.Scan(&info.Name, &info.Table, &info.Definition, &info.SizeBytes); err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, rows.Err()
}

// QueryResult holds rows returned by a user-issued query.
type QueryResult struct {
	Columns  []string         `json:"columns"`
	Rows     [][]interface{}  `json:"rows"`
	Truncated bool            `json:"truncated"`
	Duration time.Duration    `json:"durationNs"`
}

// RunQuery executes an arbitrary read-only-looking SQL string. The first
// 200 rows are returned; the rest are skipped.
func (l *Lab) RunQuery(ctx context.Context, sql string, args ...interface{}) (*QueryResult, error) {
	if err := guardReadOnly(sql); err != nil {
		return nil, err
	}
	start := time.Now()
	rows, err := l.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := rows.FieldDescriptions()
	result := &QueryResult{}
	for _, c := range cols {
		result.Columns = append(result.Columns, string(c.Name))
	}
	const maxRows = 200
	for rows.Next() {
		if len(result.Rows) >= maxRows {
			result.Truncated = true
			break
		}
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		result.Rows = append(result.Rows, values)
	}
	result.Duration = time.Since(start)
	return result, rows.Err()
}

// ExplainAnalyze runs EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) and returns
// the JSON-decoded plan plus the raw JSON payload.
func (l *Lab) ExplainAnalyze(ctx context.Context, sql string, args ...interface{}) (json.RawMessage, error) {
	if err := guardReadOnly(sql); err != nil {
		return nil, err
	}
	row := l.pool.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, VERBOSE, FORMAT JSON) "+sql, args...)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// CompareExplain runs the same query before and after creating a candidate
// index, then drops the index again so the lab stays in its starting state.
func (l *Lab) CompareExplain(ctx context.Context, sql string, spec IndexSpec) (before, after json.RawMessage, err error) {
	before, err = l.ExplainAnalyze(ctx, sql)
	if err != nil {
		return nil, nil, err
	}
	if err = l.CreateIndex(ctx, spec); err != nil {
		return nil, nil, err
	}
	if _, aerr := l.pool.Exec(ctx, "ANALYZE users_demo"); aerr != nil {
		// Non-fatal.
	}
	after, err = l.ExplainAnalyze(ctx, sql)
	if dropErr := l.DropIndex(ctx, spec.Name); dropErr != nil && err == nil {
		err = dropErr
	}
	return before, after, err
}

// AcquireConn is a helper for callers that want raw access; not used right
// now but kept so additional handlers can extend the lab without breaking
// encapsulation.
func (l *Lab) AcquireConn(ctx context.Context) (*pgxpool.Conn, error) {
	return l.pool.Acquire(ctx)
}

// guardReadOnly rejects obviously mutating SQL so the demo endpoint cannot
// be used to wipe data.
func guardReadOnly(sql string) error {
	trimmed := strings.ToLower(strings.TrimSpace(sql))
	for _, bad := range []string{"insert ", "update ", "delete ", "drop ", "truncate ", "alter ", "create ", "grant ", "revoke "} {
		if strings.HasPrefix(trimmed, bad) {
			return fmt.Errorf("only read-only queries are allowed here; use the dedicated endpoints for %q", strings.TrimSpace(bad))
		}
	}
	return nil
}

// validateIdent ensures an identifier is a simple snake_case ASCII token.
func validateIdent(s string) error {
	if s == "" {
		return errors.New("identifier required")
	}
	if len(s) > 63 {
		return errors.New("identifier too long")
	}
	for i, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9')
		if !ok {
			return fmt.Errorf("identifier %q contains invalid character", s)
		}
	}
	return nil
}

// Silence unused import for pgx in builds where we only use pgxpool.
var _ = pgx.Identifier{}
