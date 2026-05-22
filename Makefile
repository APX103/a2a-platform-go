.PHONY: build build-web build-go dev test clean

build: build-web build-go

build-web:
	cd web/admin && npm install && npx vite build

build-go:
	go build -tags frontend -o server ./cmd/server/

dev:
	cd web/admin && npm run dev

test:
	go test ./internal/... ./cmd/server

clean:
	rm -rf web/dist server data/
