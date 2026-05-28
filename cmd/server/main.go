// Command server runs the IndexLab HTTP API: the B+ tree visualizer engine
// plus the PostgreSQL index lab.
package main

import (
	"flag"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/indexlab/indexlab/internal/api"
	"github.com/indexlab/indexlab/internal/postgreslab"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	order := flag.Int("order", 4, "initial B+ tree order")
	dsn := flag.String("dsn", "", "PostgreSQL DSN (or set INDEXLAB_DSN env var)")
	flag.String("static", "./web/dist", "directory with the React build; leave empty to disable")
	flag.Parse()

	mux := http.NewServeMux()

	tree := api.NewBPTreeService(*order)
	tree.Register(mux)

	if *dsn == "" {
		*dsn = os.Getenv("INDEXLAB_DSN")
	}
	if *dsn != "" {
		lab, err := postgreslab.New(*dsn)
		if err != nil {
			log.Printf("postgres lab unavailable: %v", err)
			registerUnavailablePostgresLab(mux, "postgresql connection failed: "+err.Error())
		} else {
			labAPI := api.NewPostgresLabService(lab)
			labAPI.Register(mux)
			log.Printf("postgres lab connected to %s", redact(*dsn))
		}
	} else {
		log.Printf("INDEXLAB_DSN not set — PostgreSQL lab disabled")
		registerUnavailablePostgresLab(mux, "INDEXLAB_DSN not set")
	}

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	staticDir := flag.Lookup("static").Value.String()
	if staticDir != "" {
		if _, err := os.Stat(staticDir); err == nil {
			fs := http.FileServer(http.Dir(staticDir))
			mux.Handle("/", spaHandler(staticDir, fs))
			log.Printf("serving static frontend from %s", staticDir)
		} else {
			log.Printf("static dir %s not available: %v", staticDir, err)
		}
	}

	handler := api.AccessLog(api.CORS(mux))
	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatal(err)
	}
}

// spaHandler serves files from `dir` and falls back to index.html so the
// React SPA can handle client-side routing.
func spaHandler(dir string, fs http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			http.ServeFile(w, r, dir+"/index.html")
			return
		}
		// Try static asset; if missing, return index.html.
		full := dir + path
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, dir+"/index.html")
	})
}

func redact(dsn string) string {
	// Hide passwords if present in `postgres://user:pass@host` form.
	out := []byte(dsn)
	atIdx := -1
	for i, b := range out {
		if b == '@' {
			atIdx = i
			break
		}
	}
	if atIdx < 0 {
		return dsn
	}
	colonIdx := -1
	for i := 0; i < atIdx; i++ {
		if out[i] == ':' && i > 0 && string(out[max0(i-3):i]) != "://" {
			colonIdx = i
		}
	}
	if colonIdx > 0 && colonIdx < atIdx {
		for i := colonIdx + 1; i < atIdx; i++ {
			out[i] = '*'
		}
	}
	return string(out)
}

func max0(x int) int {
	if x < 0 {
		return 0
	}
	return x
}

// registerUnavailablePostgresLab wires stable JSON endpoints when Postgres lab
// is unavailable so the frontend does not receive SPA HTML fallback for /api/pglab/*.
func registerUnavailablePostgresLab(mux *http.ServeMux, reason string) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":  "postgres lab not configured",
			"reason": reason,
		})
	}
	mux.HandleFunc("/api/pglab/setup", handler)
	mux.HandleFunc("/api/pglab/seed", handler)
	mux.HandleFunc("/api/pglab/status", handler)
	mux.HandleFunc("/api/pglab/query", handler)
	mux.HandleFunc("/api/pglab/explain", handler)
	mux.HandleFunc("/api/pglab/indexes", handler)
	mux.HandleFunc("/api/pglab/index", handler)
	mux.HandleFunc("/api/pglab/compare", handler)
	mux.HandleFunc("/api/pglab/recommend", handler)
}
