MODULE  := github.com/arcrek/xray-vless-ws-go
BIN_DIR := bin
LDFLAGS := -s -w

.PHONY: build build-all build-linux-amd64 build-linux-arm64 build-windows-amd64 \
        build-darwin-amd64 build-darwin-arm64 build-android-arm64 \
        test vet fmt clean

# Local dev build: native GOOS/GOARCH, cgo enabled (whatever the host default is).
build:
	go build -o $(BIN_DIR)/xrayws ./cmd/xrayws

# All supported platforms, minus android/arm64 (see note in README.md — a
# transitive dependency currently fails to link on that target with
# CGO_ENABLED=0; tracked as a known limitation, not silently dropped).
build-all: build-linux-amd64 build-linux-arm64 build-windows-amd64 build-darwin-amd64 build-darwin-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/xrayws-linux-amd64 ./cmd/xrayws

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/xrayws-linux-arm64 ./cmd/xrayws

build-windows-amd64:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/xrayws-windows-amd64.exe ./cmd/xrayws

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/xrayws-darwin-amd64 ./cmd/xrayws

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/xrayws-darwin-arm64 ./cmd/xrayws

# Known-broken (see build-all's note); kept as its own target so it's easy
# to retry once the upstream wlynxg/anet/Go-toolchain incompatibility is
# resolved, without touching build-all's target list.
build-android-arm64:
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/xrayws-android-arm64 ./cmd/xrayws

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf $(BIN_DIR)
