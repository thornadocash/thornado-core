########################################################################################
# Config
########################################################################################

CHECK_ENV := $(shell ./scripts/check-env.sh)
ifneq ($(CHECK_ENV),)
$(error Check environment dependencies.)
endif

.PHONY: build install test test-all test-go-tss test-network-specific format lint lint-ci generate go-generate proto-all proto-gen proto-format proto-format-check proto-lint openapi tools shielder-ffi

COMMIT ?= $(shell git log -1 --format='%H' 2>/dev/null)
VERSION := $(shell cat version)
TAG ?= mocknet
BINARIES ?= ./cmd/thornado ./cmd/bifrost
ROOT_DIR := $(abspath ..)
PARALLELISM ?= 1
TEST_PATHS := $(shell go list ./... | grep -v bifrost/tss/go-tss)
TEST_DIR ?= ${TEST_PATHS}
BUILD_FLAGS := -ldflags '$(ldflags)' -tags ${TAG} -trimpath
TEST_BUILD_FLAGS := -parallel=$(PARALLELISM) -tags=mocknet
ifdef RUN
RUN_FLAG := -run=$(RUN)
endif

ldflags = -X github.com/thornadocash/go-thornado/constants.Version=$(VERSION) \
	-X github.com/thornadocash/go-thornado/constants.GitCommit=$(COMMIT) \
	-X github.com/cosmos/cosmos-sdk/version.Name=Thornado \
	-X github.com/cosmos/cosmos-sdk/version.AppName=thornado \
	-X github.com/cosmos/cosmos-sdk/version.Version=$(VERSION) \
	-X github.com/cosmos/cosmos-sdk/version.Commit=$(COMMIT) \
	-X github.com/cosmos/cosmos-sdk/version.BuildTags=$(TAG) \
	-buildid=

########################################################################################
# Build
########################################################################################

build:
	go build ${BUILD_FLAGS} ${BINARIES}

install:
	go install ${BUILD_FLAGS} ${BINARIES}

tools:
	@echo "operator tools are not part of Thornado"

########################################################################################
# Generate
########################################################################################

generate: go-generate openapi proto-gen
	@./scripts/generate.py

go-generate:
	@go install golang.org/x/tools/cmd/stringer@v0.28.0
	@go generate ./...

openapi:
	@docker run --rm \
		--user $(shell id -u):$(shell id -g) \
		-v $$PWD/openapi:/mnt \
		openapitools/openapi-generator-cli:v6.0.0@sha256:310bd0353c11863c0e51e5cb46035c9e0778d4b9c6fe6a7fc8307b3b41997a35 \
		generate -i /mnt/openapi.yaml -g go -o /mnt/gen
	@rm -f openapi/gen/go.mod openapi/gen/go.sum
	@find ./openapi/gen -type f | xargs sed -i '/^[- ]*API version.*$(shell cat version)/d;/APIClient.*$(shell cat version)/d'
	@find ./openapi/gen -type f | grep model | xargs sed -i 's/MarshalJSON(/MarshalJSON_deprecated(/'

protoVer = 0.13.2
protoImageName = ghcr.io/cosmos/proto-builder:$(protoVer)
protoImage = docker run --rm -v $(CURDIR):/workspace --workdir /workspace $(protoImageName)

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

########################################################################################
# Tests
########################################################################################

test:
	@go test ${TEST_BUILD_FLAGS} ${RUN_FLAG} ${TEST_DIR}
	@TAG=stagenet $(MAKE) build

test-all:
	@go test ${TEST_BUILD_FLAGS} ${RUN_FLAG} ./...

test-go-tss:
	@go test ${TEST_BUILD_FLAGS} ${RUN_FLAG} --race ./bifrost/tss/go-tss/...

test-network-specific:
	@go test -tags stagenet ${RUN_FLAG} ./common
	@go test -tags chainnet ${RUN_FLAG} ./common
	@go test -tags mainnet ${RUN_FLAG} ./common ./bifrost/pkg/chainclients/utxo/...
	@go test -tags mocknet ${RUN_FLAG} ./common ./bifrost/pkg/chainclients/utxo/...

########################################################################################
# Housekeeping
########################################################################################

format:
	@git ls-files '*.go' | grep -v -e '^api/' -e '^openapi/gen/' -e '.pb.go$$' -e '.pb.gw.go$$' -e '_gen.go$$' -e 'wire_gen.go$$' | xargs gofumpt -w

lint:
	@./scripts/lint.sh
	@./scripts/trunk check --no-fix --show-existing --upstream origin/develop

lint-ci:
	@./scripts/lint.sh
	@./scripts/trunk-ci.sh

shielder-ffi:
	@cd $(ROOT_DIR) && cargo build -p thornado-ffi --release
