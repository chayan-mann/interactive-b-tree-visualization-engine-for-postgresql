package planexplainer

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSequentialScan(t *testing.T) {
	raw := json.RawMessage(`[{
		"Plan": {
			"Node Type": "Seq Scan",
			"Relation Name": "users_demo",
			"Plan Rows": 1000,
			"Actual Rows": 1234,
			"Total Cost": 18000.0,
			"Filter": "(age = 30)"
		},
		"Planning Time": 0.5,
		"Execution Time": 120.7
	}]`)
	report, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.ExecutionTimeMs != 120.7 {
		t.Fatalf("execution time mismatch: %v", report.ExecutionTimeMs)
	}
	if len(report.Scans) != 1 || report.Scans[0].NodeType != "Seq Scan" {
		t.Fatalf("scan parse failed: %+v", report.Scans)
	}
	if len(report.Highlights) == 0 || !strings.Contains(report.Highlights[0], "Sequential scan") {
		t.Fatalf("missing seq scan highlight: %+v", report.Highlights)
	}
}

func TestParseBitmapPlan(t *testing.T) {
	raw := json.RawMessage(`[{
		"Plan": {
			"Node Type": "Bitmap Heap Scan",
			"Relation Name": "users_demo",
			"Plan Rows": 200,
			"Actual Rows": 198,
			"Total Cost": 200.0,
			"Filter": "(city = 'Mumbai')",
			"Plans": [
				{
					"Node Type": "Bitmap Index Scan",
					"Index Name": "idx_users_age",
					"Index Cond": "(age = 30)",
					"Plan Rows": 200,
					"Actual Rows": 198,
					"Total Cost": 40.0
				}
			]
		},
		"Planning Time": 0.1,
		"Execution Time": 4.2
	}]`)
	report, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Tree == nil || report.Tree.NodeType != "Bitmap Heap Scan" {
		t.Fatalf("expected bitmap heap scan as root")
	}
	if len(report.Tree.Children) != 1 || report.Tree.Children[0].NodeType != "Bitmap Index Scan" {
		t.Fatalf("expected child bitmap index scan")
	}
	if len(report.Scans) != 2 {
		t.Fatalf("expected 2 scans got %d", len(report.Scans))
	}
}

func TestRecommendComposite(t *testing.T) {
	sql := "SELECT * FROM orders WHERE customer_id = 10 AND created_at > now() - interval '30 days'"
	recs := RecommendIndexes(sql)
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}
	r := recs[0]
	if r.Table != "orders" {
		t.Fatalf("table want orders got %s", r.Table)
	}
	if len(r.Columns) != 2 || r.Columns[0] != "customer_id" || r.Columns[1] != "created_at" {
		t.Fatalf("composite cols want [customer_id, created_at] got %v", r.Columns)
	}
	if !strings.Contains(r.SQL, "CREATE INDEX") {
		t.Fatalf("missing CREATE INDEX in SQL: %s", r.SQL)
	}
}

func TestRecommendNoWhere(t *testing.T) {
	if r := RecommendIndexes("SELECT * FROM users_demo"); len(r) != 0 {
		t.Fatalf("expected no recommendation, got %v", r)
	}
}

func TestCompareReports(t *testing.T) {
	a := &PlanReport{ExecutionTimeMs: 100, TotalCost: 1000, Scans: []ScanInfo{{NodeType: "Seq Scan"}}}
	b := &PlanReport{ExecutionTimeMs: 10, TotalCost: 50, Scans: []ScanInfo{{NodeType: "Index Scan"}}}
	lines := CompareReports(a, b)
	if len(lines) == 0 {
		t.Fatal("expected comparison lines")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Plan shape changed") {
		t.Fatalf("expected change marker, got: %s", joined)
	}
}
