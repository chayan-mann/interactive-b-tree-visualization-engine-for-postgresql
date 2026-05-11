// Package planexplainer parses PostgreSQL EXPLAIN (FORMAT JSON) output and
// turns it into a structured, human-friendly summary with educational notes.
package planexplainer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PlanReport summarises a single EXPLAIN output.
type PlanReport struct {
	PlanningTimeMs  float64    `json:"planningTimeMs"`
	ExecutionTimeMs float64    `json:"executionTimeMs"`
	TotalCost       float64    `json:"totalCost"`
	Rows            int64      `json:"rows"`
	Scans           []ScanInfo `json:"scans"`
	Highlights      []string   `json:"highlights"`
	Tree            *PlanNode  `json:"tree"`
}

// ScanInfo records a scan or join node we care about.
type ScanInfo struct {
	NodeType      string  `json:"nodeType"`
	Relation      string  `json:"relation,omitempty"`
	IndexName     string  `json:"indexName,omitempty"`
	IndexCond     string  `json:"indexCond,omitempty"`
	Filter        string  `json:"filter,omitempty"`
	Rows          int64   `json:"rows"`
	ActualRows    int64   `json:"actualRows"`
	StartupCost   float64 `json:"startupCost"`
	TotalCost     float64 `json:"totalCost"`
	ActualTimeMs  float64 `json:"actualTimeMs"`
	Loops         int64   `json:"loops"`
}

// PlanNode mirrors the recursive shape of EXPLAIN JSON, simplified for the UI.
type PlanNode struct {
	NodeType    string     `json:"nodeType"`
	Relation    string     `json:"relation,omitempty"`
	IndexName   string     `json:"indexName,omitempty"`
	IndexCond   string     `json:"indexCond,omitempty"`
	Filter      string     `json:"filter,omitempty"`
	Rows        int64      `json:"rows"`
	ActualRows  int64      `json:"actualRows"`
	TotalCost   float64    `json:"totalCost"`
	ActualTime  float64    `json:"actualTimeMs"`
	Children    []*PlanNode `json:"children,omitempty"`
}

// Parse accepts a raw EXPLAIN (FORMAT JSON) payload (always an array
// containing a single plan object) and returns a PlanReport.
func Parse(raw json.RawMessage) (*PlanReport, error) {
	var outer []struct {
		Plan          json.RawMessage `json:"Plan"`
		PlanningTime  float64         `json:"Planning Time"`
		ExecutionTime float64         `json:"Execution Time"`
	}
	if err := json.Unmarshal(raw, &outer); err != nil {
		return nil, fmt.Errorf("parse explain json: %w", err)
	}
	if len(outer) == 0 {
		return nil, fmt.Errorf("explain json had no plans")
	}
	root := walk(outer[0].Plan)
	if root == nil {
		return nil, fmt.Errorf("explain json had no root plan")
	}
	report := &PlanReport{
		PlanningTimeMs:  outer[0].PlanningTime,
		ExecutionTimeMs: outer[0].ExecutionTime,
		TotalCost:       root.TotalCost,
		Rows:            root.ActualRows,
		Tree:            root,
	}
	collect(root, report)
	report.Highlights = highlights(report)
	return report, nil
}

func walk(raw json.RawMessage) *PlanNode {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	node := &PlanNode{}
	if v, ok := m["Node Type"]; ok {
		_ = json.Unmarshal(v, &node.NodeType)
	}
	if v, ok := m["Relation Name"]; ok {
		_ = json.Unmarshal(v, &node.Relation)
	}
	if v, ok := m["Index Name"]; ok {
		_ = json.Unmarshal(v, &node.IndexName)
	}
	if v, ok := m["Index Cond"]; ok {
		_ = json.Unmarshal(v, &node.IndexCond)
	}
	if v, ok := m["Filter"]; ok {
		_ = json.Unmarshal(v, &node.Filter)
	}
	if v, ok := m["Plan Rows"]; ok {
		_ = json.Unmarshal(v, &node.Rows)
	}
	if v, ok := m["Actual Rows"]; ok {
		_ = json.Unmarshal(v, &node.ActualRows)
	}
	if v, ok := m["Total Cost"]; ok {
		_ = json.Unmarshal(v, &node.TotalCost)
	}
	if v, ok := m["Actual Total Time"]; ok {
		_ = json.Unmarshal(v, &node.ActualTime)
	}
	if v, ok := m["Plans"]; ok {
		var kids []json.RawMessage
		if err := json.Unmarshal(v, &kids); err == nil {
			for _, k := range kids {
				if child := walk(k); child != nil {
					node.Children = append(node.Children, child)
				}
			}
		}
	}
	return node
}

