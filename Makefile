BINARY=gamebridge-core

.PHONY: build fmt test install clean

build:
	go build -trimpath -o $(BINARY) ./cmd/gamebridge-core

fmt:
	gofmt -w ./cmd ./internal

test:
	go test ./...

install:
	sudo ./install.sh

clean:
	rm -f $(BINARY)
	rm -rf dist
