SHELL := /bin/sh
.SHELLFLAGS := -eu -c
.DEFAULT_GOAL := help
.DELETE_ON_ERROR:

BINARY := opsmate
MODULE := github.com/HediAbed/opsmate
VERSION_FILE := VERSION
VERSION := $(strip $(shell sed -n '1p' $(VERSION_FILE) 2>/dev/null))
DIST_DIR := dist
TOOLS_DIR := $(CURDIR)/.tools
COVERAGE_TARGET := 100.0
GO_VERSION := $(strip $(shell awk '$$1 == "go" { print $$2; exit }' go.mod))
GO_TOOLCHAIN := go$(GO_VERSION)
BUILD_FLAGS := -trimpath -buildvcs=false
LINK_FLAGS := -s -w
GO_FILES := $(shell git ls-files --cached --others --exclude-standard '*.go' | \
	while IFS= read -r go_file; do if [ -f "$$go_file" ]; then printf '%s ' "$$go_file"; fi; done)

GOLANGCI_LINT_VERSION := v2.13.1
STATICCHECK_VERSION := v0.8.1
GOVULNCHECK_VERSION := v1.7.0
DEADCODE_VERSION := v0.47.0
GITLEAKS_VERSION := v8.30.1
ACTIONLINT_VERSION := v1.7.12

GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint
STATICCHECK := $(TOOLS_DIR)/staticcheck
GOVULNCHECK := $(TOOLS_DIR)/govulncheck
DEADCODE := $(TOOLS_DIR)/deadcode
GITLEAKS := $(TOOLS_DIR)/gitleaks
ACTIONLINT := $(TOOLS_DIR)/actionlint

HOST_OS := $(shell go env GOOS)
HOST_ARCH := $(shell go env GOARCH)
HOST_SUFFIX := $(if $(filter windows,$(HOST_OS)),.exe,)
OS ?= $(HOST_OS)
ARCH ?= $(HOST_ARCH)
TARGET_SUFFIX = $(if $(filter windows,$(OS)),.exe,)
TARGET_BINARY = $(BINARY)-$(OS)-$(ARCH)$(TARGET_SUFFIX)
SUPPORTED_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

export GOTOOLCHAIN := $(GO_TOOLCHAIN)
export GOFLAGS := -mod=readonly

.PHONY: help build run clean tools \
	fmt fmt-check tidy tidy-check mod-verify test test-windows coverage race vet lint staticcheck deadcode vuln secrets workflow-lint \
	repository-check version-check check \
	linux linux-amd64 linux-arm64 mac darwin-amd64 darwin-arm64 windows windows-amd64 \
	cross-build docker-build

help: ## Show available targets.
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: version-check ## Build the host binary.
	@temporary_binary=$$(mktemp "./.$(BINARY).build.XXXXXX"); \
	trap 'rm -f "$$temporary_binary"' EXIT HUP INT TERM; \
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -ldflags='$(LINK_FLAGS)' -o "$$temporary_binary" .; \
	mv "$$temporary_binary" "./$(BINARY)$(HOST_SUFFIX)"

run: build ## Build and run OpsMate.
	./$(BINARY)$(HOST_SUFFIX)

fmt: ## Format Go source files.
	gofmt -w $(GO_FILES)

fmt-check: ## Fail when Go source files are not formatted.
	@unformatted=$$(gofmt -l $(GO_FILES)); \
	if [ -n "$$unformatted" ]; then \
		printf 'unformatted Go files:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi

tidy: ## Update the module files.
	GOFLAGS= go mod tidy

tidy-check: ## Fail when the module files are not tidy.
	go mod tidy -diff

mod-verify: ## Verify downloaded module content.
	go mod verify

test: ## Run deterministic unit and integration tests.
	go test -count=1 -shuffle=on ./...

test-windows: ## Compile every test package for Windows.
	@output_directory=$$(mktemp -d "$${TMPDIR:-/tmp}/opsmate-windows-tests.XXXXXX"); \
	trap 'find "$$output_directory" -mindepth 1 -maxdepth 1 -type f -delete; rmdir "$$output_directory"' EXIT HUP INT TERM; \
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -c -o "$$output_directory/" ./...