func collect(n *PlanNode, report *PlanReport) {
	if n == nil {
		return
	}
	report.Scans = append(report.Scans, ScanInfo{
		NodeType:     n.NodeType,
		Relation:     n.Relation,
		IndexName:    n.IndexName,
		IndexCond:    n.IndexCond,
		Filter:       n.Filter,
		Rows:         n.Rows,
		ActualRows:   n.ActualRows,
		TotalCost:    n.TotalCost,
		ActualTimeMs: n.ActualTime,
	})
	for _, c := range n.Children {
		collect(c, report)
	}
}

func highlights(r *PlanReport) []string {
	var notes []string
	for _, s := range r.Scans {
		switch s.NodeType {
		case "Seq Scan":
			n := fmt.Sprintf("Sequential scan on %s — PostgreSQL read every row", s.Relation)
			if s.Filter != "" {
				n += " and applied filter " + s.Filter
			}
			notes = append(notes, n+".")
		case "Index Scan":
			notes = append(notes, fmt.Sprintf("Index Scan on %s using %s with cond %s. PostgreSQL walked the B-tree and fetched matching heap rows.",
				s.Relation, s.IndexName, s.IndexCond))
		case "Index Only Scan":
			notes = append(notes, fmt.Sprintf("Index-Only Scan on %s using %s — query was satisfied from the index alone (covering index in action).",
				s.Relation, s.IndexName))
		case "Bitmap Index Scan":
			notes = append(notes, fmt.Sprintf("Bitmap Index Scan on %s — collected matching TIDs into a bitmap.", s.IndexName))
		case "Bitmap Heap Scan":
			n := fmt.Sprintf("Bitmap Heap Scan on %s — fetched the heap pages indicated by the bitmap.", s.Relation)
			if s.Filter != "" {
				n += " Residual filter: " + s.Filter
			}
			notes = append(notes, n)
		}
	}
	if len(notes) == 0 {
		notes = append(notes, fmt.Sprintf("Top node was %s.", r.Tree.NodeType))
	}
	return notes
}

// CompareReports diffs two reports and produces a short comparison summary.
func CompareReports(before, after *PlanReport) []string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Execution time: %.2f ms -> %.2f ms (%s)",
		before.ExecutionTimeMs, after.ExecutionTimeMs, deltaPct(before.ExecutionTimeMs, after.ExecutionTimeMs)))
	lines = append(lines, fmt.Sprintf("Total cost: %.2f -> %.2f", before.TotalCost, after.TotalCost))
	beforeTypes := topTypes(before)
	afterTypes := topTypes(after)
	if beforeTypes != afterTypes {
		lines = append(lines, fmt.Sprintf("Plan shape changed: %s -> %s", beforeTypes, afterTypes))
	} else {
		lines = append(lines, "Plan shape unchanged: "+afterTypes)
	}
	return lines
}

func topTypes(r *PlanReport) string {
	var parts []string
	for _, s := range r.Scans {
		parts = append(parts, s.NodeType)
	}
	return strings.Join(parts, " > ")
}

func deltaPct(before, after float64) string {
	if before == 0 {
		return "n/a"
	}
	pct := (after - before) / before * 100
	sign := "+"
	if pct < 0 {
		sign = ""
	}
	return fmt.Sprintf("%s%.1f%%", sign, pct)
}
