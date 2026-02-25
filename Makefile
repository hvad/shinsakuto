# Variables
BINARY_DIR=bin
CMD_DIR=cmd
GO_FILES=$(shell find . -name "*.go")

# List of binaries to build based on folders in cmd/
#BINARIES=arbiter scheduler poller reactionner
BINARIES=arbiter

.PHONY: all build test clean run-scheduler run-poller

# Default target: build everything
all: test build

# Build all binaries
build: $(BINARIES)

$(BINARIES):
	@echo "Building $@..."
	@mkdir -p $(BINARY_DIR)
	@go build -o $(BINARY_DIR)/$@ ./$(CMD_DIR)/$@
	@echo " $@ built in $(BINARY_DIR)/"

# Run all unit tests
test:
	@echo "Running all tests..."
	@go test -v ./...

# Clean binaries and temporary files
clean:
	@echo "Cleaning up..."
	@rm -rf $(BINARY_DIR)
	@echo " Done."

# Help command
help:
	@echo "Go-Shinken Management Commands:"
	@echo "  make build      - Compile all binaries"
	@echo "  make test       - Run all unit tests"
	@echo "  make clean      - Remove binaries"
	@echo "  make all        - Test and Build everything"
