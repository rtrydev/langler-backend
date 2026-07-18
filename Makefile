LAMBDAS := api
BUILD_DIR := build

.PHONY: all build test clean $(LAMBDAS)

all: build

build: $(LAMBDAS)

# Each Lambda is a self-contained arm64 bootstrap binary for provided.al2023,
# zipped for terraform to pick up from build/<name>.zip.
$(LAMBDAS):
	mkdir -p $(BUILD_DIR)/$@
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -trimpath \
		-ldflags "-s -w" -o $(BUILD_DIR)/$@/bootstrap ./cmd/$@
	cd $(BUILD_DIR)/$@ && zip -q ../$@.zip bootstrap

test:
	go test ./...

clean:
	rm -rf $(BUILD_DIR)
