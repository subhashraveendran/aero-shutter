BINARY  := aero-shutter
PKG     := ./cmd/aero-shutter
DIST    := dist
VERSION ?= $(shell date +%Y.%m.%d)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build vet test fmt cross clean \
        darwin-arm64 darwin-amd64 linux-amd64 windows-amd64

all: build

build:
	@mkdir -p $(DIST)
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY) $(PKG)

vet:
	go vet ./...

test:
	go test ./...

fmt:
	gofmt -w .

cross: darwin-arm64 darwin-amd64 linux-amd64 windows-amd64

darwin-arm64:
	@mkdir -p $(DIST)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-darwin-arm64 $(PKG)

darwin-amd64:
	@mkdir -p $(DIST)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-darwin-amd64 $(PKG)

linux-amd64:
	@mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-linux-amd64 $(PKG)

windows-amd64:
	@mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-windows-amd64.exe $(PKG)

clean:
	rm -rf $(DIST)
