# Agent Development Guide

This document provides essential guidelines for AI coding agents working on the firecracker-agent codebase.

## Project Overview

Firecracker Agent is a high-performance gRPC service written in Go for managing Firecracker microVMs.
Module: `github.com/spluca/firecracker-agent`
Go version: 1.24.0

## Build & Test Commands

### Essential Commands

```bash
# Build binary (includes proto generation)
make build

# Run all tests with race detection and coverage
make test

# Run specific test
go test -v -race ./internal/agent/...
go test -v -race ./pkg/config/...

# Run single test function
go test -v -race -run TestFunctionName ./path/to/package

# Run integration tests
make test-integration

# Run linter (requires golangci-lint)
make lint

# Format code
make fmt

# Generate protobuf code
make proto

# Development mode (hot reload)
make dev

# Generate coverage report
make coverage

# Clean build artifacts
make clean
```

### Running the Agent

```bash
# With custom config
bin/fc-agent --config configs/agent.yaml

# Development mode
go run cmd/fc-agent/main.go --config configs/agent.yaml
```

## Project Structure

```
firecracker-agent/
├── cmd/fc-agent/          # Main entry point
├── api/proto/             # gRPC/protobuf definitions
├── internal/              # Private application code
│   ├── agent/            # gRPC server implementation
│   ├── firecracker/      # VM lifecycle management
│   ├── network/          # Network configuration
│   ├── storage/          # Storage management
│   └── monitor/          # Metrics and health checks
├── pkg/                   # Public library code
│   ├── config/           # Configuration management
│   └── logger/           # Logging utilities
├── configs/              # Configuration files
├── scripts/              # Deployment scripts
├── test/                 # Test files
└── docs/                 # Documentation
```

## Code Style Guidelines

### Import Ordering

Imports must be organized in three groups with blank lines between:

```go
package example

import (
    // Standard library
    "context"
    "fmt"
    "time"

    // External dependencies
    pb "github.com/spluca/firecracker-agent/api/proto/firecracker/v1"
    "github.com/sirupsen/logrus"
    "google.golang.org/grpc"

    // Internal packages
    "github.com/spluca/firecracker-agent/internal/firecracker"
    "github.com/spluca/firecracker-agent/pkg/config"
)
```

### Naming Conventions

- **Packages**: Short, lowercase, single-word names (e.g., `agent`, `config`, `logger`)
- **Files**: Lowercase with underscores (e.g., `server.go`, `handlers.go`)
- **Types**: PascalCase (e.g., `Server`, `Manager`, `VMProcess`)
- **Functions/Methods**: PascalCase for exported, camelCase for unexported
- **Variables**: camelCase (e.g., `vmID`, `cfgFile`, `grpcServer`)
- **Constants**: PascalCase or SCREAMING_SNAKE_CASE for package-level
- **gRPC aliases**: Use `pb` as the alias for protobuf imports

### Type Definitions

- Define types before functions
- Use struct embedding when appropriate
- Document exported types with GoDoc comments

```go
// Server implements the FirecrackerAgent gRPC service
type Server struct {
    pb.UnimplementedFirecrackerAgentServer

    cfg         *config.Config
    log         *logrus.Logger
    fcManager   *firecracker.Manager
    startTime   time.Time
    eventStream *EventStream
    mu          sync.RWMutex
}
```

### Error Handling

- Return errors using `fmt.Errorf` with `%w` verb for wrapping
- Use `google.golang.org/grpc/status` for gRPC errors
- Log errors with context using logrus fields
- Validate input parameters early in functions

```go
// Bad
if err != nil {
    return nil, errors.New("failed to load config: " + err.Error())
}

// Good
if err != nil {
    return nil, fmt.Errorf("failed to load config: %w", err)
}

// gRPC errors
if req.VmId == "" {
    return nil, status.Error(codes.InvalidArgument, "vm_id is required")
}
```

### Logging

- Use structured logging with logrus
- Include relevant fields for context
- Log levels: Debug, Info, Warn, Error

```go
s.log.WithFields(logrus.Fields{
    "vm_id":  req.VmId,
    "vcpus":  req.VcpuCount,
    "memory": req.MemoryMb,
}).Info("VM created successfully")

s.log.WithError(err).Error("Failed to create VM")
```

### Concurrency

- Use `sync.RWMutex` for shared state
- Acquire read locks for reads, write locks for writes
- Defer unlock calls immediately after lock

```go
func (m *Manager) GetVM(vmID string) (*pb.VMInfo, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    vm, exists := m.vms[vmID]
    if !exists {
        return nil, fmt.Errorf("VM %s not found", vmID)
    }

    return vm.Info, nil
}
```

### Function Structure

- Keep functions focused and short
- Validate inputs first
- Use early returns for error cases
- Document exported functions

```go
// CreateVM creates and starts a new VM
func (m *Manager) CreateVM(ctx context.Context, req *pb.CreateVMRequest) (*pb.VMInfo, error) {
    // Validation
    if req.VmId == "" {
        return nil, status.Error(codes.InvalidArgument, "vm_id is required")
    }

    // Implementation
    // ...

    return vmInfo, nil
}
```

## Testing Guidelines

- Place test files alongside source files with `_test.go` suffix
- Use table-driven tests for multiple scenarios
- Use `t.Run()` for subtests
- Integration tests go in `test/integration/` with build tag `// +build integration`

## Configuration

- Configuration files are in YAML format under `configs/`
- Use `configs/agent.yaml` as the default
- Local overrides: `configs/agent.local.yaml` (gitignored)

## Common Patterns

### Context Usage

Always pass and respect context for cancellation:

```go
func (m *Manager) CreateVM(ctx context.Context, req *pb.CreateVMRequest) (*pb.VMInfo, error) {
    // Check context
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Continue with operation
}
```

### gRPC Server Pattern

Embed `UnimplementedXXXServer` for forward compatibility:

```go
type Server struct {
    pb.UnimplementedFirecrackerAgentServer
    // fields...
}
```

## Pre-commit Checklist

Before committing code:

1. Run `make fmt` to format code
2. Run `make test` to ensure tests pass
3. Run `make lint` to check for issues
4. Ensure no debug prints or commented code
5. Update documentation if API changes
