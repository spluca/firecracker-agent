# Firecracker Agent

[![CI](https://github.com/spluca/firecracker-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/spluca/firecracker-agent/actions/workflows/ci.yml)

🚀 High-performance gRPC service for managing Firecracker microVMs with ultra-fast startup times (< 500ms).

## ✨ Features

- ⚡ **Ultra-fast VM creation**: < 500ms startup time
- 🔌 **gRPC API**: Efficient binary protocol for remote management
- 📊 **Prometheus metrics**: Built-in monitoring and observability
- 🛡️ **Secure isolation**: Firecracker jailer integration
- 🌐 **Network management**: TAP devices and bridge configuration
- 💾 **Storage management**: Copy-on-write with overlay filesystem
- 📡 **Event streaming**: Real-time VM state updates via gRPC streaming

## 📋 Prerequisites

- **Linux kernel 4.14+** with KVM enabled
- **Firecracker v1.x** installed (`/usr/bin/firecracker`)
- **Go 1.21+** for building from source
- **root or sudo access** for network configuration

## 🚀 Quick Start

### 1. Install Protocol Buffers Compiler

```bash
make setup-protoc
```

### 2. Build the Agent

```bash
make build
```

This will:

- Generate protobuf/gRPC code
- Compile the binary to `bin/fc-agent`

### 3. Configure

Edit `configs/agent.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 50051

firecracker:
  binary_path: "/usr/bin/firecracker"
  jailer_path: "/usr/bin/jailer"
  kernel_path: "/var/lib/firecracker/kernels/vmlinux"
  rootfs_path: "/var/lib/firecracker/rootfs/ubuntu20.ext4"

network:
  bridge_name: "br0"
  tap_prefix: "vmtap"

storage:
  vms_dir: "/srv/firecracker/vms"
  use_overlay: true

monitoring:
  enabled: true
  metrics_port: 9090

log:
  level: "info"
  format: "json"
```

### 4. Run

```bash
# Development mode
make dev

# Or build and run
make run

# Or install as systemd service
sudo make install
sudo systemctl start fc-agent
```

## 📡 API Usage

### Health Check

```bash
grpcurl -plaintext localhost:50051 firecracker.v1.FirecrackerAgent/HealthCheck
```

### Create VM

```bash
grpcurl -plaintext -d '{
  "vm_id": "test-vm-001",
  "vcpu_count": 2,
  "memory_mb": 512,
  "ip_address": "172.16.0.10"
}' localhost:50051 firecracker.v1.FirecrackerAgent/CreateVM
```

### List VMs

```bash
grpcurl -plaintext localhost:50051 firecracker.v1.FirecrackerAgent/ListVMs
```

### Stop VM

```bash
grpcurl -plaintext -d '{
  "vm_id": "test-vm-001"
}' localhost:50051 firecracker.v1.FirecrackerAgent/StopVM
```

### Delete VM

```bash
grpcurl -plaintext -d '{
  "vm_id": "test-vm-001"
}' localhost:50051 firecracker.v1.FirecrackerAgent/DeleteVM
```

### Watch Events (streaming)

```bash
grpcurl -plaintext localhost:50051 firecracker.v1.FirecrackerAgent/WatchVMEvents
```

## 📊 Monitoring

Prometheus metrics are exposed at `http://localhost:9090/metrics`

Available metrics:

- `firecracker_vms_created_total` - Total VMs created
- `firecracker_vms_running` - Currently running VMs
- `firecracker_vm_operation_duration_seconds` - Operation durations
- `firecracker_grpc_requests_total` - gRPC request counts

Example Grafana dashboard configuration included in `docs/`.

## 🏗️ Project Structure

```
firecracker-agent/
├── cmd/fc-agent/          # Entry point
├── api/proto/             # gRPC/protobuf definitions
├── internal/
│   ├── agent/            # gRPC server implementation
│   ├── firecracker/      # VM lifecycle management
│   ├── network/          # Network configuration
│   ├── storage/          # Storage management
│   └── monitor/          # Metrics and health checks
├── pkg/
│   ├── config/           # Configuration
│   └── logger/           # Logging
├── configs/              # Default configurations
├── scripts/              # Installation scripts
└── docs/                 # Documentation
```

## 🛠️ Development

### Build & Test

```bash
# Format code
make fmt

# Run tests
make test

# Run linter
make lint

# Generate coverage report
make coverage
```

### Generate Protobuf Code

After modifying `.proto` files:

```bash
make proto
```

## 🔧 Makefile Commands

Run `make help` to see all available commands:

```bash
make help
```

Available commands:

- `build` - Build the binary
- `test` - Run tests
- `run` - Run the agent
- `dev` - Run with hot reload
- `proto` - Generate protobuf code
- `install` - Install as systemd service
- `clean` - Clean build artifacts
- `setup-protoc` - Install protoc compiler

## 🐳 Docker

Build and run with Docker:

```bash
docker build -t firecracker-agent .
docker run -p 50051:50051 -p 9090:9090 firecracker-agent
```

## 📚 Documentation

- [Architecture](docs/architecture.md)
- [API Reference](docs/api-reference.md)
- [Deployment Guide](docs/deployment.md)

## 🤝 Integration with mikrom-go

This agent is designed to work with [mikrom-go](https://github.com/spluca/mikrom) API.

See [Integration Guide](docs/integration.md) for details.

## ❓ Troubleshooting

236:
237: ### Systemd Service Issues
238:
239: When running as a systemd service, the Firecracker jailer requires specific permissions to create the chroot environment and device nodes.
240:
241: If you encounter `Permission denied` or `Operation not permitted` errors:
242:
243: 1. **Capabilities**: Ensure the service has `CAP_DAC_OVERRIDE`, `CAP_DAC_READ_SEARCH` (for file access) and `CAP_KILL` (for process monitoring).
244: 2. **Device Nodes**: The jailer uses `mknod` to create devices inside the jail. Ensure `DeviceAllow` includes `rwm` (read, write, mknod) permissions for:
245:    - `/dev/kvm` (and `char-10:232`)
246:    - `/dev/net/tun` (and `char-10:200`)
247:    - `/dev/userfaultfd` (and `char-10:257`)
248:
249: Sample `fc-agent.service` configuration:
250:
251: ```ini
252: [Service]
253: CapabilityBoundingSet=... CAP_DAC_OVERRIDE CAP_DAC_READ_SEARCH CAP_KILL
254: AmbientCapabilities=... CAP_DAC_OVERRIDE CAP_DAC_READ_SEARCH CAP_KILL
255:
256: DeviceAllow=/dev/kvm rwm
257: DeviceAllow=char-10:232 rwm
258: DeviceAllow=/dev/net/tun rwm
259: DeviceAllow=char-10:200 rwm
260: DeviceAllow=/dev/userfaultfd rwm
261: DeviceAllow=char-10:257 rwm
262:```
263:
264: ## 🔒 Security

- VMs run with Firecracker jailer for additional isolation
- Network segregation via TAP devices and bridges
- Future: mTLS support for gRPC connections

## 📝 License

MIT License - See [LICENSE](LICENSE) file

## 👥 Authors

- Antonio Pardo - [@apardo](https://github.com/antpard)

## 🙏 Acknowledgments

- [Firecracker](https://firecracker-microvm.github.io/) - Secure and fast microVMs
- [gRPC](https://grpc.io/) - High-performance RPC framework
- [Prometheus](https://prometheus.io/) - Monitoring and alerting
