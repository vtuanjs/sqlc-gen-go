.PHONY: build test

build:
	go build ./...

test: bin/sqlc-gen-go.wasm
	go test ./...

all: bin/sqlc-gen-go bin/sqlc-gen-go.wasm

bin/sqlc-gen-go: bin go.mod go.sum $(wildcard **/*.go)
	cd plugin && go build -o ../bin/sqlc-gen-go ./main.go

bin/sqlc-gen-go.wasm: bin/sqlc-gen-go
	cd plugin && GOOS=wasip1 GOARCH=wasm go build -o ../bin/sqlc-gen-go.wasm main.go
	@echo "SHA256: $$(sha256sum bin/sqlc-gen-go.wasm | awk '{print $$1}')"
	@echo "Update example/sqlc.yaml wasm.sha256 with the value above if it changed."

bin:
	mkdir -p bin

generate-example:
	cd example && sqlc generate
