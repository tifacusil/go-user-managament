.PHONY: build vet test test-integration lint proto clean

build:
	go build ./...

vet:
	go vet ./...

test:
	go test -race ./internal/...

test-integration:
	go test -tags=integration -race ./test/integration/... -timeout 240s

lint:
	golangci-lint run ./...

proto:
	/tmp/protoc/bin/protoc \
	  --proto_path=proto \
	  --go_out=internal/grpc/usermanagementpb \
	  --go_opt=paths=source_relative \
	  --go-grpc_out=internal/grpc/usermanagementpb \
	  --go-grpc_opt=paths=source_relative \
	  --plugin=protoc-gen-go=$(shell go env GOPATH)/bin/protoc-gen-go \
	  --plugin=protoc-gen-go-grpc=$(shell go env GOPATH)/bin/protoc-gen-go-grpc \
	  usermanagement/v1/usermanagement.proto \
	&& mv internal/grpc/usermanagementpb/usermanagement/v1/* internal/grpc/usermanagementpb/ \
	&& rm -rf internal/grpc/usermanagementpb/usermanagement

clean:
	go clean -testcache
