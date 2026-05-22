.PHONY: build build-web build-go build-backend build-embedded dev test clean

WITH_FRONTEND ?= 1
SERVER_BIN ?= server

ifeq ($(WITH_FRONTEND),1)
BUILD_DEPS := build-web
GO_BUILD_TAGS := -tags frontend
else
BUILD_DEPS :=
GO_BUILD_TAGS :=
endif

build: $(BUILD_DEPS) build-go

build-web:
	cd web/admin && npm install && npx vite build

build-go:
	go build $(GO_BUILD_TAGS) -o $(SERVER_BIN) ./cmd/server/

build-backend:
	$(MAKE) build WITH_FRONTEND=0

build-embedded:
	$(MAKE) build WITH_FRONTEND=1

dev:
	cd web/admin && npm run dev

test:
	go test ./internal/... ./cmd/server

clean:
	rm -rf web/dist server data/
