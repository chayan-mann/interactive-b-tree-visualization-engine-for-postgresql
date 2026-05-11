package bptree

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

func mustOK(t *testing.T, tr *Tree) {
	t.Helper()
	if err := tr.CheckInvariants(); err != nil {
		t.Fatalf("invariants broken: %v\n%s", err, tr.String())
	}
}

func TestEmptyTree(t *testing.T) {
	tr := New(Config{Order: 4})
	if _, ok := tr.Search(10); ok {
		t.Fatal("empty tree should not find key")
	}
	if got := tr.RangeSearch(0, 100); len(got) != 0 {
		t.Fatalf("empty range want 0 got %d", len(got))
	}
	if tr.Size() != 0 {
		t.Fatalf("size should be 0")
	}
}

func TestInsertAndSearch(t *testing.T) {
	tr := New(Config{Order: 4})
	keys := []int{10, 20, 5, 6, 12, 30, 7, 17, 1, 2, 3, 4, 8, 9, 11}
	for _, k := range keys {
		tr.Insert(k, fmt.Sprintf("v%d", k))
		mustOK(t, tr)
	}
	for _, k := range keys {
		v, ok := tr.Search(k)
		if !ok || v != fmt.Sprintf("v%d", k) {
			t.Fatalf("search %d want v%d got %q ok=%v", k, k, v, ok)
		}
	}
	if _, ok := tr.Search(999); ok {
		t.Fatal("found nonexistent key")
	}
}

func TestUpdateExisting(t *testing.T) {
	tr := New(Config{Order: 5})
	tr.Insert(1, "a")
	tr.Insert(1, "b")
	v, ok := tr.Search(1)
	if !ok || v != "b" {
		t.Fatalf("update failed: got %q ok=%v", v, ok)
	}
	if tr.Size() != 1 {
		t.Fatalf("size should be 1 got %d", tr.Size())
	}
}

func TestRangeSearch(t *testing.T) {
	tr := New(Config{Order: 4})
	for i := 1; i <= 50; i++ {
		tr.Insert(i, fmt.Sprintf("v%d", i))
	}
	mustOK(t, tr)
	got := tr.RangeSearch(10, 20)
	if len(got) != 11 {
		t.Fatalf("range len want 11 got %d", len(got))
	}
	for i, kv := range got {
		if kv.Key != 10+i {
			t.Fatalf("range %d want %d got %d", i, 10+i, kv.Key)
		}
	}
}

func TestRangeSearchSparse(t *testing.T) {
	tr := New(Config{Order: 3})
	keys := []int{5, 15, 25, 35, 45}
	for _, k := range keys {
		tr.Insert(k, "")
	}
	got := tr.RangeSearch(10, 40)
	want := []int{15, 25, 35}
	if len(got) != len(want) {
		t.Fatalf("len want %d got %d", len(want), len(got))
	}
	for i, kv := range got {
		if kv.Key != want[i] {
			t.Fatalf("idx %d want %d got %d", i, want[i], kv.Key)
		}
	}
}

func TestDeleteLeafOnly(t *testing.T) {
	tr := New(Config{Order: 5})
	tr.Insert(1, "")
	tr.Insert(2, "")
	if !tr.Delete(1) {
		t.Fatal("expected delete to succeed")
	}
	if _, ok := tr.Search(1); ok {
		t.Fatal("deleted key still present")
	}
	mustOK(t, tr)
}

func TestDeleteForcesBorrowAndMerge(t *testing.T) {
	tr := New(Config{Order: 4})
	for i := 1; i <= 30; i++ {
		tr.Insert(i, "")
	}
	mustOK(t, tr)
	for _, k := range []int{1, 2, 3, 4, 5, 6, 7, 8} {
		if !tr.Delete(k) {
			t.Fatalf("expected delete %d", k)
		}
		mustOK(t, tr)
	}
	remaining := tr.KeysInOrder()
	want := []int{}
	for i := 9; i <= 30; i++ {
		want = append(want, i)
	}
	if !sliceEq(remaining, want) {
		t.Fatalf("after deletes want %v got %v", want, remaining)
	}
}

func TestDeleteAllContractsRoot(t *testing.T) {
	tr := New(Config{Order: 4})
	for i := 1; i <= 12; i++ {
		tr.Insert(i, "")
	}
	for i := 1; i <= 12; i++ {
		if !tr.Delete(i) {
			t.Fatalf("delete %d failed", i)
		}
		mustOK(t, tr)
	}
	if tr.Size() != 0 {
		t.Fatalf("size should be 0 got %d", tr.Size())
	}
	if tr.Height() != 1 {
		t.Fatalf("height should be 1 got %d", tr.Height())
	}
}

func TestRandomizedFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 5; trial++ {
		order := 3 + rng.Intn(6)
		tr := New(Config{Order: order})
		shadow := map[int]string{}

		for op := 0; op < 800; op++ {
			k := rng.Intn(200)
			switch rng.Intn(3) {
			case 0, 1:
				v := fmt.Sprintf("v%d-%d", k, op)
				tr.Insert(k, v)
				shadow[k] = v
			case 2:
				if tr.Delete(k) {
					delete(shadow, k)
				} else if _, ok := shadow[k]; ok {
					t.Fatalf("delete missed key %d (had it in shadow)", k)
				}
			}
			if err := tr.CheckInvariants(); err != nil {
				t.Fatalf("trial %d op %d order %d: %v", trial, op, order, err)
			}
		}

		for k, v := range shadow {
			got, ok := tr.Search(k)
			if !ok || got != v {
				t.Fatalf("trial %d order %d: search %d want %q got %q ok=%v", trial, order, k, v, got, ok)
			}
		}
		var keys []int
		for k := range shadow {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		got := tr.KeysInOrder()
		if !sliceEq(keys, got) {
			t.Fatalf("trial %d order %d: keys mismatch\nwant %v\ngot  %v", trial, order, keys, got)
		}
	}
}

func TestLeafChainStaysSorted(t *testing.T) {
	tr := New(Config{Order: 4})
	keys := []int{50, 10, 30, 70, 90, 20, 40, 60, 80, 100, 5, 15, 25, 35, 45}
	for _, k := range keys {
		tr.Insert(k, "")
	}
	mustOK(t, tr)
	chain := tr.KeysInOrder()
	for i := 1; i < len(chain); i++ {
		if chain[i-1] >= chain[i] {
			t.Fatalf("chain not sorted: %v", chain)
		}
	}
}

func TestSnapshotShape(t *testing.T) {
	tr := New(Config{Order: 4})
	for _, k := range []int{10, 20, 30, 40, 50, 60, 70} {
		tr.Insert(k, fmt.Sprintf("v%d", k))
	}
	snap := tr.Snapshot()
	if snap.Order != 4 {
		t.Fatalf("order want 4 got %d", snap.Order)
	}
	if snap.Size != 7 {
		t.Fatalf("size want 7 got %d", snap.Size)
	}
	if len(snap.Levels) < 1 {
		t.Fatal("expected at least 1 level")
	}
	if len(snap.LeafChain) == 0 {
		t.Fatal("expected leaf chain to be populated")
	}
}

func TestTraceIsEmitted(t *testing.T) {
	tr := New(Config{Order: 4})
	for i := 1; i <= 10; i++ {
		tr.Insert(i, "")
	}
	_, _ = tr.Search(5)
	events := tr.Trace()
	if len(events) == 0 {
		t.Fatal("expected trace events from search")
	}
	if events[0].Type != "search_start" {
		t.Fatalf("first event want search_start got %s", events[0].Type)
	}
}

func sliceEq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
