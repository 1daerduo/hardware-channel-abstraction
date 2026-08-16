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
	$(PROTOC) -I api/proto --go_out=. --go_opt=module=github.com/1daerduo/hardware-channel-abstraction \
		api/proto/channel/v1/channel.proto

clean:
	$(GO) clean
