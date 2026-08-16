GO      ?= go
PROTOC  ?= protoc

.PHONY: build test run vet fmt proto clean

build:
	$(GO) build ./...

test:
	$(GO) test -timeout 60s ./...

run:
	$(GO) run ./examples/goldenpath

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

proto:
	$(PROTOC) -I api/proto --go_out=. --go_opt=module=example.com/embedded-loop-channel \
		api/proto/channel/v1/channel.proto

clean:
	$(GO) clean
