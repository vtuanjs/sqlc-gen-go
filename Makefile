.PHONY: build test example-e2e example-e2e-postgres example-e2e-mysql example-e2e-sqlite example-e2e-setup example-e2e-down

build:
	go build ./...

test: bin/sqlc-gen-go.wasm
	go test ./internal/...
	cd example && go test ./test/... -v

all:
	make bin/sqlc-gen-go
	make bin/sqlc-gen-go.wasm

bin/sqlc-gen-go: bin go.mod go.sum $(wildcard **/*.go)
	cd plugin && go build -o ../bin/sqlc-gen-go ./main.go

bin/sqlc-gen-go.wasm: bin/sqlc-gen-go
	cd plugin && GOOS=wasip1 GOARCH=wasm go build -o ../bin/sqlc-gen-go.wasm main.go
	@echo "SHA256: $$(sha256sum bin/sqlc-gen-go.wasm | awk '{print $$1}')"
	@echo "Update example/sqlc.yaml wasm.sha256 with the value above if it changed."

bin:
	mkdir -p bin

generate-example:
	cd example && sqlc generate && go generate ./...

example-e2e-setup:
	docker compose -f example/e2e-setup/docker-compose.yml up -d --wait

example-e2e-down:
	docker compose -f $(CURDIR)/example/e2e-setup/docker-compose.yml down

define run-docker-e2e
	cd example && go test $(1) -count=1 -v; \
	EXIT=$$?; \
	$(MAKE) -C $(CURDIR) example-e2e-down; \
	exit $$EXIT
endef

example-e2e: example-e2e-setup
	$(call run-docker-e2e,./e2e-postgres/... ./e2e-mysql/... ./e2e-sqlite/...)

example-e2e-postgres: example-e2e-setup
	$(call run-docker-e2e,./e2e-postgres/...)

example-e2e-mysql: example-e2e-setup
	$(call run-docker-e2e,./e2e-mysql/...)

example-e2e-sqlite:
	cd example && go test ./e2e-sqlite/... -count=1 -v