coverage: ## Require exact aggregate statement coverage.
	@coverage_workspace=$$(mktemp -d "$${TMPDIR:-/tmp}/opsmate-coverage.XXXXXX"); \
	trap 'find "$$coverage_workspace" -depth -delete' EXIT HUP INT TERM; \
	mkdir "$$coverage_workspace/integration"; \
	go test -count=1 -covermode=atomic -coverpkg=./... -coverprofile="$$coverage_workspace/unit.out" ./...; \
	go build -cover -covermode=atomic -coverpkg=./... $(BUILD_FLAGS) -o "$$coverage_workspace/$(BINARY)" .; \
	actual_version=$$(GOCOVERDIR="$$coverage_workspace/integration" "$$coverage_workspace/$(BINARY)" --version 2>"$$coverage_workspace/version.err"); \
	test "$$actual_version" = "$(BINARY) $(VERSION)"; \
	test ! -s "$$coverage_workspace/version.err"; \
	go tool covdata textfmt -i="$$coverage_workspace/integration" -o="$$coverage_workspace/integration.out"; \
	awk 'FNR == 1 { next } { key = $$1 " " $$2; if (!(key in counts) || $$3 > counts[key]) counts[key] = $$3 } END { for (key in counts) print key, counts[key] }' \
		"$$coverage_workspace/unit.out" "$$coverage_workspace/integration.out" | sort > "$$coverage_workspace/blocks.out"; \
	{ printf 'mode: atomic\n'; sed -n '1,$$p' "$$coverage_workspace/blocks.out"; } > "$$coverage_workspace/merged.out"; \
	uncovered_blocks=$$(awk 'NR > 1 && $$2 > 0 && $$3 == 0 { count++ } END { print count + 0 }' "$$coverage_workspace/merged.out"); \
	if [ "$$uncovered_blocks" -ne 0 ]; then \
		printf 'uncovered instrumented blocks: %s\n' "$$uncovered_blocks" >&2; \
		awk 'NR > 1 && $$2 > 0 && $$3 == 0 { print $$1 }' "$$coverage_workspace/merged.out" >&2; \
		exit 1; \
	fi; \
	coverage=$$(go tool cover -func="$$coverage_workspace/merged.out" | awk '/^total:/ {gsub(/%/, "", $$3); print $$3}'); \
	printf 'aggregate coverage: %s%%\n' "$$coverage"; \
	test "$$coverage" = "$(COVERAGE_TARGET)"

race: ## Run tests with the race detector.
	CGO_ENABLED=1 go test -count=1 -race ./...

vet: ## Run the Go vet analyzer.
	go vet ./...

lint: $(GOLANGCI_LINT) ## Run the pinned lint suite.
	$(GOLANGCI_LINT) run --timeout=5m ./...

staticcheck: $(STATICCHECK) ## Run the pinned static analyzer.
	$(STATICCHECK) ./...

deadcode: $(DEADCODE) ## Reject unreachable functions, including test-only APIs.
	@findings=$$($(DEADCODE) -test ./...); \
	if [ -n "$$findings" ]; then \
		printf 'unreachable functions:\n%s\n' "$$findings"; \
		exit 1; \
	fi

vuln: $(GOVULNCHECK) ## Check reachable code for known vulnerabilities.
	$(GOVULNCHECK) ./...

secrets: $(GITLEAKS) ## Scan repository history and present source files for secrets.
	$(GITLEAKS) git --redact --no-banner .
	@scan_directory=$$(mktemp -d "$${TMPDIR:-/tmp}/opsmate-secret-scan.XXXXXX"); \
	trap 'find "$$scan_directory" -depth -delete' EXIT HUP INT TERM; \
	git ls-files --cached --others --exclude-standard -z | \
		tar --null --files-from=- --ignore-failed-read -cf - 2>/dev/null | \
		tar -xf - -C "$$scan_directory"; \
	$(GITLEAKS) dir --redact --no-banner "$$scan_directory"

workflow-lint: $(ACTIONLINT) ## Validate GitHub Actions workflows.
	$(ACTIONLINT)

tools: $(GOLANGCI_LINT) $(STATICCHECK) $(DEADCODE) $(GOVULNCHECK) $(GITLEAKS) $(ACTIONLINT) ## Install pinned development tools.

$(GOLANGCI_LINT):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(TOOLS_DIR)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(STATICCHECK):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(TOOLS_DIR)" go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

$(DEADCODE):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(TOOLS_DIR)" go install golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION)

$(GOVULNCHECK):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(TOOLS_DIR)" go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(GITLEAKS):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(TOOLS_DIR)" go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)

$(ACTIONLINT):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(TOOLS_DIR)" go install github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

version-check: ## Validate the release version.
	@printf '%s\n' "$(VERSION)" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$$' || \
		{ printf 'VERSION must contain a semantic version\n' >&2; exit 1; }

