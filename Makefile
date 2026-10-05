.PHONY: build test lint clean fmt tidy proto docker docker-push ci help

GO ?= go
PROTOC ?= protoc
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
LDFLAGS ?= -s -w -X main.version=$(VERSION)
BINARY ?= metadata-musicbrainz

build:
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/module

test:
	$(GO) test -race -count=1 -timeout 60s ./...

lint:
	golangci-lint run --timeout 120s ./...

clean:
	rm -f $(BINARY)
	rm -f cmd/module/module
	rm -rf dist/

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

# Regenerates proto/musicbrainzv1/*.pb.go. Needs protoc 25.3,
# protoc-gen-go v1.36.6 and protoc-gen-go-grpc v1.5.1 on PATH.
proto:
	$(PROTOC) --go_out=. --go_opt=module=github.com/Muxcore-Media/metadata-musicbrainz \
		--go-grpc_out=. --go-grpc_opt=module=github.com/Muxcore-Media/metadata-musicbrainz \
		-I proto proto/muxcore/musicbrainz/v1/musicbrainz.proto

docker:
	docker build -t ghcr.io/muxcore-media/$(BINARY):$(VERSION) .
	docker tag ghcr.io/muxcore-media/$(BINARY):$(VERSION) ghcr.io/muxcore-media/$(BINARY):latest

docker-push: docker
	docker push ghcr.io/muxcore-media/$(BINARY):$(VERSION)
	docker push ghcr.io/muxcore-media/$(BINARY):latest

ci: lint test build

help:
	@echo "Targets:"
	@echo "  build       - compile the module binary"
	@echo "  test        - run tests with race detection"
	@echo "  lint        - golangci-lint"
	@echo "  proto       - regenerate gRPC code (protoc 25.3)"
	@echo "  clean       - remove build artifacts"
	@echo "  fmt         - format Go source"
	@echo "  tidy        - go mod tidy"
	@echo "  docker      - build Docker image"
	@echo "  docker-push - build and push Docker image"
	@echo "  ci          - lint + test + build"
