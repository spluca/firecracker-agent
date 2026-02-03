# Deployment Guide - Firecracker Agent

## Prerequisites

### System Requirements

- **OS**: Linux (Ubuntu 20.04+, Debian 11+, RHEL 8+)
- **Kernel**: 4.14+ with KVM enabled
- **CPU**: x86_64 or ARM64 with virtualization support
- **RAM**: Minimum 2GB (more for production)
- **Storage**: 10GB+ for VM images

### Required Software

1. **Firecracker**

```bash
# Download Firecracker v1.6.0
wget https://github.com/firecracker-microvm/firecracker/releases/download/v1.6.0/firecracker-v1.6.0-x86_64.tgz
tar -xzf firecracker-v1.6.0-x86_64.tgz
sudo mv release-v1.6.0-x86_64/firecracker-v1.6.0-x86_64 /usr/bin/firecracker
sudo mv release-v1.6.0-x86_64/jailer-v1.6.0-x86_64 /usr/bin/jailer
sudo chmod +x /usr/bin/firecracker /usr/bin/jailer
```

2. **Verify KVM**

```bash
# Check if KVM is available
lsmod | grep kvm

# If not loaded, load the module
sudo modprobe kvm
sudo modprobe kvm_intel  # or kvm_amd for AMD CPUs
```

## Installation

### Option 1: From Release Binary

```bash
# Download latest release
wget https://github.com/apardo/firecracker-agent/releases/download/v0.1.0/fc-agent-linux-amd64.tar.gz
tar -xzf fc-agent-linux-amd64.tar.gz
cd fc-agent-linux-amd64

# Run installation script
sudo ./scripts/install.sh
```

### Option 2: Build from Source

```bash
# Clone repository
git clone https://github.com/apardo/firecracker-agent.git
cd firecracker-agent

# Install dependencies
make setup-protoc
make deps

# Build
make build

# Install
sudo make install
```

## Configuration

### 1. Edit Configuration File

```bash
sudo nano /etc/fc-agent/agent.yaml
```

**Production configuration example**:

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

### 2. Setup Network Bridge

```bash
# Install bridge-utils
sudo apt-get install bridge-utils

# Create bridge
sudo brctl addbr br0
sudo ip addr add 172.16.0.1/24 dev br0
sudo ip link set br0 up

# Enable IP forwarding
sudo sysctl -w net.ipv4.ip_forward=1
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf

# Configure NAT
sudo iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
sudo iptables -A FORWARD -i br0 -j ACCEPT
```

**Persistent configuration** (`/etc/network/interfaces`):

```
auto br0
iface br0 inet static
    address 172.16.0.1
    netmask 255.255.255.0
    bridge_ports none
    bridge_stp off
    bridge_fd 0
```

### 3. Prepare VM Images

```bash
# Create directories
sudo mkdir -p /var/lib/firecracker/kernels
sudo mkdir -p /var/lib/firecracker/rootfs
sudo mkdir -p /srv/firecracker/vms

# Download kernel
wget https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.6/x86_64/vmlinux-5.10.198 \
  -O /var/lib/firecracker/kernels/vmlinux

# Download rootfs (Ubuntu 20.04)
wget https://cloud-images.ubuntu.com/minimal/releases/focal/release/ubuntu-20.04-minimal-cloudimg-amd64-root.tar.xz
# Extract and convert to ext4...
# (See Firecracker docs for detailed steps)
```

## Starting the Agent

### Using Systemd (Recommended)

```bash
# Enable service
sudo systemctl enable fc-agent

# Start service
sudo systemctl start fc-agent

# Check status
sudo systemctl status fc-agent

# View logs
sudo journalctl -u fc-agent -f
```

### Manual Start

```bash
sudo /usr/local/bin/fc-agent --config /etc/fc-agent/agent.yaml
```

## Verification

### 1. Health Check

```bash
# Using grpcurl
grpcurl -plaintext localhost:50051 firecracker.v1.FirecrackerAgent/HealthCheck

# Expected response:
{
  "healthy": true,
  "version": "0.1.0",
  "uptimeSeconds": "123"
}
```

### 2. Create Test VM

```bash
grpcurl -plaintext -d '{
  "vm_id": "test-vm",
  "vcpu_count": 1,
  "memory_mb": 256,
  "ip_address": "172.16.0.10"
}' localhost:50051 firecracker.v1.FirecrackerAgent/CreateVM
```

### 3. Check Metrics

```bash
curl http://localhost:9090/metrics
```

## Production Deployment

### Security Hardening

1. **Enable mTLS** (when available)
2. **Firewall rules**:

```bash
# Allow only from mikrom-go API server
sudo ufw allow from <api-server-ip> to any port 50051
sudo ufw allow from <monitoring-ip> to any port 9090
```

3. **Run as non-root** (use jailer)

### High Availability

For multi-host deployment:

1. Deploy agent on each KVM host
2. Configure mikrom-go API with all agent addresses
3. Implement health checks and failover

### Monitoring

**Prometheus scrape config**:

```yaml
scrape_configs:
  - job_name: 'firecracker-agent'
    static_configs:
      - targets:
          - 'kvm-host1:9090'
          - 'kvm-host2:9090'
          - 'kvm-host3:9090'
```

**Grafana Dashboard**:
- Import `docs/grafana-dashboard.json`

### Backup & Recovery

1. **Backup configuration**:
```bash
sudo cp /etc/fc-agent/agent.yaml /backup/
```

2. **VM snapshots** (future feature)

## Troubleshooting

### Agent won't start

```bash
# Check logs
sudo journalctl -u fc-agent -n 50

# Common issues:
# - Port already in use
# - Missing kernel/rootfs files
# - Permissions on /srv/firecracker
```

### VM creation fails

```bash
# Check Firecracker binary
firecracker --version

# Check KVM availability
ls -l /dev/kvm

# Check bridge
brctl show

# View agent logs
sudo journalctl -u fc-agent -f
```

### Performance issues

```bash
# Check system resources
htop
free -h
df -h

# Check VM limits
cat /proc/sys/vm/max_map_count
```

## Updating

### Update Agent

```bash
# Stop service
sudo systemctl stop fc-agent

# Backup config
sudo cp /etc/fc-agent/agent.yaml /tmp/

# Install new version
sudo make install

# Restore config if needed
sudo cp /tmp/agent.yaml /etc/fc-agent/

# Start service
sudo systemctl start fc-agent
```

### Rolling Updates (Multi-host)

1. Update one host at a time
2. Verify health before proceeding
3. Migrate VMs if needed (future feature)

## Uninstall

```bash
# Stop service
sudo systemctl stop fc-agent
sudo systemctl disable fc-agent

# Remove files
sudo rm /usr/local/bin/fc-agent
sudo rm -rf /etc/fc-agent
sudo rm /etc/systemd/system/fc-agent.service

# Optional: Clean up VM data
# sudo rm -rf /srv/firecracker
```

## Support

- Documentation: https://github.com/apardo/firecracker-agent/docs
- Issues: https://github.com/apardo/firecracker-agent/issues
- Slack: #firecracker-agent
