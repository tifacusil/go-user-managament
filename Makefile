.PHONY: build vet test test-integration test-contract lint proto clean

build:
	go build ./...

vet:
	go vet ./...

test:
	go test -race ./internal/...

test-integration:
	go test -tags=integration -race ./test/integration/... -timeout 120s

test-contract:
	go run github.com/pact-foundation/pact-go/v2 install
	go test -tags=contract ./test/cdct/... -timeout 60s

lint:
	golangci-lint run ./...

proto:
	/tmp/protoc/bin/protoc \
	  --proto_path=proto \
	  --go_out=internal/grpc/alertrecipientpb \
	  --go_opt=paths=source_relative \
	  --go-grpc_out=internal/grpc/alertrecipientpb \
	  --go-grpc_opt=paths=source_relative \
	  --plugin=protoc-gen-go=$(shell go env GOPATH)/bin/protoc-gen-go \
	  --plugin=protoc-gen-go-grpc=$(shell go env GOPATH)/bin/protoc-gen-go-grpc \
	  alertrecipient/alert_recipient.proto \
	&& mv internal/grpc/alertrecipientpb/alertrecipient/* internal/grpc/alertrecipientpb/ \
	&& rmdir internal/grpc/alertrecipientpb/alertrecipient

clean:
	go clean -testcache
