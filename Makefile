GO      ?= go
PROTOC  ?= protoc
VERSION ?= v0.1.0
LDFLAGS  = -s -w -X main.version=$(VERSION)

.PHONY: build test run vet fmt proto release clean

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
	$(PROTOC) -I api/proto \
		--go_out=. --go_opt=module=github.com/1daerduo/hardware-channel-abstraction \
		--go-grpc_out=. --go-grpc_opt=module=github.com/1daerduo/hardware-channel-abstraction \
		api/proto/channel/v1/channel.proto api/proto/channel/v1/service.proto

# release 交叉编译出全平台 elc 二进制（预编译分发，运维零依赖）
release:
	rm -rf dist
	mkdir -p dist
	GOOS=linux   GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o dist/elc-linux-amd64        ./cmd/elc
	GOOS=linux   GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o dist/elc-linux-arm64        ./cmd/elc
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o dist/elc-windows-amd64.exe  ./cmd/elc
	GOOS=darwin  GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o dist/elc-darwin-amd64       ./cmd/elc
	GOOS=darwin  GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o dist/elc-darwin-arm64       ./cmd/elc
	@echo "=== dist/ ==="
	@ls -lh dist/

clean:
	$(GO) clean
