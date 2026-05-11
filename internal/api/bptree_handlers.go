package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/indexlab/indexlab/internal/bptree"
)

// BPTreeService wraps a single shared tree behind a mutex so the visualizer
// can stream a sequence of operations against a consistent state.
type BPTreeService struct {
	mu   sync.Mutex
	tree *bptree.Tree
}

// NewBPTreeService creates a service with an empty tree of the given order.
func NewBPTreeService(order int) *BPTreeService {
	return &BPTreeService{tree: bptree.New(bptree.Config{Order: order})}
}

// Register attaches the bptree routes to the provided mux.
func (s *BPTreeService) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/bptree/snapshot", s.handleSnapshot)
	mux.HandleFunc("/api/bptree/reset", s.handleReset)
	mux.HandleFunc("/api/bptree/insert", s.handleInsert)
	mux.HandleFunc("/api/bptree/delete", s.handleDelete)
	mux.HandleFunc("/api/bptree/search", s.handleSearch)
	mux.HandleFunc("/api/bptree/range", s.handleRange)
	mux.HandleFunc("/api/bptree/bulk", s.handleBulk)
}

type opResponse struct {
	Operation string            `json:"operation"`
	Key       *int              `json:"key,omitempty"`
	Lo        *int              `json:"lo,omitempty"`
	Hi        *int              `json:"hi,omitempty"`
	Value     string            `json:"value,omitempty"`
	Found     *bool             `json:"found,omitempty"`
	Results   []bptree.KV       `json:"results,omitempty"`
	Trace     []bptree.Event    `json:"trace"`
	Snapshot  bptree.Snapshot   `json:"snapshot"`
}

func (s *BPTreeService) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.tree.Snapshot())
}

func (s *BPTreeService) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Order int `json:"order"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Order < 3 {
		body.Order = 4
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tree = bptree.New(bptree.Config{Order: body.Order})
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "reset",
		Snapshot:  s.tree.Snapshot(),
		Trace:     []bptree.Event{},
	})
}

func (s *BPTreeService) handleInsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Key   int    `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tree.Insert(body.Key, body.Value)
	k := body.Key
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "insert",
		Key:       &k,
		Value:     body.Value,
		Trace:     s.tree.Trace(),
		Snapshot:  s.tree.Snapshot(),
	})
}

func (s *BPTreeService) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Key int `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	found := s.tree.Delete(body.Key)
	k := body.Key
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "delete",
		Key:       &k,
		Found:     &found,
		Trace:     s.tree.Trace(),
		Snapshot:  s.tree.Snapshot(),
	})
}

func (s *BPTreeService) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Key int `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, found := s.tree.Search(body.Key)
	k := body.Key
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "search",
		Key:       &k,
		Value:     v,
		Found:     &found,
		Trace:     s.tree.Trace(),
		Snapshot:  s.tree.Snapshot(),
	})
}

func (s *BPTreeService) handleRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Lo int `json:"lo"`
		Hi int `json:"hi"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	results := s.tree.RangeSearch(body.Lo, body.Hi)
	lo, hi := body.Lo, body.Hi
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "range",
		Lo:        &lo,
		Hi:        &hi,
		Results:   results,
		Trace:     s.tree.Trace(),
		Snapshot:  s.tree.Snapshot(),
	})
}

// handleBulk runs a batch of inserts to seed the tree quickly.
func (s *BPTreeService) handleBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Keys  []int    `json:"keys"`
		Reset bool     `json:"reset"`
		Order int      `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if body.Reset {
		order := body.Order
		if order < 3 {
			order = s.tree.Order()
		}
		s.tree = bptree.New(bptree.Config{Order: order})
	}
	for _, k := range body.Keys {
		s.tree.Insert(k, "")
	}
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "bulk",
		Trace:     []bptree.Event{},
		Snapshot:  s.tree.Snapshot(),
	})
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
