package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
