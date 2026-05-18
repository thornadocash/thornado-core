########################################################################################
# Environment Checks
########################################################################################

CHECK_ENV:=$(shell ./scripts/check-env.sh)
ifneq ($(CHECK_ENV),)
$(error Check environment dependencies.)
endif

########################################################################################
# Config
########################################################################################

.PHONY: build test tools export healthcheck run-mocknet build-mocknet stop-mocknet halt-mocknet ps-mocknet reset-mocknet logs-mocknet openapi test-performance test-performance-quick test-performance-all test-performance-analyze test-performance-profile-cpu test-performance-profile-mem test-performance-trace test-performance-clean

# mocknet docker compose files and overrides for macOS
DOCKER_COMPOSE_FILES ?= -f build/docker/docker-compose.yml
ifeq ($(shell uname -s),Darwin)
DOCKER_COMPOSE_FILES += -f build/docker/docker-compose.darwin.yml
endif

# pull branch name from CI if unset and available
ifdef CI_COMMIT_BRANCH
BRANCH?=${CI_COMMIT_BRANCH}
BUILDTAG?=${CI_COMMIT_BRANCH}
endif

# image build settings
COMMIT?=$(shell git log -1 --format='%H' 2>/dev/null)
BRANCH?=$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GITREF?=$(shell git rev-parse --short HEAD 2>/dev/null)
BUILDTAG?=$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)

# compiler flags
VERSION:=$(shell cat version)
TAG?=mocknet
ldflags = -X gitlab.com/thorchain/thornode/v3/constants.Version=$(VERSION) \
      -X gitlab.com/thorchain/thornode/v3/constants.GitCommit=$(COMMIT) \
      -X github.com/cosmos/cosmos-sdk/version.Name=THORChain \
      -X github.com/cosmos/cosmos-sdk/version.AppName=thornode \
      -X github.com/cosmos/cosmos-sdk/version.Version=$(VERSION) \
      -X github.com/cosmos/cosmos-sdk/version.Commit=$(COMMIT) \
      -X github.com/cosmos/cosmos-sdk/version.BuildTags=$(TAG) \
      -buildid=

# golang settings
TEST_PATHS=$(shell go list ./... | grep -v bifrost/tss/go-tss) # Skip compute-intensive tests by default

# Parse test path argument from command line (e.g., "make test ./path")
ifeq (test,$(firstword $(MAKECMDGOALS)))
  TEST_PATH_ARG := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
  ifneq ($(TEST_PATH_ARG),)
    # Check if the argument looks like a path (contains /)
    ifneq ($(findstring /,$(TEST_PATH_ARG)),)
      # Ensure path starts with ./
      ifeq ($(filter ./%,$(TEST_PATH_ARG)),)
        TEST_DIR := ./$(TEST_PATH_ARG)
      else
        TEST_DIR := $(TEST_PATH_ARG)
      endif
      # Prevent make from treating the path as a target
      .PHONY: $(TEST_PATH_ARG)
      $(eval $(TEST_PATH_ARG):;@:)
    endif
  endif
endif

TEST_DIR?=${TEST_PATHS}
BUILD_FLAGS := -ldflags '$(ldflags)' -tags ${TAG} -trimpath
PARALLELISM ?= 1
TEST_BUILD_FLAGS := -parallel=$(PARALLELISM) -tags=mocknet
ifdef RUN
RUN_FLAG := -run=$(RUN)
endif
BINARIES?=./cmd/thornode ./cmd/bifrost
GOVERSION=$(shell awk '($$1 == "go") { print $$2 }' go.mod)

# docker tty args are disabled in CI
ifndef CI
DOCKER_TTY_ARGS=-it
endif

HTTPS_GIT := https://gitlab.com/thorchain/thornode.git

########################################################################################
# Targets
########################################################################################

# ------------------------------ Generate ------------------------------

generate: go-generate generate-abi openapi proto-gen
	@./scripts/generate.py

go-generate:
	@go install golang.org/x/tools/cmd/stringer@v0.28.0
	@go generate ./...

