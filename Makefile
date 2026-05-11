.PHONY: help test build run web-dev web-build docker-up docker-down clean

help:
	@echo "IndexLab make targets:"
	@echo "  make test         - run all Go tests"
	@echo "  make build        - build the server and CLI binaries into ./bin"
	@echo "  make run          - run the API server (needs INDEXLAB_DSN for the PG lab)"
	@echo "  make web-dev      - start the Vite dev server"
	@echo "  make web-build    - build the React app into web/dist"
	@echo "  make docker-up    - launch postgres + server via docker compose"
	@echo "  make docker-down  - stop docker compose stack"
	@echo "  make clean        - remove build artifacts"

test:
	go test ./...

build:
	mkdir -p bin
	go build -o bin/server ./cmd/server
	go build -o bin/bptree ./cmd/bptree

run:
	go run ./cmd/server

web-dev:
	cd web && npm install --no-audit --no-fund && npm run dev

web-build:
	cd web && npm install --no-audit --no-fund && npm run build

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down -v

clean:
	rm -rf bin web/dist
