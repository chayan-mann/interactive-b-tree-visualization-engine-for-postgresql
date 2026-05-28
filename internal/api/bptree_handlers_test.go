package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/indexlab/indexlab/internal/bptree"
)

func newTestServer() *httptest.Server {
	mux := http.NewServeMux()
	svc := NewBPTreeService(4)
	svc.Register(mux)
	return httptest.NewServer(mux)
}

func TestBPTreeAPIRoundTrip(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	post := func(path, body string) map[string]interface{} {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s -> %d", path, resp.StatusCode)
		}
		var m map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return m
	}

	post("/api/bptree/reset", `{"order":4}`)
	for _, k := range []int{10, 20, 5, 6, 12, 30, 7, 17} {
		post("/api/bptree/insert", `{"key":`+itoa(k)+`,"value":"row-`+itoa(k)+`"}`)
	}
	res := post("/api/bptree/search", `{"key":17}`)
	found, ok := res["found"].(bool)
	if !ok || !found {
		t.Fatalf("expected to find key 17, got %v", res)
	}
	if v, _ := res["value"].(string); v != "row-17" {
		t.Fatalf("value mismatch: %v", v)
	}
	rg := post("/api/bptree/range", `{"lo":5,"hi":17}`)
	results, _ := rg["results"].([]interface{})
	if len(results) == 0 {
		t.Fatalf("expected range results, got %v", rg)
	}
	del := post("/api/bptree/delete", `{"key":10}`)
	if del["found"].(bool) != true {
		t.Fatalf("expected delete to find key 10")
	}
}

func TestBulkSupportsScenarioOperations(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	postJSON := func(path string, payload interface{}, out interface{}) {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			text, _ := io.ReadAll(resp.Body)
			t.Fatalf("%s -> %d %s", path, resp.StatusCode, strings.TrimSpace(text))
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}

	type bulkStep struct {
		Step   int `json:"step"`
		Op     string
		Metrics struct {
			PathLength        int            `json:"pathLength"`
			NodeReads         int            `json:"nodeReads"`
			NodeWrites        int            `json:"nodeWrites"`
			EventCountsByType map[string]int  `json:"eventCountsByType"`
			InvariantOK       bool           `json:"invariantChecksPassed"`
		} `json:"metrics"`
		ResultFound *bool `json:"resultFound,omitempty"`
	}
	type scenarioRun struct {
		Steps  []bulkStep `json:"steps"`
		Total  int        `json:"totalSteps"`
		Name   string     `json:"name"`
	}
	type bulkResp struct {
		Scenario *scenarioRun     `json:"scenarioRun"`
		Metrics  *operationMetrics `json:"metrics"`
		Snapshot bptree.Snapshot   `json:"snapshot"`
		Trace    []bptree.Event    `json:"trace"`
	}

	var got bulkResp
	postJSON("/api/bptree/bulk", map[string]interface{}{
		"reset": true,
		"order": 4,
		"operations": []map[string]interface{}{
			{"op": "insert", "key": 10, "value": "row-10"},
			{"op": "insert", "key": 20, "value": "row-20"},
			{"op": "search", "key": 10},
			{"op": "delete", "key": 20},
			{"op": "range", "lo": 5, "hi": 12},
		},
	}, &got)

	if got.Scenario == nil {
		t.Fatal("expected scenarioRun in response")
	}
	if got.Scenario.Total != len(got.Scenario.Steps) {
		t.Fatalf("scenario total mismatch: %d vs %d", got.Scenario.Total, len(got.Scenario.Steps))
	}
	if got.Scenario.Total == 0 {
		t.Fatal("expected scenario steps")
	}
	if got.Scenario.Steps[2].ResultFound == nil || !*got.Scenario.Steps[2].ResultFound {
		t.Fatal("expected search step to find key 10")
	}

	steps := got.Scenario.Steps
	if steps[0].Metrics.PathLength <= 0 || steps[0].Metrics.NodeWrites == 0 {
		t.Fatalf("expected non-zero insert metrics, got %+v", steps[0].Metrics)
	}
	if steps[2].Metrics.EventCountsByType["search_hit"] == 0 {
		t.Fatalf("expected search hit event, got %+v", steps[2].Metrics.EventCountsByType)
	}
	if got.Snapshot.Size != 1 {
		t.Fatalf("expected one remaining key, got %d", got.Snapshot.Size)
	}
}

func TestBulkScenarioOperationsDeterministic(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	payload := map[string]interface{}{
		"reset": true,
		"order": 4,
		"operations": []map[string]interface{}{
			{"op": "insert", "key": 8, "value": "row-8"},
			{"op": "insert", "key": 15, "value": "row-15"},
			{"op": "delete", "key": 8},
		},
	}

	type metricsOnly struct {
		Steps []struct {
			Metrics struct {
				PathLength        int            `json:"pathLength"`
				NodeReads         int            `json:"nodeReads"`
				NodeWrites        int            `json:"nodeWrites"`
				EventCountsByType map[string]int  `json:"eventCountsByType"`
			} `json:"metrics"`
		} `json:"steps"`
	}

	postJSON := func(path string, payload interface{}, out interface{}) {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("request failed: %s", resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}

	var a metricsOnly
	postJSON("/api/bptree/bulk", payload, &a)
	var b metricsOnly
	postJSON("/api/bptree/bulk", payload, &b)

	if len(a.Steps) != len(b.Steps) {
		t.Fatalf("step count mismatch: %d vs %d", len(a.Steps), len(b.Steps))
	}
	for i := range a.Steps {
		if a.Steps[i].Metrics.PathLength != b.Steps[i].Metrics.PathLength {
			t.Fatalf("step %d path len mismatch: %d vs %d", i+1, a.Steps[i].Metrics.PathLength, b.Steps[i].Metrics.PathLength)
		}
		if a.Steps[i].Metrics.NodeWrites != b.Steps[i].Metrics.NodeWrites {
			t.Fatalf("step %d write mismatch: %d vs %d", i+1, a.Steps[i].Metrics.NodeWrites, b.Steps[i].Metrics.NodeWrites)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
