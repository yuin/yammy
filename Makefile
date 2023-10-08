SOURCES = $(shell find . -type f -name '*.go')
APP=yammy
RM=rm -f
GOCILINT=golangci-lint

all: linux windows

linux: cmd/$(APP)/$(APP) ## Build a linux/amd64 binary

windows: cmd/$(APP)/$(APP).exe ## Build a windows/amd64 binary

macosx: cmd/$(APP)/$(APP)_macosx ## Build a darwin/amd64 binary

cmd/$(APP)/$(APP): $(SOURCES)
	cd cmd/$(APP) && GOOS=linux GOARCH=amd64 go build .

cmd/$(APP)/$(APP).exe: $(SOURCES)
	cd cmd/$(APP)/ && GOOS=windows GOARCH=amd64 go build .

cmd/$(APP)/$(APP)_macosx: $(SOURCES)
	cd cmd/$(APP) && GOOS=darwin GOARCH=amd64 go build -o $(APP)_macosx .

test: cover.html ## Run tests

cover.html: $(SOURCES)
	go test -coverprofile cover.out .
	go tool cover -html cover.out -o cover.html

.PHONY: lint
lint: ## Run lints
	$(GOCILINT) run -c .golangci.yml ./...

.PHONY: clean
clean: ## Remove generated files
	$(RM) cmd/$(APP)/$(APP) 
	$(RM) cmd/$(APP)/$(APP).exe
	$(RM) cmd/$(APP)/$(APP)_macosx
	$(RM) cover.*

.PHONY: help
help: ## Display this help message
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' Makefile | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-10s\033[0m %s\n", $$1, $$2}'
