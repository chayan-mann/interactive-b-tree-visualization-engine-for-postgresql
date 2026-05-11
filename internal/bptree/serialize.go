package bptree

// NodeView is the JSON representation of a single tree node.
type NodeView struct {
	PageID       int      `json:"pageId"`
	IsLeaf       bool     `json:"isLeaf"`
	Keys         []int    `json:"keys"`
	Values       []string `json:"values,omitempty"`
	ChildPageIDs []int    `json:"childPageIds,omitempty"`
	NextPageID   *int     `json:"nextPageId,omitempty"`
	Level        int      `json:"level"`
}

// Snapshot captures the full tree shape for rendering or testing.
type Snapshot struct {
	Order      int          `json:"order"`
	Size       int          `json:"size"`
	Height     int          `json:"height"`
	MaxKeys    int          `json:"maxKeys"`
	MinKeys    int          `json:"minKeys"`
	RootPageID int          `json:"rootPageId"`
	Nodes      []NodeView   `json:"nodes"`
	Levels     [][]NodeView `json:"levels"`
	LeafChain  []int        `json:"leafChain"`
	DiskReads  int          `json:"diskReads"`
	DiskWrites int          `json:"diskWrites"`
}

// Snapshot returns a serializable view of the tree.
func (t *Tree) Snapshot() Snapshot {
	snap := Snapshot{
		Order:      t.order,
		Size:       t.size,
		MaxKeys:    t.maxKeys,
		MinKeys:    t.minKeys,
		DiskReads:  t.diskReads,
		DiskWrites: t.diskWrites,
	}
	if t.root == nil {
		return snap
	}
	snap.RootPageID = t.root.pageID

	type entry struct {
		n     *node
		level int
	}
	queue := []entry{{t.root, 0}}
	maxLevel := 0
	for len(queue) > 0 {
		e := queue[0]
		queue = queue[1:]
		if e.level > maxLevel {
			maxLevel = e.level
		}
		view := NodeView{
			PageID: e.n.pageID,
			IsLeaf: e.n.isLeaf,
			Keys:   append([]int{}, e.n.keys...),
			Level:  e.level,
		}
		if e.n.isLeaf {
			view.Values = append([]string{}, e.n.values...)
			if e.n.next != nil {
				next := e.n.next.pageID
				view.NextPageID = &next
			}
		} else {
			for _, c := range e.n.children {
				view.ChildPageIDs = append(view.ChildPageIDs, c.pageID)
				queue = append(queue, entry{c, e.level + 1})
			}
		}
		snap.Nodes = append(snap.Nodes, view)
	}

	snap.Height = maxLevel + 1
	snap.Levels = make([][]NodeView, snap.Height)
	for _, n := range snap.Nodes {
		snap.Levels[n.Level] = append(snap.Levels[n.Level], n)
	}

	leaf := t.firstLeaf()
	for leaf != nil {
		snap.LeafChain = append(snap.LeafChain, leaf.pageID)
		leaf = leaf.next
	}
	return snap
}

func (t *Tree) firstLeaf() *node {
	n := t.root
	if n == nil {
		return nil
	}
	for !n.isLeaf {
		n = n.children[0]
	}
	return n
}

// KeysInOrder returns every key currently stored in ascending order.
func (t *Tree) KeysInOrder() []int {
	var out []int
	leaf := t.firstLeaf()
	for leaf != nil {
		out = append(out, leaf.keys...)
		leaf = leaf.next
	}
	return out
}
