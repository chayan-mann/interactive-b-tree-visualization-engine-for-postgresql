package bptree

import "fmt"

// CheckInvariants validates structural invariants of the tree. It returns
// nil if the tree is well-formed and a descriptive error otherwise. This is
// primarily intended for tests but can be useful for runtime debugging.
func (t *Tree) CheckInvariants() error {
	if t.root == nil {
		return fmt.Errorf("nil root")
	}
	// Check sort order, value/key alignment, child count consistency, and
	// uniform leaf depth via a depth/error tracker.
	leafDepth := -1
	var visit func(n *node, depth int, lower, upper *int) error
	visit = func(n *node, depth int, lower, upper *int) error {
		for i := 1; i < len(n.keys); i++ {
			if n.keys[i-1] >= n.keys[i] {
				return fmt.Errorf("page %d keys not strictly sorted: %v", n.pageID, n.keys)
			}
		}
		if lower != nil {
			for _, k := range n.keys {
				if k < *lower {
					return fmt.Errorf("page %d key %d below lower bound %d", n.pageID, k, *lower)
				}
			}
		}
		if upper != nil {
			for _, k := range n.keys {
				if k >= *upper {
					return fmt.Errorf("page %d key %d above upper bound %d", n.pageID, k, *upper)
				}
			}
		}
		if n.isLeaf {
			if len(n.keys) != len(n.values) {
				return fmt.Errorf("page %d keys/values length mismatch", n.pageID)
			}
			if leafDepth == -1 {
				leafDepth = depth
			} else if depth != leafDepth {
				return fmt.Errorf("leaf depth mismatch: %d vs %d", depth, leafDepth)
			}
			if n != t.root && len(n.keys) < t.minKeys {
				return fmt.Errorf("leaf page %d underflow: %d keys (min %d)", n.pageID, len(n.keys), t.minKeys)
			}
			if len(n.keys) > t.maxKeys {
				return fmt.Errorf("leaf page %d overflow: %d keys (max %d)", n.pageID, len(n.keys), t.maxKeys)
			}
			return nil
		}
		if len(n.children) != len(n.keys)+1 {
			return fmt.Errorf("internal page %d has %d keys and %d children", n.pageID, len(n.keys), len(n.children))
		}
		if n != t.root && len(n.keys) < t.minKeys {
			return fmt.Errorf("internal page %d underflow: %d keys (min %d)", n.pageID, len(n.keys), t.minKeys)
		}
		if len(n.keys) > t.maxKeys {
			return fmt.Errorf("internal page %d overflow: %d keys (max %d)", n.pageID, len(n.keys), t.maxKeys)
		}
		for i, child := range n.children {
			var childLower, childUpper *int
			if i == 0 {
				childLower = lower
			} else {
				k := n.keys[i-1]
				childLower = &k
			}
			if i == len(n.children)-1 {
				childUpper = upper
			} else {
				k := n.keys[i]
				childUpper = &k
			}
			if err := visit(child, depth+1, childLower, childUpper); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(t.root, 0, nil, nil); err != nil {
		return err
	}

	// Validate the leaf chain order matches sorted keys.
	leaf := t.firstLeaf()
	var prev int
	first := true
	for leaf != nil {
		for _, k := range leaf.keys {
			if !first && k <= prev {
				return fmt.Errorf("leaf chain not ascending at key %d", k)
			}
			prev = k
			first = false
		}
		leaf = leaf.next
	}
	return nil
}
