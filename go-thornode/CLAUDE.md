# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Building and Installation

### Prerequisites

- Go (as specified in go.mod)
- Make
- Docker and Docker Compose V2
- Protobuf compiler

### Installation

```bash
# Clone the repository
git clone https://gitlab.com/thorchain/thornode.git
cd thornode

# Install thornode
make go-generate openapi proto-gen install

# Verify installation
thornode help
```

### Common Development Commands

```bash
# Build the project
make build

# Install binaries
make install

# Format code
make format

# Run linter
make lint

# Generate protocol buffers, openapi, etc.
make generate
```

## Testing

```bash
# Run standard tests (excluding computationally-expensive tests like go-tss)
make test

# Run all tests (including computationally-expensive ones)
make test-all

# Run only go-tss tests
make test-go-tss

# Run tests with race detector
make test-race

# Run regression tests
make test-regression

# Run simulation tests (interactive prompt for stage selection)
make test-simulation

# Run simulation tests without prompts (all stages)
STAGES=all make test-simulation

# Run simulation tests with specific stages
STAGES=seed,bootstrap,swaps make test-simulation

# Create test coverage report
make test-coverage
make coverage-report   # HTML report
```

### Testing Guidelines

- To run unit tests, use `make test`. To run a specific unit test, use `go test -tags=mocknet -parallel=1 -run TestName ./...`
- To run regression tests, use `make test-regression`. To test a specific suite or regression test, utilize RUN as an environment variable
- To test a specific suite or regression test, utilize RUN as an environment variable
- For simulation tests:
  - By default, `make test-simulation` prompts for stage selection if running in a terminal
  - Use `STAGES=all` to run all stages without prompts
  - Use `STAGES=stage1,stage2` to run specific stages
  - Available stages: seed, bootstrap, arb, swaps, memoless-swaps, consolidate, churn, inactive-vault-refunds, solvency, ragnarok
- Do not edit generated files such as `pulsar.go` and `pb.go`, and no files in ./openai/gen or ./test/regression/mnt
- When building, use `make build`
- After editing any Go file, use `goimports -w` to ensure proper formatting
- After editing any protobuf files (ie ".proto") or any yaml files in ./openapi, run `make generate`
- For Markdown files, use `trunk` to format
- For JSON files, format using `jq` or `trunk`

## Local Development Environment

```bash
# Start a local mocknet for development
make run-mocknet

# Stop mocknet and remove volumes
make stop-mocknet

# Reset mocknet (stop and restart)
make reset-mocknet

# Run CLI in mocknet container
make cli-mocknet

# View logs
make logs-mocknet

# Bootstrap mocknet with liquidity
make bootstrap-mocknet
```

## Architecture Overview

THORChain is a decentralized liquidity network built with Cosmos SDK and TSS-lib. The architecture consists of several key components:

### Core Components

1. **THORNode**: The main blockchain node

   - Implements the THORChain state machine logic
   - Handles transactions, consensus, and the core protocol
   - Built on Cosmos SDK and Tendermint (CometBFT)

2. **Bifrost**: Bridge component connecting external chains to THORChain

   - Consists of observers and signers
   - Monitors external chains and processes transactions
   - Communicates with THORNode via gRPC

3. **TSS (Threshold Signature Scheme)**: Cryptographic system for secure multi-party signing
   - Implements threshold cryptography for key management
   - Allows nodes to collectively sign transactions without revealing private keys

### Data Flow

1. **Inbound**: External chains → Bifrost → THORNode

   - Bifrost observes transactions on external chains
   - Communicates with THORNode via gRPC

2. **Outbound**: THORNode → Bifrost → External chains
   - THORNode processes swaps, adds liquidity, etc.
   - Creates outbound transactions
   - Bifrost signs and broadcasts to external chains

### Key Modules

- **x/thorchain**: Main module implementing THORChain-specific logic

  - Handlers for various transaction types (swap, add/withdraw liquidity, etc.)
  - Managers for different aspects (pools, network, etc.)
  - Keepers for state management

- **bifrost/chainclients**: Clients for different blockchains

  - Support for UTXO chains (BTC, LTC, BCH, DOGE)
  - Support for EVM chains (ETH, BSC, AVAX)
  - Support for Cosmos-based chains

- **bifrost/observer**: Monitors external chains for inbound transactions
- **bifrost/signer**: Signs outbound transactions for external chains

### Network Types

THORChain supports multiple network types, each with specific configurations:

- **Mainnet**: Production network
- **Stagenet**: Testing environment
- **Mocknet**: Local development environment

### Testing Infrastructure

1. **Unit Tests**: Standard Go tests throughout the codebase
2. **Regression Tests**: YAML-defined test suites testing specific functionality
3. **Simulation Tests**: End-to-end tests simulating real-world scenarios
4. **Mocknet**: Local development environment for testing

## Development Guidelines

1. When adding a new feature, ensure you implement:

   - Handler logic in `/x/thorchain/handler_*.go`
   - Unit tests for all new code
   - Updates to regression and simulation tests if needed

2. Follow the ADR (Architecture Decision Record) process for significant changes:

   - Document design decisions in `/docs/architecture/`
   - Provide rationale and implementation details

3. For cross-chain functionality:

   - Understand the bifrost observer/signer workflow
   - Test with mocknet to ensure proper integration

4. When making chain-specific changes:

   - Test on the appropriate network type (mainnet, stagenet, mocknet)
   - Consider chain-specific configurations in files like `*_mainnet.go`, `*_stagenet.go`, etc.

5. For protocol upgrades:
   - The network generally soft-forks monthly and hard-forks annually
   - Ensure backward compatibility where appropriate
   - Test thoroughly with regression and simulation tests

## Memories

- When you want to generate go files or the api, please use "make generate"
- this code repository is a gitlab repository, so remember to use the GitLab CLI ('glab') for all GitLab-related tasks

- when modifying gitlab ci file (.gitlab-ci.yaml), validate it with the glab cli (`glab ci lint`)
- when updating a proto file, use make proto-format to ensure the formatting of that file is correct
- **Cache Context**: Do not add redundant cache contexts in handler functions. The handler's `Run` method is already wrapped in a `CacheContext` at `x/thorchain/handler.go:40-41`, which ensures atomicity for all state changes during handler execution. Adding another cache context inside handler methods (like `modifyLimitSwap`) is unnecessary and redundant.
- **Go Refactoring**: When renaming types, functions, or variables in Go code, always use `gorename` instead of text-based find/replace. Example: `gorename -from '"gitlab.com/thorchain/thornode/v3/x/thorchain".TypeName' -to NewTypeName`. This tool understands Go's type system and scope, avoiding false matches in comments/strings and handling shadowed variables correctly.
- **MR Title/Description Consistency**: After making any code changes to an MR, always update the MR title and description to accurately reflect the current state of changes. Use `glab mr update <MR_NUMBER> --title "..." --description "..."` to keep the title and description synchronized with the actual code.