# mirror router abi from single source of truth to reference locations
generate-abi:
	@echo "EVM router ABI generation is disabled in the Thornado THORNode fork"

openapi:
	@docker run --rm \
		--user $(shell id -u):$(shell id -g) \
		-v $$PWD/openapi:/mnt \
		openapitools/openapi-generator-cli:v6.0.0@sha256:310bd0353c11863c0e51e5cb46035c9e0778d4b9c6fe6a7fc8307b3b41997a35 \
		generate -i /mnt/openapi.yaml -g go -o /mnt/gen
	@rm openapi/gen/go.mod openapi/gen/go.sum
	@find ./openapi/gen -type f | xargs sed -i '/^[- ]*API version.*$(shell cat version)/d;/APIClient.*$(shell cat version)/d'
	@find ./openapi/gen -type f | grep model | xargs sed -i 's/MarshalJSON(/MarshalJSON_deprecated(/'

protoVer=0.13.2
protoImageName=ghcr.io/cosmos/proto-builder:$(protoVer)
protoImage=docker run --rm -v $(CURDIR):/workspace --workdir /workspace $(protoImageName)

proto-all: proto-format proto-lint proto-gen format

proto-gen:
	@echo "Generating Protobuf files"
	@$(protoImage) sh ./scripts/protocgen.sh

proto-format:
	@echo "Formatting Protobuf files"
	@$(protoImage) find ./ -name "*.proto" -exec clang-format -i {} \;

proto-format-check:
	@echo "Checking Protobuf formatting"
	@find ./ -name "*.proto" -print0 | xargs -0L1 $(protoImage) clang-format --dry-run -Werror

proto-lint:
	@$(protoImage) sh -c "cd proto/thorchain/v1 && buf lint --error-format=json"

proto-check-breaking:
	@$(protoImage) sh -c "cd proto/thorchain/v1 && buf breaking --against $(HTTPS_GIT)#branch=develop,subdir=proto/thorchain/v1"

# ------------------------------ Docs ------------------------------

docs-init:
	@cargo install mdbook --version 0.4.52
	@cargo install mdbook-admonish --version 1.18.0
	@cargo install mdbook-katex --version 0.9.4
	@cargo install mdbook-embed --version 0.2.0
	@cargo install mdbook-mermaid --version 0.14.0
	@cd docs && mdbook-admonish install --css-dir theme
	@cd docs && mdbook-mermaid install

docs-generate: docs-init
	@cd docs && mdbook build -d ../public

docs-dev: docs-init
	@cd docs && mdbook serve -d ../public --open

# ------------------------------ Build ------------------------------

build:
	go build ${BUILD_FLAGS} ${BINARIES}

install:
	go install ${BUILD_FLAGS} ${BINARIES}

tools:
	@echo "operator tools are not part of the Thornado THORNode fork"

# ------------------------------ Gitlab CI ------------------------------

gitlab-trigger-ci:
	@./scripts/gitlab-trigger-ci.sh

# ------------------------------ Housekeeping ------------------------------

format:
	@git ls-files '*.go' | grep -v -e '^docs/' -e '^api/' -e '^openapi/gen/' -e '.pb.go$$' -e '.pb.gw.go$$' -e '_gen.go$$' -e 'wire_gen.go$$' |\
		xargs gofumpt -w

lint:
	@./scripts/lint.sh
	@./scripts/trunk check --no-fix --show-existing --upstream origin/develop

lint-ci:
	@./scripts/lint.sh
	@./scripts/trunk-ci.sh

# ------------------------------ Unit Tests ------------------------------

test-coverage: test-network-specific
	@go test ${TEST_BUILD_FLAGS} -v -coverprofile=coverage.txt -covermode count ${TEST_DIR}
	sed -i '/\.pb\.go:/d' coverage.txt

coverage-report: test-coverage
	@go tool cover -html=coverage.txt

