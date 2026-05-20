.PHONY: build build-web build-go dev clean

build: build-web build-go

build-web:
	cd web/admin && npm install && npx vite build

build-go:
	go build -o server ./cmd/server/

dev:
	cd web/admin && npm run dev

clean:
	rm -rf web/dist server data/
