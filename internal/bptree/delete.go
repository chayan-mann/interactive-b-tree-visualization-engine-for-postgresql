package bptree

// Delete removes the entry with the given key. Returns true if a key was
// removed, false if the key was not present.
func (t *Tree) Delete(key int) bool {
	t.resetTrace()
	t.record("delete_start", map[string]interface{}{"key": key})

	leaf, path, visited := t.findLeaf(key)
	t.record("path", map[string]interface{}{"nodePath": append([]int{}, visited...), "pathLength": len(visited), "fromNode": t.root.pageID})

	idx := -1
	for i, k := range leaf.keys {
		if k == key {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.record("delete_miss", map[string]interface{}{"key": key, "fromNode": leaf.pageID})
		return false
	}

	leaf.keys = removeIntAt(leaf.keys, idx)
	leaf.values = removeStrAt(leaf.values, idx)
	t.size--
	t.diskWrites++
	t.record("delete_from_leaf", map[string]interface{}{
		"key":      key,
		"fromNode": leaf.pageID,
		"nodeKeys": append([]int{}, leaf.keys...),
		"nodeSize": len(leaf.keys),
	})

	if leaf == t.root {
		return true
	}
	if len(leaf.keys) >= t.minKeys {
		return true
	}
	t.rebalanceLeaf(leaf, path)
	return true
}

func (t *Tree) rebalanceLeaf(leaf *node, path []pathEntry) {
	pe := path[len(path)-1]
	parent := pe.node
	idx := pe.childIdx

	if idx > 0 {
		left := parent.children[idx-1]
		if len(left.keys) > t.minKeys {
			bKey := left.keys[len(left.keys)-1]
			bVal := left.values[len(left.values)-1]
			left.keys = left.keys[:len(left.keys)-1]
			left.values = left.values[:len(left.values)-1]
			leaf.keys = append([]int{bKey}, leaf.keys...)
			leaf.values = append([]string{bVal}, leaf.values...)
			parent.keys[idx-1] = leaf.keys[0]
			t.diskWrites += 3
			t.record("borrow_from_left_leaf", map[string]interface{}{
				"fromNode":         left.pageID,
				"toNode":           leaf.pageID,
				"borrowedKey":      bKey,
				"borrowedValue":    bVal,
				"separator":        parent.keys[idx-1],
				"rebalanceResult":  "borrow_left_done",
				"nodeKeys":         append([]int{}, leaf.keys...),
			})
			t.record("borrow_from_left_leaf_done", map[string]interface{}{
			"fromNode":   leaf.pageID,
			"toNode":     left.pageID,
			"separator":  parent.keys[idx-1],
			"nodeKeys":   append([]int{}, leaf.keys...),
			"result":     "borrowed_from_left",
		})
			return
		}
	}

	if idx < len(parent.children)-1 {
		right := parent.children[idx+1]
		if len(right.keys) > t.minKeys {
			bKey := right.keys[0]
			bVal := right.values[0]
			right.keys = right.keys[1:]
			right.values = right.values[1:]
			leaf.keys = append(leaf.keys, bKey)
			leaf.values = append(leaf.values, bVal)
			parent.keys[idx] = right.keys[0]
			t.diskWrites += 3
			t.record("borrow_from_right_leaf", map[string]interface{}{
				"fromNode":         right.pageID,
				"toNode":           leaf.pageID,
				"borrowedKey":      bKey,
				"borrowedValue":    bVal,
				"separator":        parent.keys[idx],
				"rebalanceResult":  "borrow_right_done",
				"nodeKeys":         append([]int{}, leaf.keys...),
			})
			t.record("borrow_from_right_leaf_done", map[string]interface{}{
			"fromNode":  leaf.pageID,
			"toNode":    right.pageID,
			"separator": parent.keys[idx],
			"nodeKeys":  append([]int{}, leaf.keys...),
			"result":    "borrowed_from_right",
		})
			return
		}
	}

	if idx > 0 {
		left := parent.children[idx-1]
		left.keys = append(left.keys, leaf.keys...)
		left.values = append(left.values, leaf.values...)
		left.next = leaf.next
		parent.keys = removeIntAt(parent.keys, idx-1)
		parent.children = removeNodeAt(parent.children, idx)
		t.diskWrites += 2
		t.record("merge_leaf", map[string]interface{}{
			"fromNode":         left.pageID,
			"toNode":           leaf.pageID,
			"removedNode":       leaf.pageID,
			"rebalanceResult":  "merge_left",
			"nodeKeys":         append([]int{}, left.keys...),
		})
	} else {
		right := parent.children[idx+1]
		leaf.keys = append(leaf.keys, right.keys...)
		leaf.values = append(leaf.values, right.values...)
		leaf.next = right.next
		parent.keys = removeIntAt(parent.keys, idx)
		parent.children = removeNodeAt(parent.children, idx+1)
		t.diskWrites += 2
		t.record("merge_leaf", map[string]interface{}{
			"fromNode":         leaf.pageID,
			"toNode":           right.pageID,
			"removedNode":       right.pageID,
			"rebalanceResult":  "merge_right",
			"nodeKeys":         append([]int{}, leaf.keys...),
		})
	}

	t.handleParentAfterMerge(parent, path[:len(path)-1])
}

func (t *Tree) handleParentAfterMerge(parent *node, path []pathEntry) {
	if parent == t.root {
		if len(parent.keys) == 0 {
			t.root = parent.children[0]
			t.record("root_contract", map[string]interface{}{
				"fromNode":         parent.pageID,
				"toNode":           t.root.pageID,
				"rebalanceResult":  "root_contract",
			})
		}
		return
	}
	if len(parent.keys) < t.minKeys {
		t.rebalanceInternal(parent, path)
	}
}

func (t *Tree) rebalanceInternal(n *node, path []pathEntry) {
	pe := path[len(path)-1]
	parent := pe.node
	idx := pe.childIdx

	if idx > 0 {
		left := parent.children[idx-1]
		if len(left.keys) > t.minKeys {
			sep := parent.keys[idx-1]
			bKey := left.keys[len(left.keys)-1]
			bChild := left.children[len(left.children)-1]
			left.keys = left.keys[:len(left.keys)-1]
			left.children = left.children[:len(left.children)-1]
			n.keys = append([]int{sep}, n.keys...)
			n.children = append([]*node{bChild}, n.children...)
			parent.keys[idx-1] = bKey
			t.diskWrites += 3
			t.record("borrow_from_left_internal", map[string]interface{}{
				"fromNode":        left.pageID,
				"toNode":          n.pageID,
				"separator":       bKey,
				"rotatedKey":      sep,
				"rebalanceResult": "borrow_internal_left",
			})
			return
		}
	}

	if idx < len(parent.children)-1 {
		right := parent.children[idx+1]
		if len(right.keys) > t.minKeys {
			sep := parent.keys[idx]
			bKey := right.keys[0]
			bChild := right.children[0]
			right.keys = right.keys[1:]
			right.children = right.children[1:]
			n.keys = append(n.keys, sep)
			n.children = append(n.children, bChild)
			parent.keys[idx] = bKey
			t.diskWrites += 3
			t.record("borrow_from_right_internal", map[string]interface{}{
				"fromNode":        right.pageID,
				"toNode":          n.pageID,
				"separator":       bKey,
				"rotatedKey":      sep,
				"rebalanceResult": "borrow_internal_right",
			})
			return
		}
	}

	if idx > 0 {
		left := parent.children[idx-1]
		sep := parent.keys[idx-1]
		left.keys = append(left.keys, sep)
		left.keys = append(left.keys, n.keys...)
		left.children = append(left.children, n.children...)
		parent.keys = removeIntAt(parent.keys, idx-1)
		parent.children = removeNodeAt(parent.children, idx)
		t.diskWrites += 2
		t.record("merge_internal", map[string]interface{}{
			"fromNode":        left.pageID,
			"toNode":          n.pageID,
			"separator":       sep,
			"rebalanceResult": "merge_internal_left",
			"nodeKeys":        append([]int{}, left.keys...),
		})
	} else {
		right := parent.children[idx+1]
		sep := parent.keys[idx]
		n.keys = append(n.keys, sep)
		n.keys = append(n.keys, right.keys...)
		n.children = append(n.children, right.children...)
		parent.keys = removeIntAt(parent.keys, idx)
		parent.children = removeNodeAt(parent.children, idx+1)
		t.diskWrites += 2
		t.record("merge_internal", map[string]interface{}{
			"fromNode":        n.pageID,
			"toNode":          right.pageID,
			"separator":       sep,
			"rebalanceResult": "merge_internal_right",
			"nodeKeys":        append([]int{}, n.keys...),
		})
	}

	t.handleParentAfterMerge(parent, path[:len(path)-1])
}
