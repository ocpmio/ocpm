APP_NAME := ocpm
MODULE := github.com/marian2js/ocpm
DIST_DIR := dist

.PHONY: fmt test build run clean snapshot npm-stage

fmt:
	go fmt ./...

test:
	go test ./...

build:
	mkdir -p $(DIST_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME) ./cmd/ocpm

run:
	go run ./cmd/ocpm

clean:
	rm -rf $(DIST_DIR)

snapshot:
	goreleaser release --snapshot --clean

npm-stage:
	node scripts/stage-npm-release.js

