package planexplainer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Recommendation is a candidate index suggestion derived from a SQL string.
type Recommendation struct {
	Table   string   `json:"table"`
	Columns []string `json:"columns"`
	Reason  string   `json:"reason"`
	SQL     string   `json:"sql"`
}

var (
	reFromTable = regexp.MustCompile(`(?is)\bfrom\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	// match `col = val`, `col > val`, `col < val`, etc., with simple identifiers
	rePredicate = regexp.MustCompile(`(?i)([a-zA-Z_][a-zA-Z0-9_]*)\s*(=|<=|>=|<>|!=|<|>)\s*([^\s,()]+)`)
)

// RecommendIndexes inspects a simple SELECT and proposes a small composite
// index based on equality and range predicates in WHERE / AND clauses.
// The heuristic intentionally stays conservative — it is meant to teach,
// not replace PostgreSQL's planner.
func RecommendIndexes(sql string) []Recommendation {
	tableMatch := reFromTable.FindStringSubmatch(sql)
	if tableMatch == nil {
		return nil
	}
	table := tableMatch[1]

	whereIdx := indexOfFold(sql, " where ")
	if whereIdx < 0 {
		return nil
	}
	rest := sql[whereIdx+len(" where "):]
	for _, term := range []string{" group by ", " order by ", " limit ", " offset "} {
		if i := indexOfFold(rest, term); i > 0 {
			rest = rest[:i]
		}
	}

	matches := rePredicate.FindAllStringSubmatch(rest, -1)
	if len(matches) == 0 {
		return nil
	}

	// Bucket predicates into equality vs range and dedup column references.
	seen := map[string]bool{}
	var equality []string
	var ranges []string
	for _, m := range matches {
		col := strings.ToLower(m[1])
		op := m[2]
		if seen[col] {
			continue
		}
		seen[col] = true
		if op == "=" {
			equality = append(equality, col)
		} else {
			ranges = append(ranges, col)
		}
	}

	sort.Strings(equality)
	sort.Strings(ranges)
	cols := append([]string{}, equality...)
	cols = append(cols, ranges...)
	if len(cols) == 0 {
		return nil
	}

	reason := buildReason(len(equality), len(ranges))
	name := buildIndexName(table, cols)
	indexSQL := fmt.Sprintf("CREATE INDEX %s ON %s (%s);", name, table, strings.Join(cols, ", "))

	rec := Recommendation{
		Table:   table,
		Columns: cols,
		Reason:  reason,
		SQL:     indexSQL,
	}
	return []Recommendation{rec}
}

func buildReason(eqCount, rangeCount int) string {
	switch {
	case eqCount > 0 && rangeCount > 0:
		return "Equality columns are placed first so PostgreSQL can pin the search to those values, then the range column can be scanned within that subtree."
	case eqCount > 0:
		return "Equality predicates can be answered directly with a B-tree lookup."
	case rangeCount > 0:
		return "Range predicates benefit from a B-tree index because the linked leaves make sequential reads cheap."
	default:
		return "Suggested based on WHERE clause columns."
	}
}

func buildIndexName(table string, cols []string) string {
	parts := []string{"idx", table}
	parts = append(parts, cols...)
	return strings.Join(parts, "_")
}

func indexOfFold(s, sub string) int {
	return strings.Index(strings.ToLower(s), sub)
}
