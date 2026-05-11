package bptree

// Insert adds or updates the value stored under key.
func (t *Tree) Insert(key int, value string) {
	t.resetTrace()
	t.record("insert_start", map[string]interface{}{"key": key, "value": value})

	leaf, path, visited := t.findLeaf(key)
	t.record("path", map[string]interface{}{"pageIds": visited})

	for i, k := range leaf.keys {
		if k == key {
			leaf.values[i] = value
			t.diskWrites++
			t.record("insert_update", map[string]interface{}{"key": key, "pageId": leaf.pageID})
			return
		}
	}

	pos := 0
	for pos < len(leaf.keys) && leaf.keys[pos] < key {
		pos++
	}
	leaf.keys = insertIntAt(leaf.keys, pos, key)
	leaf.values = insertStrAt(leaf.values, pos, value)
	t.size++
	t.diskWrites++
	t.record("insert_into_leaf", map[string]interface{}{
		"key": key, "pageId": leaf.pageID, "keys": append([]int{}, leaf.keys...),
	})

	if len(leaf.keys) > t.maxKeys {
		t.splitLeaf(leaf, path)
	}
}

func (t *Tree) splitLeaf(leaf *node, path []pathEntry) {
	mid := (len(leaf.keys) + 1) / 2
	right := t.newLeaf()
	right.keys = append(right.keys, leaf.keys[mid:]...)
	right.values = append(right.values, leaf.values[mid:]...)
	leaf.keys = leaf.keys[:mid]
	leaf.values = leaf.values[:mid]
	right.next = leaf.next
	leaf.next = right
	promote := right.keys[0]
	t.diskWrites += 2
	t.record("split_leaf", map[string]interface{}{
		"leftPageId":  leaf.pageID,
		"rightPageId": right.pageID,
		"leftKeys":    append([]int{}, leaf.keys...),
		"rightKeys":   append([]int{}, right.keys...),
		"promote":     promote,
	})
	t.insertIntoParent(leaf, right, promote, path)
}

func (t *Tree) insertIntoParent(left, right *node, promote int, path []pathEntry) {
	if len(path) == 0 {
		newRoot := t.newInternal()
		newRoot.keys = []int{promote}
		newRoot.children = []*node{left, right}
		t.root = newRoot
		t.diskWrites++
		t.record("new_root", map[string]interface{}{
			"pageId": newRoot.pageID, "keys": []int{promote},
		})
		return
	}
	parent := path[len(path)-1].node
	pos := 0
	for pos < len(parent.keys) && parent.keys[pos] < promote {
		pos++
	}
	parent.keys = insertIntAt(parent.keys, pos, promote)
	parent.children = insertNodeAt(parent.children, pos+1, right)
	t.diskWrites++
	t.record("promote_key", map[string]interface{}{
		"parentPageId": parent.pageID,
		"key":          promote,
		"parentKeys":   append([]int{}, parent.keys...),
	})
	if len(parent.keys) > t.maxKeys {
		t.splitInternal(parent, path[:len(path)-1])
	}
}

func (t *Tree) splitInternal(n *node, path []pathEntry) {
	mid := len(n.keys) / 2
	promote := n.keys[mid]
	right := t.newInternal()
	right.keys = append(right.keys, n.keys[mid+1:]...)
	right.children = append(right.children, n.children[mid+1:]...)
	n.keys = n.keys[:mid]
	n.children = n.children[:mid+1]
	t.diskWrites += 2
	t.record("split_internal", map[string]interface{}{
		"leftPageId":  n.pageID,
		"rightPageId": right.pageID,
		"leftKeys":    append([]int{}, n.keys...),
		"rightKeys":   append([]int{}, right.keys...),
		"promote":     promote,
	})
	t.insertIntoParent(n, right, promote, path)
}
