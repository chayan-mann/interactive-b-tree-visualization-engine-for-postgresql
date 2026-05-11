package bptree

type node struct {
	isLeaf   bool
	keys     []int
	values   []string
	children []*node
	next     *node
	pageID   int
}

func (t *Tree) newLeaf() *node {
	t.nextPageID++
	return &node{isLeaf: true, pageID: t.nextPageID}
}

func (t *Tree) newInternal() *node {
	t.nextPageID++
	return &node{isLeaf: false, pageID: t.nextPageID}
}

func insertIntAt(s []int, pos int, v int) []int {
	s = append(s, 0)
	copy(s[pos+1:], s[pos:])
	s[pos] = v
	return s
}

func insertStrAt(s []string, pos int, v string) []string {
	s = append(s, "")
	copy(s[pos+1:], s[pos:])
	s[pos] = v
	return s
}

func insertNodeAt(s []*node, pos int, v *node) []*node {
	s = append(s, nil)
	copy(s[pos+1:], s[pos:])
	s[pos] = v
	return s
}

func removeIntAt(s []int, pos int) []int {
	return append(s[:pos], s[pos+1:]...)
}

func removeStrAt(s []string, pos int) []string {
	return append(s[:pos], s[pos+1:]...)
}

func removeNodeAt(s []*node, pos int) []*node {
	return append(s[:pos], s[pos+1:]...)
}
