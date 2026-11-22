APP_NAME := learn-linux
CMD_PKG  := ./cmd/learn-linux
DIST_DIR := dist
LDFLAGS  := -s -w

.PHONY: all build test vet race clean dist

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) $(CMD_PKG)

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

dist:
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 $(CMD_PKG)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 $(CMD_PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe $(CMD_PKG)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 $(CMD_PKG)
	@ls -lh $(DIST_DIR)

clean:
	rm -rf $(DIST_DIR) $(APP_NAME) $(APP_NAME).exe
