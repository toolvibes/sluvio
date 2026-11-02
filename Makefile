VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
HAPROXY_VERSION ?= 3.2

.PHONY: all ui build test lint run docker clean

all: ui build

ui:
	cd web && npm ci && npm run build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o sluvio ./cmd/sluvio

test:
	go test ./...

lint:
	gofmt -l . | (! grep .) && go vet ./...

# Run against the example config in testdata (read-only, no HAProxy needed).
run: build
	./sluvio -config testdata/realworld.cfg -data-dir ./.data -read-only -listen 127.0.0.1:8484

docker:
	docker build --build-arg HAPROXY_VERSION=$(HAPROXY_VERSION) --build-arg VERSION=$(VERSION) -t sluvio:haproxy$(HAPROXY_VERSION) .

clean:
	rm -rf sluvio .data web/dist/assets web/dist/index.html
