.PHONY: test race build proto lint

test:
	go test ./...

race:
	go test -race ./...

build:
	go build ./cmd/gateway ./cmd/pluginctl

proto:
	mkdir -p gen/go
	protoc -I proto --go_out=gen/go --go_opt=paths=source_relative --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative proto/gateway/v1/*.proto

lint:
	go vet ./...