test-coverage-sum: test-network-specific
	@go run gotest.tools/gotestsum --junitfile report.xml --format testname -- ${TEST_BUILD_FLAGS} -v -coverprofile=coverage.txt -covermode count ${TEST_DIR}
	sed -i '/\.pb\.go:/d' coverage.txt
	@GOFLAGS='${TEST_BUILD_FLAGS}' go run github.com/boumenot/gocover-cobertura < coverage.txt > coverage.xml
	@go tool cover -func=coverage.txt
	@go tool cover -html=coverage.txt -o coverage.html

test:
ifeq ($(TEST_DIR),$(TEST_PATHS))
	@$(MAKE) test-network-specific
endif
	@go test ${TEST_BUILD_FLAGS} ${RUN_FLAG} ${TEST_DIR}
	@TAG=stagenet $(MAKE) build

test-all: test-network-specific
	@go test ${TEST_BUILD_FLAGS} ${RUN_FLAG} "./..."

test-go-tss:
	@go test ${TEST_BUILD_FLAGS} ${RUN_FLAG} --race "./bifrost/tss/go-tss/..."

test-network-specific:
	@go test -tags stagenet ${RUN_FLAG} ./common
	@go test -tags chainnet ${RUN_FLAG} ./common
	@go test -tags mainnet ${RUN_FLAG} ./common ./bifrost/pkg/chainclients/utxo/...
	@go test -tags mocknet ${RUN_FLAG} ./common ./bifrost/pkg/chainclients/utxo/...

test-race:
	@go test -race ${TEST_BUILD_FLAGS} ${RUN_FLAG} ${TEST_DIR}

# ------------------------------ Performance Tests ------------------------------

# Run comprehensive benchmarks with profiling (recommended)
test-performance:
	@echo "Running comprehensive performance benchmarks with profiling..."
	@./test/performance/run_quote_benchmarks.sh

# Quick benchmark comparison (Queue Disabled vs Enabled)
test-performance-quick:
	@echo "Running quick performance comparison..."
	@cd x/thorchain && go test -run='^$$' -bench='^BenchmarkQuoteSwapComparison$$' -benchtime=10x -benchmem

# Run all benchmarks without profiling
test-performance-all:
	@echo "Running all benchmark tests..."
	@cd x/thorchain && go test -run='^$$' -bench=. -benchtime=5x -benchmem

# Analyze latest performance results
test-performance-analyze:
	@echo "Analyzing latest performance results..."
	@LATEST=$$(ls -td test/performance/results/*/ 2>/dev/null | head -1); \
	if [ -n "$$LATEST" ]; then \
		./test/performance/analyze_profiles.sh "$$LATEST"; \
	else \
		echo "No performance results found. Run 'make test-performance' first."; \
		exit 1; \
	fi

# Open interactive CPU profile viewer
test-performance-profile-cpu:
	@LATEST=$$(ls -td test/performance/results/*/ 2>/dev/null | head -1); \
	if [ -n "$$LATEST" ] && [ -f "$$LATEST/cpu.prof" ]; then \
		echo "Opening CPU profile at http://localhost:8080..."; \
		go tool pprof -http=:8080 "$$LATEST/cpu.prof"; \
	else \
		echo "No CPU profile found. Run 'make test-performance' first."; \
		exit 1; \
	fi

# Open interactive memory profile viewer
test-performance-profile-mem:
	@LATEST=$$(ls -td test/performance/results/*/ 2>/dev/null | head -1); \
	if [ -n "$$LATEST" ] && [ -f "$$LATEST/mem.prof" ]; then \
		echo "Opening memory profile at http://localhost:8080..."; \
		go tool pprof -http=:8080 "$$LATEST/mem.prof"; \
	else \
		echo "No memory profile found. Run 'make test-performance' first."; \
		exit 1; \
	fi

# View execution trace
test-performance-trace:
	@LATEST=$$(ls -td test/performance/results/*/ 2>/dev/null | head -1); \
	if [ -n "$$LATEST" ] && [ -f "$$LATEST/trace.out" ]; then \
		echo "Opening execution trace viewer..."; \
		go tool trace "$$LATEST/trace.out"; \
	else \
		echo "No trace found. Run 'make test-performance' first."; \
		exit 1; \
	fi

