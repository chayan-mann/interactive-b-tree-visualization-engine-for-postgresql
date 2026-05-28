package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/indexlab/indexlab/internal/bptree"
)

// BPTreeService wraps a single shared tree behind a mutex so the visualizer
// can stream a sequence of operations against a consistent state.
type BPTreeService struct {
	mu    sync.Mutex
	tree  *bptree.Tree
	opSeq uint64
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

type snapshotSummary struct {
	Order      int `json:"order"`
	Size       int `json:"size"`
	Height     int `json:"height"`
	RootPageID int `json:"rootPageId"`
	DiskReads  int `json:"diskReads"`
	DiskWrites int `json:"diskWrites"`
}

type operationMetrics struct {
	PathLength        int            `json:"pathLength"`
	NodeReads         int            `json:"nodeReads"`
	NodeWrites        int            `json:"nodeWrites"`
	EventCountsByType map[string]int  `json:"eventCountsByType"`
	InvariantOK       bool           `json:"invariantChecksPassed"`
}

type bulkOperation struct {
	Op    string `json:"op"`
	Key   *int   `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
	Lo    *int   `json:"lo,omitempty"`
	Hi    *int   `json:"hi,omitempty"`
	Label string `json:"label,omitempty"`
	Notes string `json:"notes,omitempty"`
}

type bulkStep struct {
	Step        int              `json:"step"`
	Op          string           `json:"op"`
	Label       string           `json:"label,omitempty"`
	Notes       string           `json:"notes,omitempty"`
	Key         *int             `json:"key,omitempty"`
	Lo          *int             `json:"lo,omitempty"`
	Hi          *int             `json:"hi,omitempty"`
	Metrics     operationMetrics `json:"metrics"`
	EventFrom   int              `json:"eventFrom"`
	EventTo     int              `json:"eventTo"`
	ResultFound *bool            `json:"resultFound,omitempty"`
	ResultCount *int             `json:"resultCount,omitempty"`
	Pre         *snapshotSummary `json:"pre,omitempty"`
	After       *snapshotSummary `json:"after,omitempty"`
}

type scenarioRun struct {
	Name      string      `json:"name,omitempty"`
	Total     int         `json:"totalSteps"`
	Steps     []bulkStep  `json:"steps"`
	Timestamp int64       `json:"timestampMs"`
}

// opResponse mirrors the previous response shape and adds additive learning metadata.
type opResponse struct {
	Operation string           `json:"operation"`
	Key       *int             `json:"key,omitempty"`
	Lo        *int             `json:"lo,omitempty"`
	Hi        *int             `json:"hi,omitempty"`
	Value     string           `json:"value,omitempty"`
	Found     *bool            `json:"found,omitempty"`
	Results   []bptree.KV      `json:"results,omitempty"`
	Trace     []bptree.Event    `json:"trace"`
	Snapshot  bptree.Snapshot  `json:"snapshot"`
	Metrics   *operationMetrics `json:"metrics,omitempty"`
	Pre       *snapshotSummary `json:"pre,omitempty"`
	After     *snapshotSummary `json:"after,omitempty"`
	Scenario  *scenarioRun     `json:"scenarioRun,omitempty"`
	EventGroups []bulkStep     `json:"eventGroups,omitempty"`
}

func (s *BPTreeService) nextOpID(op string) string {
	id := atomic.AddUint64(&s.opSeq, 1)
	return op + "-" + strconv.FormatUint(id, 10)
}

func snapshotSummaryFromSnapshot(s bptree.Snapshot) snapshotSummary {
	return snapshotSummary{
		Order:      s.Order,
		Size:       s.Size,
		Height:     s.Height,
		RootPageID: s.RootPageID,
		DiskReads:  s.DiskReads,
		DiskWrites: s.DiskWrites,
	}
}

func annotateEvents(events []bptree.Event, operationID, operation, label, notes string) []bptree.Event {
	for i := range events {
		events[i].OperationID = operationID
		events[i].EventPhase = operation
		if operation == "insert" || operation == "delete" || operation == "search" || operation == "range" {
			events[i].EventPhase = operation
		}
		switch {
		case events[i].Type == "search_start" ||
			events[i].Type == "search_hit" ||
			events[i].Type == "search_miss" ||
			events[i].Type == "range_start" ||
			events[i].Type == "range_visit_leaf" ||
			events[i].Type == "leaf_link_follow" ||
			events[i].Type == "range_end":
			events[i].EventPhase = "search"
		case strings.HasPrefix(events[i].Type, "borrow_") ||
			strings.HasPrefix(events[i].Type, "merge_") ||
			events[i].Type == "root_contract":
			events[i].EventPhase = "rebalance"
		case operation == "range":
			events[i].EventPhase = "search"
		default:
			events[i].EventPhase = operation
		}
		if label != "" {
			events[i].Label = label
		}
		if notes != "" {
			events[i].Notes = notes
		}
	}
	return events
}

func computeMetrics(pre, after bptree.Snapshot, events []bptree.Event, invariantOK bool) operationMetrics {
	counts := map[string]int{}
	pathLen := 0
	for _, e := range events {
		counts[e.Type]++
		if e.Type == "path" && pathLen == 0 {
			if n, ok := getInt(e.Details["pathLength"]); ok {
				pathLen = n
				continue
			}
			if p, ok := e.Details["nodePath"].([]int); ok {
				pathLen = len(p)
			}
		}
	}
	return operationMetrics{
		PathLength:        pathLen,
		NodeReads:         after.DiskReads - pre.DiskReads,
		NodeWrites:        after.DiskWrites - pre.DiskWrites,
		EventCountsByType: counts,
		InvariantOK:       invariantOK,
	}
}

func getInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
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
	pre := s.tree.Snapshot()
	s.tree = bptree.New(bptree.Config{Order: body.Order})
	post := s.tree.Snapshot()
	metrics := computeMetrics(pre, post, nil, true)
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "reset",
		Trace:     []bptree.Event{},
		Snapshot:  post,
		Metrics:   &metrics,
		Pre:       &snapshotSummaryFromSnapshot(pre),
		After:     &snapshotSummaryFromSnapshot(post),
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

	pre := s.tree.Snapshot()
	opID := s.nextOpID("insert")
	s.tree.Insert(body.Key, body.Value)
	post := s.tree.Snapshot()
	events := annotateEvents(s.tree.Trace(), opID, "insert", "insert", "")
	metrics := computeMetrics(pre, post, events, s.tree.CheckInvariants() == nil)
	k := body.Key
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "insert",
		Key:       &k,
		Value:     body.Value,
		Trace:     events,
		Snapshot:  post,
		Metrics:   &metrics,
		Pre:       &snapshotSummaryFromSnapshot(pre),
		After:     &snapshotSummaryFromSnapshot(post),
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

	pre := s.tree.Snapshot()
	opID := s.nextOpID("delete")
	k := body.Key
	found := s.tree.Delete(body.Key)
	post := s.tree.Snapshot()
	events := annotateEvents(s.tree.Trace(), opID, "delete", "delete", "")
	metrics := computeMetrics(pre, post, events, s.tree.CheckInvariants() == nil)
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "delete",
		Key:       &k,
		Found:     &found,
		Trace:     events,
		Snapshot:  post,
		Metrics:   &metrics,
		Pre:       &snapshotSummaryFromSnapshot(pre),
		After:     &snapshotSummaryFromSnapshot(post),
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

	pre := s.tree.Snapshot()
	opID := s.nextOpID("search")
	k := body.Key
	v, found := s.tree.Search(body.Key)
	post := s.tree.Snapshot()
	events := annotateEvents(s.tree.Trace(), opID, "search", "search", "")
	metrics := computeMetrics(pre, post, events, s.tree.CheckInvariants() == nil)
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "search",
		Key:       &k,
		Value:     v,
		Found:     &found,
		Trace:     events,
		Snapshot:  post,
		Metrics:   &metrics,
		Pre:       &snapshotSummaryFromSnapshot(pre),
		After:     &snapshotSummaryFromSnapshot(post),
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

	pre := s.tree.Snapshot()
	opID := s.nextOpID("range")
	lo, hi := body.Lo, body.Hi
	results := s.tree.RangeSearch(body.Lo, body.Hi)
	post := s.tree.Snapshot()
	events := annotateEvents(s.tree.Trace(), opID, "search", "range", "")
	metrics := computeMetrics(pre, post, events, s.tree.CheckInvariants() == nil)
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "range",
		Lo:        &lo,
		Hi:        &hi,
		Results:   results,
		Trace:     events,
		Snapshot:  post,
		Metrics:   &metrics,
		Pre:       &snapshotSummaryFromSnapshot(pre),
		After:     &snapshotSummaryFromSnapshot(post),
	})
}

// handleBulk runs a batch of inserts or scenario operations.
func (s *BPTreeService) handleBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Keys       []int           `json:"keys"`
		Reset      bool            `json:"reset"`
		Order      int             `json:"order"`
		Operations []bulkOperation `json:"operations"`
		Scenario   string          `json:"scenarioName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	pre := s.tree.Snapshot()
	if body.Reset {
		order := body.Order
		if order < 3 {
			order = s.tree.Order()
		}
		s.tree = bptree.New(bptree.Config{Order: order})
	}

	if len(body.Operations) > 0 {
		var allEvents []bptree.Event
		var steps []bulkStep
		for i, op := range body.Operations {
			step, stepEvents, err := s.runBulkOperation(op)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			step.Step = i + 1
			stepStart := len(allEvents)
			allEvents = append(allEvents, stepEvents...)
			step.EventFrom = stepStart
			step.EventTo = len(allEvents) - 1
			steps = append(steps, step)
		}
		post := s.tree.Snapshot()
		metrics := computeMetrics(pre, post, allEvents, s.tree.CheckInvariants() == nil)
		writeJSON(w, http.StatusOK, opResponse{
			Operation: "bulk",
			Trace:     allEvents,
			Snapshot:  post,
			Metrics:   &metrics,
			Pre:       &snapshotSummaryFromSnapshot(pre),
			After:     &snapshotSummaryFromSnapshot(post),
			Scenario: &scenarioRun{
				Name:      body.Scenario,
				Total:     len(steps),
				Steps:     steps,
				Timestamp: time.Now().UnixMilli(),
			},
			EventGroups: steps,
		})
		return
	}

	if len(body.Keys) == 0 {
		http.Error(w, "bulk requires keys[] or operations[]", http.StatusBadRequest)
		return
	}

	for _, k := range body.Keys {
		s.tree.Insert(k, "")
	}
	post := s.tree.Snapshot()
	metrics := computeMetrics(pre, post, nil, s.tree.CheckInvariants() == nil)
	writeJSON(w, http.StatusOK, opResponse{
		Operation: "bulk",
		Trace:     []bptree.Event{},
		Snapshot:  post,
		Metrics:   &metrics,
		Pre:       &snapshotSummaryFromSnapshot(pre),
		After:     &snapshotSummaryFromSnapshot(post),
	})
}

func (s *BPTreeService) runBulkOperation(op bulkOperation) (bulkStep, []bptree.Event, error) {
	operation := strings.ToLower(strings.TrimSpace(op.Op))
	step := bulkStep{
		Op:     operation,
		Label:  op.Label,
		Notes:  op.Notes,
		Key:    op.Key,
		Lo:     op.Lo,
		Hi:     op.Hi,
	}
	if step.Label == "" {
		step.Label = operation
	}
	pre := s.tree.Snapshot()
	step.Pre = &snapshotSummaryFromSnapshot(pre)
	opID := s.nextOpID(operation)

	switch operation {
	case "insert":
		if op.Key == nil {
			return step, nil, &operationError{"insert", "missing key"}
		}
		s.tree.Insert(*op.Key, op.Value)
	case "delete":
		if op.Key == nil {
			return step, nil, &operationError{"delete", "missing key"}
		}
		found := s.tree.Delete(*op.Key)
		foundCopy := found
		step.ResultFound = &foundCopy
	case "search":
		if op.Key == nil {
			return step, nil, &operationError{"search", "missing key"}
		}
		v, found := s.tree.Search(*op.Key)
		_ = v
		foundCopy := found
		step.ResultFound = &foundCopy
	case "range":
		if op.Lo == nil || op.Hi == nil {
			return step, nil, &operationError{"range", "missing lo/hi"}
		}
		results := s.tree.RangeSearch(*op.Lo, *op.Hi)
		count := len(results)
		step.ResultCount = &count
	default:
		return step, nil, &operationError{"bulk operation", "unsupported op: " + operation}
	}

	post := s.tree.Snapshot()
	events := annotateEvents(s.tree.Trace(), opID, operation, op.Label, op.Notes)
	metrics := computeMetrics(pre, post, events, s.tree.CheckInvariants() == nil)
	step.After = &snapshotSummaryFromSnapshot(post)
	step.Metrics = metrics
	if step.ResultCount == nil {
		step.ResultCount = nil
	}
	return step, events, nil
}

type operationError struct {
	operation string
	reason    string
}

func (e *operationError) Error() string {
	if e.reason == "" {
		return e.operation
	}
	return fmt.Sprintf("%s: %s", e.operation, e.reason)
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
