# Firecracker Agent - API Reference

Complete gRPC API documentation for the Firecracker Agent service.

## Service: FirecrackerAgent

Package: `firecracker.v1`

### Methods

---

## CreateVM

Creates and starts a new Firecracker microVM.

**Request: `CreateVMRequest`**

```protobuf
message CreateVMRequest {
  string vm_id = 1;          // Required: Unique VM identifier
  int32 vcpu_count = 2;      // Required: Number of vCPUs (1-32)
  int32 memory_mb = 3;       // Required: Memory size in MB (min 128)
  string ip_address = 4;     // Optional: IP address to assign
  string kernel_path = 5;    // Optional: Custom kernel path
  string rootfs_path = 6;    // Optional: Custom rootfs path
  map<string, string> metadata = 7;  // Optional: Custom metadata
}
```

**Response: `CreateVMResponse`**

```protobuf
message CreateVMResponse {
  string vm_id = 1;          // VM identifier
  VMState state = 2;         // Current state
  string socket_path = 3;    // Firecracker API socket path
  int64 created_at = 4;      // Creation timestamp (Unix)
  string error_message = 5;  // Error message if failed
}
```

**Example (grpcurl)**:

```bash
grpcurl -plaintext -d '{
  "vm_id": "vm-001",
  "vcpu_count": 2,
  "memory_mb": 512,
  "ip_address": "172.16.0.10"
}' localhost:50051 firecracker.v1.FirecrackerAgent/CreateVM
```

---

## StartVM

Starts an existing VM.

**Request: `StartVMRequest`**

```protobuf
message StartVMRequest {
  string vm_id = 1;  // Required: VM identifier
}
```

**Response: `StartVMResponse`**

```protobuf
message StartVMResponse {
  string vm_id = 1;          // VM identifier
  VMState state = 2;         // New state
  string error_message = 3;  // Error if failed
}
```

---

## StopVM

Stops a running VM.

**Request: `StopVMRequest`**

```protobuf
message StopVMRequest {
  string vm_id = 1;  // Required: VM identifier
  bool force = 2;    // Optional: Force stop (kill)
}
```

**Response: `StopVMResponse`**

```protobuf
message StopVMResponse {
  string vm_id = 1;
  VMState state = 2;
  string error_message = 3;
}
```

---

## DeleteVM

Deletes a VM and cleans up resources.

**Request: `DeleteVMRequest`**

```protobuf
message DeleteVMRequest {
  string vm_id = 1;  // Required: VM identifier
}
```

**Response: `DeleteVMResponse`**

```protobuf
message DeleteVMResponse {
  string vm_id = 1;
  bool success = 2;
  string error_message = 3;
}
```

---

## GetVM

Retrieves VM information.

**Request: `GetVMRequest`**

```protobuf
message GetVMRequest {
  string vm_id = 1;
}
```

**Response: `GetVMResponse`**

```protobuf
message GetVMResponse {
  VMInfo vm = 1;
}
```

---

## ListVMs

Lists all VMs.

**Request: `ListVMsRequest`**

```protobuf
message ListVMsRequest {
  int32 page_size = 1;
  string page_token = 2;
}
```

**Response: `ListVMsResponse`**

```protobuf
message ListVMsResponse {
  repeated VMInfo vms = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}
```

---

## WatchVMEvents

Streams VM events in real-time (server-side streaming).

**Request: `WatchVMEventsRequest`**

```protobuf
message WatchVMEventsRequest {
  string vm_id = 1;  // Optional: Filter by VM (empty = all VMs)
}
```

**Response: Stream of `VMEvent`**

```protobuf
message VMEvent {
  string vm_id = 1;
  VMState state = 2;
  string message = 3;
  int64 timestamp = 4;
  EventType type = 5;
}
```

**Example**:

```bash
grpcurl -plaintext localhost:50051 \
  firecracker.v1.FirecrackerAgent/WatchVMEvents
```

---

## GetHostInfo

Returns host system information.

**Request: `GetHostInfoRequest`**

```protobuf
message GetHostInfoRequest {}
```

**Response: `GetHostInfoResponse`**

```protobuf
message GetHostInfoResponse {
  string hostname = 1;
  int32 total_cpus = 2;
  int64 total_memory_mb = 3;
  int64 available_memory_mb = 4;
  int32 running_vms = 5;
  float cpu_usage = 6;
  string version = 7;
}
```

---

## HealthCheck

Returns agent health status.

**Request: `HealthCheckRequest`**

```protobuf
message HealthCheckRequest {}
```

**Response: `HealthCheckResponse`**

```protobuf
message HealthCheckResponse {
  bool healthy = 1;
  string version = 2;
  int64 uptime_seconds = 3;
}
```

---

## Common Types

### VMState Enum

```protobuf
enum VMState {
  VM_STATE_UNSPECIFIED = 0;
  VM_STATE_CREATING = 1;
  VM_STATE_RUNNING = 2;
  VM_STATE_STOPPING = 3;
  VM_STATE_STOPPED = 4;
  VM_STATE_DELETING = 5;
  VM_STATE_ERROR = 6;
}
```

### EventType Enum

```protobuf
enum EventType {
  EVENT_TYPE_UNSPECIFIED = 0;
  EVENT_TYPE_CREATED = 1;
  EVENT_TYPE_STARTED = 2;
  EVENT_TYPE_STOPPED = 3;
  EVENT_TYPE_DELETED = 4;
  EVENT_TYPE_ERROR = 5;
}
```

### VMInfo

```protobuf
message VMInfo {
  string vm_id = 1;
  VMState state = 2;
  int32 vcpu_count = 3;
  int32 memory_mb = 4;
  string ip_address = 5;
  string socket_path = 6;
  int64 created_at = 7;
  map<string, string> metadata = 8;
}
```

---

## Error Handling

The agent uses standard gRPC status codes:

- `OK (0)`: Success
- `INVALID_ARGUMENT (3)`: Invalid parameters
- `NOT_FOUND (5)`: VM not found
- `ALREADY_EXISTS (6)`: VM already exists
- `INTERNAL (13)`: Internal error

---

## Client Examples

### Go Client

```go
import (
    pb "github.com/apardo/firecracker-agent/api/proto/firecracker/v1"
    "google.golang.org/grpc"
)

conn, _ := grpc.Dial("localhost:50051", grpc.WithInsecure())
client := pb.NewFirecrackerAgentClient(conn)

resp, err := client.CreateVM(context.Background(), &pb.CreateVMRequest{
    VmId: "vm-001",
    VcpuCount: 2,
    MemoryMb: 512,
})
```

### Python Client

```python
import grpc
from api.proto.firecracker.v1 import firecracker_pb2, firecracker_pb2_grpc

channel = grpc.insecure_channel('localhost:50051')
client = firecracker_pb2_grpc.FirecrackerAgentStub(channel)

response = client.CreateVM(firecracker_pb2.CreateVMRequest(
    vm_id='vm-001',
    vcpu_count=2,
    memory_mb=512
))
```

---

## Rate Limiting

Currently no rate limiting is implemented. Future versions will support:
- Per-client limits
- Token bucket algorithm
- Configurable quotas