# Clean performance test results
test-performance-clean:
	@echo "Cleaning performance test results..."
	@rm -rf test/performance/results/*
	@echo "Performance results cleaned."

# ------------------------------ Regression Tests ------------------------------

test-regression: build-test-regression
	@docker run --rm ${DOCKER_TTY_ARGS} \
		-e DEBUG -e RUN -e EXPORT -e TIME_FACTOR -e PARALLELISM -e FAIL_FAST \
		-e AUTO_UPDATE -e IGNORE_FAILURES -e CI -e UNFILTER_EXPORT \
		-e UID=$(shell id -u) -e GID=$(shell id -g) \
		-p 1317:1317 -p 26657:26657 \
		-v $(shell pwd)/test/regression/mnt:/mnt \
		-v $(shell pwd)/test/regression/suites:/app/test/regression/suites \
		-v $(shell pwd)/test/regression/templates:/app/test/regression/templates \
		-w /app thornode-regtest sh -c 'make _test-regression'

build-test-regression:
	@DOCKER_BUILDKIT=1 docker build . \
		-t thornode-regtest \
		-f ci/Dockerfile.regtest \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DOCKER_MIRROR

test-regression-coverage:
	@go tool cover -html=test/regression/mnt/coverage/coverage.txt

# internal target used in docker build - version pinned for consistent app hashes
_build-test-regression:
	@go install -ldflags '$(ldflags)' -tags=mocknet,regtest ./cmd/thornode
	@go build -ldflags '$(ldflags) -X gitlab.com/thorchain/thornode/v3/constants.Version=9.999.0' \
		-cover -tags=mocknet,regtest -o /regtest/cover-thornode ./cmd/thornode
	@go build -ldflags '$(ldflags) -X gitlab.com/thorchain/thornode/v3/constants.Version=9.999.0' \
		-tags mocknet -o /regtest/regtest ./test/regression/cmd

# internal target used in test run
_test-regression:
	@rm -rf /mnt/coverage && mkdir -p /mnt/coverage
	@cd test/regression && /regtest/regtest
	@go tool covdata textfmt -i /mnt/coverage -o /mnt/coverage/coverage.txt
	@grep -v -E -e archive.go -e 'v[0-9]+.go' -e openapi/gen /mnt/coverage/coverage.txt > /mnt/coverage/coverage-filtered.txt
	@go tool cover -func /mnt/coverage/coverage-filtered.txt > /mnt/coverage/func-coverage.txt
	@awk '/^total:/ {print "Regression Coverage: " $$3}' /mnt/coverage/func-coverage.txt
	@chown -R ${UID}:${GID} /mnt

# ------------------------------ Simulation Tests ------------------------------

test-simulation: build-mocknet reset-mocknet test-simulation-no-reset

STAGES ?= all
test-simulation-cluster: build-test-simulation build-mocknet-cluster reset-mocknet-cluster
	@STAGES=$(STAGES) docker run --rm ${DOCKER_TTY_ARGS} \
		-e PARALLELISM -e STAGES --network host -w /app \
		thornode-simtest sh -c 'make _test-simulation'

test-simulation-no-reset: build-test-simulation
	@docker run --rm ${DOCKER_TTY_ARGS} \
		-e PARALLELISM -e STAGES --network host -w /app \
		thornode-simtest sh -c 'make _test-simulation'

build-test-simulation:
	@echo "simulation tests are not part of the Thornado THORNode fork"

test-simulation-events:
	@docker compose $(DOCKER_COMPOSE_FILES) run --rm events

# internal target used in docker build
_build-test-simulation:
	@echo "simulation tests are not part of the Thornado THORNode fork"

# internal target used in test run
_test-simulation:
	@echo "simulation tests are not part of the Thornado THORNode fork"

# ------------------------------ Single Node Mocknet ------------------------------

cli-mocknet:
	@docker compose $(DOCKER_COMPOSE_FILES) run --rm cli

run-mocknet:
	@docker compose $(DOCKER_COMPOSE_FILES) \
		--profile mocknet --profile midgard up -d

stop-mocknet:
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet --profile midgard down -v

# Halt the Mocknet without erasing the blockchain history, so it can be resumed later.
halt-mocknet:
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet --profile midgard down

build-mocknet:
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet build \
		--build-arg COMMIT=$(COMMIT)

bootstrap-mocknet:
	@docker run --rm ${DOCKER_TTY_ARGS} \
		-e PARALLELISM -e STAGES=seed,bootstrap --network host -w /app \
		thornode-simtest sh -c 'make _test-simulation'

ps-mocknet:
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet --profile midgard images
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet --profile midgard ps

logs-mocknet:
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet logs -f thornode bifrost

reset-mocknet: stop-mocknet run-mocknet

# ------------------------------ Mocknet EVM Fork ------------------------------

reset-mocknet-fork-%: stop-mocknet
	@./tools/evm/run-mocknet-fork.sh $*

# ------------------------------ Multi Node Mocknet ------------------------------

run-mocknet-cluster:
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet-cluster \
		--profile midgard up -d

stop-mocknet-cluster:
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet-cluster --profile midgard down -v

halt-mocknet-cluster:
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet-cluster --profile midgard down

build-mocknet-cluster:
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet-cluster build

ps-mocknet-cluster:
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet-cluster --profile midgard images
	@docker compose $(DOCKER_COMPOSE_FILES) --profile mocknet-cluster --profile midgard ps

reset-mocknet-cluster: stop-mocknet-cluster build-mocknet-cluster run-mocknet-cluster

# ------------------------------ Test Sync ------------------------------

test-sync-mainnet:
	@./scripts/test-sync.sh mainnet

test-sync-stagenet:
	@./scripts/test-sync.sh stagenet

# ------------------------------ Docker Build ------------------------------

docker-gitlab-login:
	docker login -u ${CI_REGISTRY_USER} -p ${CI_REGISTRY_PASSWORD} ${CI_REGISTRY}

docker-gitlab-push:
	./build/docker/semver_tags.sh registry.gitlab.com/thorchain/thornode ${BRANCH} $(shell cat version) \
		| xargs -n1 | grep registry | xargs -n1 docker push
	docker push registry.gitlab.com/thorchain/thornode:${GITREF}

docker-gitlab-build:
	docker build . \
		-f build/docker/Dockerfile \
		$(shell sh ./build/docker/semver_tags.sh registry.gitlab.com/thorchain/thornode ${BRANCH} $(shell cat version)) \
		-t registry.gitlab.com/thorchain/thornode:${GITREF} \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg TAG=$(BUILDTAG) \
		--build-arg DOCKER_MIRROR

########################################################################################
# Tools
########################################################################################

thorscan-build:
	@docker build tools/thorscan -f tools/thorscan/Dockerfile \
		$(shell sh ./build/docker/semver_tags.sh registry.gitlab.com/thorchain/thornode thorscan-${BRANCH} $(shell cat version))

thorscan-gitlab-push: docker-gitlab-login
	@./build/docker/semver_tags.sh registry.gitlab.com/thorchain/thornode thorscan-${BRANCH} $(shell cat version) \
		| xargs -n1 | grep registry | xargs -n1 docker push

events-build:
	@docker build . -f tools/events/Dockerfile \
		$(shell sh ./build/docker/semver_tags.sh registry.gitlab.com/thorchain/thornode events-${BRANCH} $(shell cat version)) \
		--build-arg TAG=$(BUILDTAG) \
		--build-arg DOCKER_MIRROR

events-gitlab-push: docker-gitlab-login
	@./build/docker/semver_tags.sh registry.gitlab.com/thorchain/thornode events-${BRANCH} $(shell cat version) \
		| xargs -n1 | grep registry | xargs -n1 docker push
