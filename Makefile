LAMBDAS := api
BUILD_DIR := build
GO_FILES := $(shell find . -type f -name '*.go' -not -path './build/*')

.PHONY: all build test race lint fmt vuln clean $(LAMBDAS)

all: build

build: $(LAMBDAS)

$(LAMBDAS):
	mkdir -p $(BUILD_DIR)/$@
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -trimpath \
		-ldflags "-s -w" -o $(BUILD_DIR)/$@/bootstrap ./cmd/$@
	cd $(BUILD_DIR)/$@ && zip -q ../$@.zip bootstrap

test:
	go test ./...

race:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	gofmt -w $(GO_FILES)
	goimports -w $(GO_FILES)

vuln:
	govulncheck ./...

clean:
	rm -rf $(BUILD_DIR)
