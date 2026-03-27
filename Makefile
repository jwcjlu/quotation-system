.PHONY: init
init:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
	go install github.com/google/wire/cmd/wire@latest

.PHONY: conf
conf:
	protoc --proto_path=./internal/conf --go_out=paths=source_relative:./internal/conf ./internal/conf/conf.proto

.PHONY: api
api:
	protoc --proto_path=./api --proto_path=./third_party \
		--go_out=paths=source_relative:./api \
		--go-http_out=paths=source_relative:./api \
		--go-grpc_out=paths=source_relative:./api \
		api/bom/v1/bom.proto \
		api/agent/v1/agent.proto
	protoc --proto_path=./api --proto_path=./third_party \
		--go_out=paths=source_relative:./api \
		api/conf/v1/conf.proto

.PHONY: wire
wire:
	cd cmd/server && wire

.PHONY: generate
generate: api wire

.PHONY: run
run:
	go run ./cmd/server/...

.PHONY: build
build:
	go build -o bin/server ./cmd/server/...
