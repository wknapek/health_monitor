MODULE   := ubi
BIN      := bin/monitor
PROTO_DIR := proto
PROTO_PKG := $(PROTO_DIR)/monitor/v1
VERSION  ?= dev
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build clean proto tidy vet

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/monitor

clean:
	rm -rf $(BIN)

tidy:
	go mod tidy

vet:
	go vet ./...

proto: install-tools
	protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=. --go_opt=module=$(MODULE) \
		--go-grpc_out=. --go-grpc_opt=module=$(MODULE) \
		$(PROTO_PKG)/monitor.proto

install-tools:
	@which protoc-gen-go      >/dev/null 2>&1 || go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@which protoc-gen-go-grpc >/dev/null 2>&1 || go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@which protoc             >/dev/null 2>&1 || (echo "protoc not found; install from https://github.com/protocolbuffers/protobuf/releases" && exit 1)

run: build
	./$(BIN) -config configs/devices.yaml
