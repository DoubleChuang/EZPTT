BIN      := ezptt
PKG      := ./cmd/ezptt
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: all build test race vet fmt lint docker clean

all: build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BIN) $(PKG)

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Fails when a file is not gofmt'ed: this is the same gate CI runs.
lint:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "$$unformatted"; echo "上面這些檔案沒有 gofmt"; exit 1; fi
	go vet ./...

docker:
	docker build --build-arg VERSION=$(VERSION) -t ezptt:$(VERSION) .

clean:
	rm -rf bin dist
