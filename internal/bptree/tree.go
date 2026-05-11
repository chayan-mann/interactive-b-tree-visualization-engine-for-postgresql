// Package bptree implements a teaching-grade B+ tree with linked leaves,
// configurable order, structured trace events for visualization, and simple
// disk-page simulation counters.
package bptree

import (
	"fmt"
	"sort"
)

// Config controls how the tree is constructed.
type Config struct {
	// Order is the maximum number of children for an internal node and the
	// maximum number of key slots for a leaf is Order - 1. Must be >= 3.
	Order int
}

// KV is a key/value pair returned from range scans and snapshots.
type KV struct {
	Key   int    `json:"key"`
	Value string `json:"value"`
}

// Tree is the public handle for a B+ tree instance.
type Tree struct {
	root       *node
	order      int
	maxKeys    int
	minKeys    int
	size       int
	nextPageID int
	trace      []Event
	diskReads  int
	diskWrites int
}

// New returns an empty tree using the provided configuration. Order < 3 is
// promoted to 4 so the structure stays well-defined for splits and merges.
func New(cfg Config) *Tree {
	if cfg.Order < 3 {
		cfg.Order = 4
	}
	maxKeys := cfg.Order - 1
	// floor((order-1)/2) — matches the smaller half produced by both leaf
	// and internal splits, so split outputs always satisfy the minimum.
	minKeys := (cfg.Order - 1) / 2
	if minKeys < 1 {
		minKeys = 1
	}
	t := &Tree{
		order:   cfg.Order,
		maxKeys: maxKeys,
		minKeys: minKeys,
	}
	t.root = t.newLeaf()
	return t
}

// Order returns the configured maximum number of children per internal node.
func (t *Tree) Order() int { return t.order }

// Size returns the number of distinct keys stored in the tree.
func (t *Tree) Size() int { return t.size }

// Height returns the number of levels in the tree (1 for a single leaf).
func (t *Tree) Height() int {
	h := 1
	n := t.root
	for !n.isLeaf {
		n = n.children[0]
		h++
	}
	return h
}

// DiskStats returns the simulated page reads and writes accumulated so far.
func (t *Tree) DiskStats() (reads, writes int) { return t.diskReads, t.diskWrites }

// pathEntry tracks an ancestor and the index of the child we descended into.
type pathEntry struct {
	node     *node
	childIdx int
}

func (t *Tree) findLeaf(key int) (*node, []pathEntry, []int) {
	var path []pathEntry
	var visited []int
	n := t.root
	t.diskReads++
	visited = append(visited, n.pageID)
	for !n.isLeaf {
		i := sort.SearchInts(n.keys, key+1)
		path = append(path, pathEntry{node: n, childIdx: i})
		n = n.children[i]
		t.diskReads++
		visited = append(visited, n.pageID)
	}
	return n, path, visited
}

// Search returns the value stored under key and a found indicator.
func (t *Tree) Search(key int) (string, bool) {
	t.resetTrace()
	leaf, _, visited := t.findLeaf(key)
	t.record("search_start", map[string]interface{}{"key": key})
	t.record("path", map[string]interface{}{"pageIds": visited})
	for i, k := range leaf.keys {
		if k == key {
			t.record("search_hit", map[string]interface{}{
				"key": key, "value": leaf.values[i], "pageId": leaf.pageID,
			})
			return leaf.values[i], true
		}
	}
	t.record("search_miss", map[string]interface{}{"key": key})
	return "", false
}

// RangeSearch returns key/value pairs with lo <= key <= hi in ascending order.
func (t *Tree) RangeSearch(lo, hi int) []KV {
	t.resetTrace()
	t.record("range_start", map[string]interface{}{"lo": lo, "hi": hi})
	if lo > hi {
		return nil
	}
	leaf, _, visited := t.findLeaf(lo)
	t.record("path", map[string]interface{}{"pageIds": visited})
	var out []KV
	for leaf != nil {
		t.record("range_visit_leaf", map[string]interface{}{"pageId": leaf.pageID, "keys": append([]int{}, leaf.keys...)})
		for i, k := range leaf.keys {
			if k < lo {
				continue
			}
			if k > hi {
				t.record("range_end", map[string]interface{}{"count": len(out)})
				return out
			}
			out = append(out, KV{Key: k, Value: leaf.values[i]})
		}
		if leaf.next != nil {
			t.record("leaf_link_follow", map[string]interface{}{"from": leaf.pageID, "to": leaf.next.pageID})
		}
		leaf = leaf.next
		if leaf != nil {
			t.diskReads++
		}
	}
	t.record("range_end", map[string]interface{}{"count": len(out)})
	return out
}

// String returns a quick multi-line tree view for debugging.
func (t *Tree) String() string {
	if t.root == nil {
		return "<empty>"
	}
	var out string
	type item struct {
		n     *node
		level int
	}
	queue := []item{{t.root, 0}}
	curLevel := 0
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		if it.level != curLevel {
			out += "\n"
			curLevel = it.level
		}
		out += fmt.Sprintf("%v ", it.n.keys)
		if !it.n.isLeaf {
			for _, c := range it.n.children {
				queue = append(queue, item{c, it.level + 1})
			}
		}
	}
	return out
}
