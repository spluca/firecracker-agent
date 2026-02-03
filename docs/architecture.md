# Firecracker Agent - Architecture

## Overview

The Firecracker Agent is a gRPC service that manages Firecracker microVMs with ultra-fast startup times (< 500ms). It provides high-performance VM lifecycle management through a native gRPC interface.

## Components

### 1. gRPC Server (`internal/agent/`)
- **server.go**: Main gRPC service implementation
- **handlers.go**: RPC method handlers (CreateVM, StartVM, StopVM, etc.)
- **middleware.go**: Interceptors for logging and metrics

### 2. Firecracker Manager (`internal/firecracker/`)
- **manager.go**: VM lifecycle management
- **client.go**: Firecracker API client (Unix socket)
- **config.go**: VM configuration generation
- **jailer.go**: Jailer integration for security

### 3. Network Management (`internal/network/`)
- **tap.go**: TAP device creation and management
- **bridge.go**: Linux bridge configuration
- **iptables.go**: Firewall rules and NAT

### 4. Storage Management (`internal/storage/`)
- **manager.go**: Storage operations
- **overlay.go**: Copy-on-write filesystem

### 5. Monitoring (`internal/monitor/`)
- **metrics.go**: Prometheus metrics
- **health.go**: Health checks

## Data Flow

```
Client → gRPC Request
    ↓
Logging Interceptor
    ↓
Handler (CreateVM/Start/Stop/etc.)
    ↓
Firecracker Manager
    ↓
┌─────────┬─────────┬─────────┐
│ Network │ Storage │ FC API  │
└─────────┴─────────┴─────────┘
    ↓
Firecracker Process
    ↓
microVM Running
```

## VM Creation Flow

1. **Validate Request**: Check parameters (vcpu, memory, etc.)
2. **Allocate Resources**:
   - Create VM directory
   - Generate unique VM ID
   - Assign IP address
3. **Configure Network**:
   - Create TAP device
   - Attach to bridge
   - Configure iptables
4. **Prepare Storage**:
   - Copy/link kernel
   - Setup rootfs (overlay if enabled)
5. **Generate Config**: Create Firecracker JSON config
6. **Start VM**:
   - Launch with jailer (if enabled)
   - Connect to API socket
   - Send boot command
7. **Monitor**: Track state and emit events

## Event Streaming

The agent supports real-time event streaming using gRPC server-side streaming:

```
Client subscribes → WatchVMEvents
    ↓
Server broadcasts events:
- VM_CREATED
- VM_STARTED
- VM_STOPPED
- VM_DELETED
- VM_ERROR
```

## Security

### Firecracker Jailer
- Runs VMs in chroot
- Drops privileges
- Sets resource limits

### Network Isolation
- Separate TAP device per VM
- Bridge with iptables filtering
- Optional VLANs

### Future: mTLS
- Mutual authentication
- Certificate-based authorization

## Performance Optimizations

1. **No SSH Overhead**: Direct communication with Firecracker
2. **Binary Protocol**: gRPC uses protobuf (faster than JSON)
3. **Connection Pooling**: Reuse Unix socket connections
4. **Async Operations**: Non-blocking VM operations
5. **Pre-warming**: Optional resource pre-allocation

## Monitoring & Observability

### Prometheus Metrics
- `firecracker_vms_created_total`: Counter
- `firecracker_vms_running`: Gauge
- `firecracker_vm_operation_duration_seconds`: Histogram
- `firecracker_grpc_requests_total`: Counter

### Structured Logging
- JSON format for parsing
- Configurable log levels
- Request tracing

## Configuration

All configuration is loaded from YAML:

```yaml
server:
  host: "0.0.0.0"
  port: 50051

firecracker:
  binary_path: "/usr/bin/firecracker"
  # ...

network:
  bridge_name: "br0"
  # ...
```

## Future Enhancements

1. **TLS/mTLS Support**: Secure communication
2. **Resource Quotas**: Per-user/tenant limits
3. **Snapshot Support**: Save/restore VMs
4. **Hot-plugging**: Dynamic resource changes
5. **Multi-host**: Cluster coordination
6. **Advanced Networking**: SR-IOV, DPDK