repository-check: version-check ## Validate repository metadata and file modes.
	@git diff --check
	@indexed_modes=$$(git ls-files -s | while read -r mode object stage tracked_file; do \
		if [ -f "$$tracked_file" ] && [ "$$mode" != "100644" ]; then printf '%s (%s)\n' "$$tracked_file" "$$mode"; fi; \
	done); \
	if [ -n "$$indexed_modes" ]; then \
		printf 'tracked files must use index mode 100644:\n%s\n' "$$indexed_modes"; \
		exit 1; \
	fi
	@untracked_executables=$$(git ls-files --others --exclude-standard | while IFS= read -r untracked_file; do \
		if [ -f "$$untracked_file" ] && [ -x "$$untracked_file" ]; then printf '%s\n' "$$untracked_file"; fi; \
	done); \
	if [ -n "$$untracked_executables" ]; then \
		printf 'untracked source files must not be executable:\n%s\n' "$$untracked_executables"; \
		exit 1; \
	fi
	@test "$$(go list -m)" = "$(MODULE)"
	@grep -q '^FROM golang:$(GO_VERSION)-' Dockerfile.build
	@forbidden_pattern='co-authored''-by|generated[ -]by (chat''gpt|cop''ilot|claude)|chat''gpt|github cop''ilot|claude co''de|ai-''generated'; \
	set +e; \
	residue=$$(git grep -I --untracked -nEI "$$forbidden_pattern" -- . ':!go.sum'); \
	grep_status=$$?; \
	set -e; \
	if [ "$$grep_status" -gt 1 ]; then exit "$$grep_status"; fi; \
	if [ -n "$$residue" ]; then printf 'forbidden development attribution or tooling residue:\n%s\n' "$$residue"; exit 1; fi
	@set +e; \
	emails=$$(git grep -I --untracked -nE '[[:alnum:]._%+-]+@[[:alnum:].-]+\.[A-Za-z]{2,}' -- . ':!go.sum'); \
	grep_status=$$?; \
	set -e; \
	if [ "$$grep_status" -gt 1 ]; then exit "$$grep_status"; fi; \
	if [ -n "$$emails" ]; then printf 'email addresses are not allowed:\n%s\n' "$$emails"; exit 1; fi

check: fmt-check tidy-check mod-verify test test-windows coverage race vet staticcheck deadcode lint vuln secrets workflow-lint repository-check ## Run every local quality gate.

$(DIST_DIR):
	@mkdir -p "$(DIST_DIR)"

define cross_build
$(1)-$(2): version-check | $(DIST_DIR)
	CGO_ENABLED=0 GOOS=$(1) GOARCH=$(2) go build $(BUILD_FLAGS) -ldflags='$(LINK_FLAGS)' -o "$(DIST_DIR)/$(BINARY)-$(1)-$(2)$(if $(filter windows,$(1)),.exe,)" .
endef

$(eval $(call cross_build,linux,amd64))
$(eval $(call cross_build,linux,arm64))
$(eval $(call cross_build,darwin,amd64))
$(eval $(call cross_build,darwin,arm64))
$(eval $(call cross_build,windows,amd64))

linux: linux-amd64 linux-arm64 ## Build Linux release binaries.

mac: darwin-amd64 darwin-arm64 ## Build macOS release binaries.

windows: windows-amd64 ## Build the Windows release binary.

cross-build: linux mac windows ## Build every supported release binary.

docker-build: version-check | $(DIST_DIR) ## Build one target with the pinned container image.
	@case " $(SUPPORTED_TARGETS) " in \
		*" $(OS)/$(ARCH) "*) ;; \
		*) printf 'unsupported target: %s/%s\n' "$(OS)" "$(ARCH)" >&2; exit 1 ;; \
	esac; \
	temporary_output=$$(mktemp -d "$(DIST_DIR)/.docker-$(OS)-$(ARCH).XXXXXX"); \
	trap 'find "$$temporary_output" -mindepth 1 -maxdepth 1 -type f -delete; rmdir "$$temporary_output"' EXIT HUP INT TERM; \
	docker build \
		--file Dockerfile.build \
		--target export \
		--build-arg GOOS="$(OS)" \
		--build-arg GOARCH="$(ARCH)" \
		--build-arg OUTPUT_NAME="$(TARGET_BINARY)" \
		--output "type=local,dest=$$temporary_output" \
		.; \
	mv "$$temporary_output/$(TARGET_BINARY)" "$(DIST_DIR)/$(TARGET_BINARY)"

clean: ## Remove only known generated files.
	@rm -f -- "./$(BINARY)" "./$(BINARY).exe" ./coverage.out ./coverage-*.out
	@rm -f -- \
		"$(DIST_DIR)/$(BINARY)-linux-amd64" \
		"$(DIST_DIR)/$(BINARY)-linux-arm64" \
		"$(DIST_DIR)/$(BINARY)-darwin-amd64" \
		"$(DIST_DIR)/$(BINARY)-darwin-arm64" \
		"$(DIST_DIR)/$(BINARY)-windows-amd64.exe" \
		"$(GOLANGCI_LINT)" "$(STATICCHECK)" "$(DEADCODE)" "$(GOVULNCHECK)" "$(GITLEAKS)" "$(ACTIONLINT)"
	@for directory in "$(DIST_DIR)" "$(TOOLS_DIR)"; do \
		if [ -d "$$directory" ] && [ -z "$$(find "$$directory" -mindepth 1 -print -quit)" ]; then \
			rmdir "$$directory"; \
		fi; \
	done
