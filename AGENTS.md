# Development Guidelines

This document contains critical information about working with this codebase.

## Core Development Rules

- NEVER mention a `co-authored-by` or similar aspects. Never mention the tool used to create the commit message or PR.

## Coding Best Practices

- **Testing**:
  - Prefer table-tests unless the change is extremely simple
  - Don't use suite-tests, these are legacy and should only be maintained. All new tests should be either plain Go tests or table-tests
  - Don't use https://github.com/stretchr/testify for any new mocks, these are legacy. Always use https://github.com/uber-go/mock when creating new tests
  - Try to round-trip all mappers where possible. Generate symmetric mappers when converting types
  - Tests should be proportional to the code they cover. Avoid standalone tests that duplicate table-test cases, and avoid exhaustive matrices when the code path is a simple error check. If tests are significantly larger than the code under test, look for redundancy.

- **Comments**:
  - Comments should be timeless — explain non-obvious invariants or constraints, never narrate what the current PR does or why a specific change was made. PR-level context belongs in the commit message or PR description, not in the code.

- **Types**:
  - Never use IDL code directly in service logic, map them to common/types

## System Architecture

### Core Components

- `service/sharddistributor` is the main service
- `common/types` contains the RPC layer internal type representation. This package should have few, if any dependencies and should be near the top of the dependency tree. It has values which represent IDL values and mappers in `common/types/mapper`
- `cmd/` contains the entry points: `server`, `smctl` (CLI), `sharddistributor-canary`, and `tools`
- `idls` is the submodule for building thrift specifically. Protobuf is imported via a go module.
- `proto` contains protobuf definitions

## Development Workflow Commands

- Tests are run via `make test` or `go test` for a specific package
- Linting can be performed by `make lint`
- Preparing all changes for a PR should be done via `make pr` which re-performs all IDL codegen, linting, formatting etc in order
