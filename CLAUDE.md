# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Firecracker Agent is a gRPC service (Go) for managing Firecracker microVMs on Linux hosts. It is the "Node" component in the Mikrom platform, controlled by the Mikrom Core API but also usable standalone.

**Module:** `github.com/spluca/firecracker-agent` | **Go:** 1.25.6 | **Phase:** 1 (MVP) with Phase 2 networking/storage in progress.

## Build & Dev Commands

```bash
make build              # Generate protobuf + build binary to bin/fc-agent
make dev                # Run with go run (hot reload, debug logs)
sudo make run           # Build + run (needs root for KVM/networking)
make test               # Unit tests with race detection + coverage
make test-integration   # Integration tests (requires KVM + Firecracker)
make proto              # Regenerate protobuf code only
make lint               # golangci-lint
make fmt                # go fmt
make coverage           # Tests + HTML coverage report
make deps               # go mod download + tidy
make clean              # Remove bin/, coverage files, generated .pb.go
make setup-protoc       # Install protoc compiler + Go plugins (first-time setup)
make install            # Build + install via scripts/install.sh

# Run specific package tests
go test -v -race ./internal/firecracker/...

# Run a single test function
go test -v -race -run TestFunctionName ./internal/agent/...
```

## Architecture

```
gRPC Client (mikrom core or grpcurl)
    │
    ▼
Agent Server (internal/agent/)
    ├── server.go     — gRPC server setup, EventStream pub/sub, logging interceptor
    └── handlers.go   — RPC handlers: Create/Start/Stop/Delete/Get/ListVMs, WatchVMEvents, HealthCheck
         │
         ▼
Firecracker Manager (internal/firecracker/)
    ├── manager.go    — VM lifecycle orchestration, in-memory registry (sync.RWMutex)
    ├── client.go     — HTTP-over-Unix-socket client to Firecracker API
    ├── process.go    — Firecracker process start/stop, process group management
    └── jailer.go     — Chroot jail setup, UID/GID isolation, binary staging
         │
    ┌────┴────┐
    ▼         ▼
Network       Storage
(internal/    (internal/
 network/)     storage/)
TAP/bridge    VM dirs,
management    rootfs prep,
              overlay COW
```

**Supporting packages:**
- `pkg/config/` — YAML config loader (`configs/agent.yaml`)
- `pkg/logger/` — Structured logging (logrus wrapper)
- `internal/monitor/` — Prometheus metrics on `:9090` (`/metrics`, `/health`)
- `cmd/fc-agent/main.go` — Entry point (Cobra CLI)

### Key design points

- **Firecracker communication:** HTTP over Unix socket, not TCP. The `client.go` sends REST calls to Firecracker's API socket.
- **VM state is in-memory:** No persistence layer. The `Manager` tracks VMs in a map protected by `sync.RWMutex`.
- **Jailer enabled by default:** VMs run in a chroot with dropped privileges (configurable UID/GID).
- **Event streaming:** `WatchVMEvents` is a server-side gRPC stream. `EventStream` broadcasts to subscribers via channels.
- **Root required:** Network operations (TAP/bridge) and KVM access need root or `CAP_NET_ADMIN`.

## gRPC API

Defined in `api/proto/firecracker/v1/firecracker.proto`. Service: `FirecrackerAgent`.

**RPCs:** `CreateVM`, `StartVM`, `StopVM`, `DeleteVM`, `GetVM`, `ListVMs`, `WatchVMEvents` (server-stream), `GetHostInfo`, `HealthCheck`

**VM states:** `CREATING → RUNNING → STOPPING → STOPPED → DELETING` (+ `ERROR`)

After modifying the `.proto` file, run `make proto` to regenerate Go code. Generated files (`*.pb.go`) are committed.

## Configuration

`configs/agent.yaml` — copy from `configs/agent.example.yaml`:

```yaml
server:     { host, port: 50051 }
firecracker: { binary_path, jailer_path, kernel_path, rootfs_path, use_jailer: true, jail_uid, jail_gid }
network:    { bridge_name: "fcbr0", tap_prefix: "fctap" }
storage:    { vms_dir, use_overlay: false }
monitoring: { enabled: true, metrics_port: 9090 }
log:        { level: "info", format: "json" }
```

## Code Conventions

- **Imports:** Three groups separated by blank lines: stdlib, external deps, internal packages. Use `pb` alias for protobuf imports.
- **Error handling:** Wrap with `fmt.Errorf("...: %w", err)`. Use `google.golang.org/grpc/status` for gRPC errors.
- **Logging:** Structured logrus with `WithFields`/`WithError`. Levels: Debug, Info, Warn, Error.
- **Concurrency:** `sync.RWMutex` for shared state; defer unlock immediately after lock.
- **Tests:** Files alongside source (`_test.go`). Table-driven with `t.Run()`. Integration tests use build tag `integration` and go in `test/integration/`.

## CI/CD

GitLab CI (`.gitlab-ci.yml`): lint (gofmt, go vet, staticcheck) → test (unit + integration) → build (protobuf, then binary with `-ldflags "-s -w"`) → package (Debian `.deb` for Trixie).

Build is static: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`.
