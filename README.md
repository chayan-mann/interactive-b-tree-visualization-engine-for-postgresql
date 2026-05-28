# Visualizing B+ Trees and PostgreSQL Query Plans

a learning-focused web tool that combines two things in one place:

1. **A B+ tree playground** — a from-scratch B+ tree engine written in Go,
   with insert / delete / search / range / split / merge / borrow visualised
   step-by-step in the browser.
2. **A PostgreSQL index lab** — a connected PostgreSQL instance you can seed
   with synthetic users, create and drop indexes against, and compare query
   plans before/after indexing with `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`.

The project demonstrates how the textbook data structure (B+ tree) actually
shows up inside a real database, and why PostgreSQL's planner sometimes
chooses a sequential scan, an index scan, a bitmap scan, or an index-only
scan.

## Architecture

```
React dashboard ─┐
                 ├─► Go HTTP API ─► B+ Tree engine (in-memory)
                 │                ─► Plan explainer (parses EXPLAIN JSON)
                 └───────────────► PostgreSQL lab ─► PostgreSQL
```

- Engine: [`internal/bptree`](internal/bptree)
- API server: [`cmd/server`](cmd/server) (+ [`internal/api`](internal/api))
- CLI REPL: [`cmd/bptree`](cmd/bptree)
- PostgreSQL lab: [`internal/postgreslab`](internal/postgreslab)
- Plan parser & recommender: [`internal/planexplainer`](internal/planexplainer)
- Frontend: [`web/`](web)

## Quick start

### 1. Run the Go API and the B+ tree visualizer (no PostgreSQL needed)

```bash
go run ./cmd/server -addr=:8080
# in another shell
cd web && npm install && npm run dev
# open http://localhost:5173
```

The Vite dev server proxies `/api/*` to the Go server. You can play with the
B+ tree immediately. The PostgreSQL panel will show a setup hint until you
connect a database.

### 2. Add PostgreSQL via Docker

```bash
docker compose up -d --build
# server is on http://localhost:8080
# postgres listens on host port 5433
```

The image bundles the compiled React app, so you can browse the whole UI at
[http://localhost:8080](http://localhost:8080).

### 3. Talk to your own PostgreSQL

```bash
INDEXLAB_DSN="postgres://USER:PASS@HOST:5432/DBNAME?sslmode=disable" \
  go run ./cmd/server
```

The lab limits write SQL to the dedicated `/api/pglab/*` endpoints; the
free-form query box only accepts read-only statements.

## Using the B+ tree CLI

```
go run ./cmd/bptree -order=4
B+ tree REPL (order=4). Type HELP for commands.
> INSERT 10
inserted 10 -> "row-10"
> INSERT 20
> INSERT 5
> SEARCH 20
found 20 -> "row-20"
> RANGE 5 25
  5 -> "row-5"
  10 -> "row-10"
  20 -> "row-20"
(3 rows)
> PRINT
[20] 
[5 10] [20] 
leaf chain: [5 10 20]
```

## API surface

### B+ tree

| Method | Path                       | Purpose                              |
|--------|----------------------------|--------------------------------------|
| GET    | `/api/bptree/snapshot`     | Current tree state                   |
| POST   | `/api/bptree/reset`        | Reset with a new order               |
| POST   | `/api/bptree/insert`       | `{key, value}`                       |
| POST   | `/api/bptree/delete`       | `{key}`                              |
| POST   | `/api/bptree/search`       | `{key}` -> snapshot + trace          |
| POST   | `/api/bptree/range`        | `{lo, hi}`                           |
| POST   | `/api/bptree/bulk`         | `{keys, reset, order}`               |

Every mutating endpoint returns the resulting snapshot **and** a list of
trace events (search path, splits, promotions, borrows, merges, leaf-link
follow). The frontend animates the trace one step at a time.

### PostgreSQL lab

| Method  | Path                       | Purpose                                                 |
|---------|----------------------------|---------------------------------------------------------|
| POST    | `/api/pglab/setup`         | Create the `users_demo` table                           |
| POST    | `/api/pglab/seed`          | Seed N synthetic rows                                   |
| GET     | `/api/pglab/status`        | Row count + indexes                                     |
| POST    | `/api/pglab/query`         | Run a read-only SQL statement                           |
| POST    | `/api/pglab/explain`       | `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` + parsed view |
| GET     | `/api/pglab/indexes`       | List indexes on `users_demo`                            |
| POST    | `/api/pglab/index`         | Create a single-column / composite / covering index     |
| DELETE  | `/api/pglab/index?name=…`  | Drop the named index                                    |
| POST    | `/api/pglab/compare`       | EXPLAIN, create index, EXPLAIN again, drop, diff        |
| POST    | `/api/pglab/recommend`     | Heuristic CREATE INDEX suggestion from a SELECT         |

## Sample experiments

[`scripts/experiments.sql`](scripts/experiments.sql) is a runnable script you
can step through in `psql`, or you can pick the same queries from the
**PostgreSQL lab** tab in the UI and use the **Compare before/after** button
to see the planner switch from `Seq Scan` to `Index Scan`/`Bitmap …`/
`Index Only Scan`.

Highlights you can demonstrate:

- Equality on `age` → sequential scan turns into a bitmap index scan once
  `idx_users_age` exists.
- `WHERE age BETWEEN 20 AND 30` shows how range queries traverse the linked
  B-tree leaves once an index is in place.
- A composite `(city, age)` index satisfies `city = 'Mumbai' AND age = 25`
  with a single index scan — the equality columns are pinned first.
- `CREATE INDEX idx_users_age ON users_demo(age) INCLUDE (username)` lets
  `SELECT age, username FROM users_demo WHERE age = 27` use an
  **Index Only Scan** (the covering index includes everything the SELECT
  needs).
- A predicate on `created_at` shows how time-window queries benefit from a
  B-tree index because adjacent timestamps live next to each other in the
  leaves.

## Project layout

```
.
├── cmd/
│   ├── bptree/         # CLI REPL for the engine
│   └── server/         # HTTP API + serves the React build
├── internal/
│   ├── api/            # HTTP handlers, CORS, access log
│   ├── bptree/         # B+ tree engine + tests + invariants
│   ├── planexplainer/  # EXPLAIN JSON parser + index recommender
│   └── postgreslab/    # PostgreSQL connection, seed, indexes
├── scripts/
│   ├── 01-pageinspect.sql  # init script for docker-compose
│   └── experiments.sql     # runnable experiments
├── web/                # React + TypeScript dashboard (Vite)
├── docker-compose.yml
├── Dockerfile
└── Makefile
```

## Tests

```bash
make test            # all Go tests including invariant + fuzz tests
cd web && npm run build  # type-checks the frontend
```

Notable test cases:

- [`internal/bptree/tree_test.go`](internal/bptree/tree_test.go) runs a
  randomized fuzz over 5 trees with varying orders, doing 800 random
  ops each and checking every invariant after every step.
- [`internal/planexplainer/explain_test.go`](internal/planexplainer/explain_test.go)
  parses representative EXPLAIN JSON for sequential, index, bitmap, and
  composite plans.
- [`internal/api/bptree_handlers_test.go`](internal/api/bptree_handlers_test.go)
  exercises the HTTP API end to end against an `httptest` server.

## Why this exists

This project proves understanding of:

- B+ tree structure (splits, merges, separators, linked leaves)
- Why databases use high-fanout trees
- Why range queries on B+ trees are efficient
- How PostgreSQL chooses between Sequential / Index / Bitmap / Index-Only
  scans
- Why indexes are not always used
- The gap between the textbook data structure and how a real database
  applies it
