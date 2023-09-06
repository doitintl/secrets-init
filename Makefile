MODULE   = $(shell env GO111MODULE=on $(GO) list -m)
DATE    ?= $(shell date +%FT%T%z)
VERSION ?= $(shell git describe --tags --always --dirty --match="*" 2> /dev/null || \
			cat $(CURDIR)/.version 2> /dev/null || echo v0)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null)
BRANCH  ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
PKGS     = $(or $(PKG),$(shell env GO111MODULE=on $(GO) list ./...))
TESTPKGS = $(shell env GO111MODULE=on $(GO) list -f \
			'{{ if or .TestGoFiles .XTestGoFiles }}{{ .ImportPath }}{{ end }}' \
			$(PKGS))
BIN      = $(CURDIR)/.bin
LINT_CONFIG = $(CURDIR)/.golangci.yaml
PLATFORMS     = darwin linux
ARCHITECTURES = amd64 arm64
TARGETOS   ?= $(GOOS)
TARGETARCH ?= $(GOARCH)
LDFLAGS_VERSION = -s -w -X main.Version=$(VERSION) -X main.BuildDate=$(DATE) -X main.GitCommit=$(COMMIT) -X main.GitBranch=$(BRANCH)

DOCKER  = docker
GO      = go
TIMEOUT = 15
V = 0
Q = $(if $(filter 1,$V),,@)
M = $(shell printf "\033[34;1m▶\033[0m")

export GO111MODULE=on
export CGO_ENABLED=0
export GOPROXY=https://proxy.golang.org

.PHONY: all
all: fmt lint test ; $(info $(M) building executable...) @ ## Build program binary
	$Q env GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) $(GO) build \
		-tags release \
		-ldflags "$(LDFLAGS_VERSION) -X main.Platform=$(TARGETOS)/$(TARGETARCH)" \
		-o $(BIN)/$(basename $(MODULE)) main.go

# Release for multiple platforms

.PHONY: platfrom-build
platfrom-build: clean lint test ; $(info $(M) building binaries for multiple os/arch...) @ ## Build program binary for platforms and os
	$(foreach GOOS, $(PLATFORMS),\
		$(foreach GOARCH, $(ARCHITECTURES), \
			$(shell \
				GOPROXY=$(GOPROXY) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
				$(GO) build \
				-tags release \
				-ldflags "$(LDFLAGS_VERSION) -X main.Platform=$(GOOS)/$(GOARCH)" \
				-o $(BIN)/$(basename $(MODULE))-$(GOOS)-$(GOARCH) main.go || true)))

# Tools

setup-tools: setup-lint setup-gocov setup-gocov-xml setup-go2xunit setup-mockery setup-ghr

setup-lint:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.54.2
setup-gocov:
	$(GO) install github.com/axw/gocov/...
setup-gocov-xml:
	$(GO) install github.com/AlekSi/gocov-xml
setup-go2xunit:
	$(GO) install github.com/tebeka/go2xunit
setup-mockery:
	$(GO) install github.com/vektra/mockery/v2/
setup-ghr:
	$(GO) install github.com/tcnksm/ghr@v0.13.0

GOLINT=golangci-lint
GOCOV=gocov
GOCOVXML=gocov-xml
GO2XUNIT=go2xunit
GOMOCK=mockery

# Tests

TEST_TARGETS := test-default test-bench test-short test-verbose test-race
.PHONY: $(TEST_TARGETS) test-xml check test tests
test-bench:   ARGS=-run=__absolutelynothing__ -bench=. ## Run benchmarks
test-short:   ARGS=-short        ## Run only short tests
test-verbose: ARGS=-v            ## Run tests in verbose mode with coverage reporting
test-race:    ARGS=-race         ## Run tests with race detector
$(TEST_TARGETS): NAME=$(MAKECMDGOALS:test-%=%)
$(TEST_TARGETS): test
check test tests: fmt ; $(info $(M) running $(NAME:%=% )tests...) @ ## Run tests
	$Q $(GO) test -timeout $(TIMEOUT)s $(ARGS) $(TESTPKGS)

test-xml: fmt lint | $(GO2XUNIT) ; $(info $(M) running xUnit tests...) @ ## Run tests with xUnit output
	$Q mkdir -p test
	$Q 2>&1 $(GO) test -timeout $(TIMEOUT)s -v $(TESTPKGS) | tee test/tests.output
	$(GO2XUNIT) -fail -input test/tests.output -output test/tests.xml

COVERAGE_MODE    = atomic
COVERAGE_PROFILE = $(COVERAGE_DIR)/profile.out
COVERAGE_XML     = $(COVERAGE_DIR)/coverage.xml
COVERAGE_HTML    = $(COVERAGE_DIR)/index.html
.PHONY: test-coverage test-coverage-tools
test-coverage-tools: | $(GOCOV) $(GOCOVXML)
test-coverage: COVERAGE_DIR := $(CURDIR)/test/coverage.$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
test-coverage: fmt lint test-coverage-tools ; $(info $(M) running coverage tests...) @ ## Run coverage tests
	$Q mkdir -p $(COVERAGE_DIR)
	$Q $(GO) test \
		-coverpkg=$$($(GO) list -f '{{ join .Deps "\n" }}' $(TESTPKGS) | \
					grep '^$(MODULE)/' | \
					tr '\n' ',' | sed 's/,$$//') \
		-covermode=$(COVERAGE_MODE) \
		-coverprofile="$(COVERAGE_PROFILE)" $(TESTPKGS)
	$Q $(GO) tool cover -html=$(COVERAGE_PROFILE) -o $(COVERAGE_HTML)
	$Q $(GOCOV) convert $(COVERAGE_PROFILE) | $(GOCOVXML) > $(COVERAGE_XML)

.PHONY: lint
lint: setup-lint ; $(info $(M) running golangci-lint) @ ## Run golangci-lint
	$Q $(GOLINT) run --timeout=5m -v -c $(LINT_CONFIG) ./...

.PHONY: fmt
fmt: ; $(info $(M) running gofmt...) @ ## Run gofmt on all source files
	$Q $(GO) fmt $(PKGS)

.PHONY: mock
mock: ; $(info $(M) generating mocks...) @ ## Run mockery
	$Q $(GO) mod vendor -v
	$Q $(GOMOCK) --name SecretsManagerAPI --dir vendor/github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface
	$Q $(GOMOCK) --name SSMAPI --dir vendor/github.com/aws/aws-sdk-go/service/ssm/ssmiface
	$Q $(GOMOCK) --name GoogleSecretsManagerAPI --dir pkg/secrets/google
	$Q rm -rf vendor

# Misc

# generate CHANGELOG.md changelog file
.PHONY: changelog
changelog: ; $(info $(M) generating changelog...)	@ ## Generating CAHNGELOG.md
ifndef GITHUB_TOKEN
	$(error GITHUB_TOKEN is undefined)
endif
	$Q $(DOCKER) run --rm \
		-v $(CURDIR):/usr/local/src/app \
		-w /usr/local/src/app githubchangeloggenerator/github-changelog-generator \
		--user doitintl --project secrets-init \
		--token $(GITHUB_TOKEN)


.PHONY: clean
clean: ; $(info $(M) cleaning...)	@ ## Cleanup everything
	@rm -rf $(BIN)
	@rm -rf test/tests.* test/coverage.*

.PHONY: help
help:
	@grep -E '^[ a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.PHONY: version
version:
	@echo $(VERSION)
