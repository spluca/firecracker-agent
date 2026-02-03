# Debian Package Installation Guide

This guide explains how to build and install the firecracker-agent Debian package on your KVM server.

## Prerequisites

### System Requirements

- **Operating System**: Debian 11+ or Ubuntu 20.04+
- **Architecture**: amd64 (x86_64)
- **Kernel**: Linux 5.10+ (for Firecracker support)
- **Privileges**: Root or sudo access required

### Required Packages

Install build dependencies:

```bash
sudo apt-get update
sudo apt-get install -y \
    build-essential \
    debhelper \
    golang-go \
    protobuf-compiler \
    git
```

### Runtime Dependencies

These will be installed automatically with the package:

- `firecracker` (>= 1.0.0) - Firecracker microVM binary
- `qemu-utils` - For qcow2 image manipulation
- `iproute2` - For network configuration
- `iptables` - For NAT configuration
- `bridge-utils` - For bridge network setup

## Building the Package

### 1. Clone the Repository

```bash
cd /usr/src
git clone https://github.com/apardo/firecracker-agent.git
cd firecracker-agent
```

### 2. Build the Debian Package

```bash
./build-deb.sh
```

This script will:
- Check for required tools
- Generate protobuf files
- Download Go dependencies
- Build the binary
- Create the .deb package

The package will be created in the parent directory: `../firecracker-agent_0.1.0-1_amd64.deb`

### 3. Alternative: Manual Build

If you prefer to build manually:

```bash
# Generate protobuf files
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    api/proto/firecracker/v1/firecracker.proto

# Download dependencies
go mod download

# Build package
dpkg-buildpackage -us -uc -b
```

## Installing the Package

### 1. Install Firecracker First

If Firecracker is not already installed:

```bash
# Download Firecracker
FIRECRACKER_VERSION="v1.7.0"
wget https://github.com/firecracker-microvm/firecracker/releases/download/${FIRECRACKER_VERSION}/firecracker-${FIRECRACKER_VERSION}-x86_64.tgz

# Extract and install
tar -xzf firecracker-${FIRECRACKER_VERSION}-x86_64.tgz
sudo mv release-${FIRECRACKER_VERSION}-x86_64/firecracker-${FIRECRACKER_VERSION}-x86_64 /usr/local/bin/firecracker
sudo chmod +x /usr/local/bin/firecracker
```

### 2. Install the Package

```bash
cd /usr/src/firecracker-agent
sudo dpkg -i ../firecracker-agent_0.1.0-1_amd64.deb

# Fix dependencies if needed
sudo apt-get install -f
```

### 3. Verify Installation

```bash
# Check binary
which fc-agent
fc-agent --version

# Check systemd service
systemctl status fc-agent

# Check configuration
ls -la /etc/firecracker-agent/
```

## Configuration

### 1. Edit Configuration

The default configuration is located at `/etc/firecracker-agent/agent.yaml`:

```bash
sudo nano /etc/firecracker-agent/agent.yaml
```

Important settings:

```yaml
server:
  grpc_address: "0.0.0.0:50051"  # gRPC listen address
  log_level: "info"

firecracker:
  binary_path: "/usr/local/bin/firecracker"  # Path to Firecracker binary
  kernel_path: "/var/lib/firecracker/images/vmlinux.bin"  # Kernel image
  rootfs_path: "/var/lib/firecracker/images/rootfs.ext4"  # Root filesystem
  
storage:
  vms_dir: "/var/lib/firecracker/vms"  # VM storage directory
  use_overlay: true  # Enable copy-on-write overlay

network:
  bridge_name: "fc-br0"  # Bridge name
  tap_prefix: "fc-tap"   # TAP device prefix
```

### 2. Prepare Images

Place your kernel and rootfs images:

```bash
# Create images directory
sudo mkdir -p /var/lib/firecracker/images

# Copy kernel (example)
sudo cp /path/to/vmlinux.bin /var/lib/firecracker/images/

# Copy rootfs (example)
sudo cp /path/to/rootfs.ext4 /var/lib/firecracker/images/

# Set permissions
sudo chown -R firecracker:firecracker /var/lib/firecracker/images
```

### 3. Configure Network Bridge

The package will enable IP forwarding automatically. To create the bridge manually:

```bash
# Create bridge
sudo ip link add name fc-br0 type bridge
sudo ip addr add 172.16.0.1/24 dev fc-br0
sudo ip link set fc-br0 up

# Enable NAT (for internet access from VMs)
sudo iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
sudo iptables -A FORWARD -i fc-br0 -j ACCEPT
sudo iptables -A FORWARD -o fc-br0 -j ACCEPT
```

To make the bridge persistent, create `/etc/network/interfaces.d/fc-br0`:

```bash
auto fc-br0
iface fc-br0 inet static
    address 172.16.0.1
    netmask 255.255.255.0
    bridge_ports none
    bridge_stp off
    bridge_fd 0
```

## Running the Service

### 1. Enable and Start Service

```bash
# Enable service to start at boot
sudo systemctl enable fc-agent

# Start service now
sudo systemctl start fc-agent

# Check status
sudo systemctl status fc-agent
```

### 2. Check Logs

```bash
# View logs
sudo journalctl -u fc-agent -f

# Or check log file
sudo tail -f /var/log/firecracker-agent/fc-agent.log
```

### 3. Test the Service

```bash
# Test gRPC connection (requires grpcurl)
grpcurl -plaintext localhost:50051 list

# Or use the mikrom-go client
# (from the mikrom-go project)
```

## Service Management

### Start/Stop/Restart

```bash
sudo systemctl start fc-agent
sudo systemctl stop fc-agent
sudo systemctl restart fc-agent
```

### Enable/Disable Auto-start

```bash
sudo systemctl enable fc-agent   # Start on boot
sudo systemctl disable fc-agent  # Don't start on boot
```

### View Status

```bash
systemctl status fc-agent
```

## Troubleshooting

### Service Won't Start

1. Check logs:
   ```bash
   sudo journalctl -u fc-agent -n 50
   ```

2. Verify configuration:
   ```bash
   sudo fc-agent --config /etc/firecracker-agent/agent.yaml --validate
   ```

3. Check permissions:
   ```bash
   ls -la /etc/firecracker-agent/
   ls -la /var/lib/firecracker/
   ```

### Network Issues

1. Verify bridge exists:
   ```bash
   ip link show fc-br0
   ```

2. Check IP forwarding:
   ```bash
   sysctl net.ipv4.ip_forward
   ```

3. Verify iptables rules:
   ```bash
   sudo iptables -t nat -L -n -v
   ```

### Permission Denied Errors

1. Check if firecracker user exists:
   ```bash
   id firecracker
   ```

2. Verify KVM access:
   ```bash
   sudo usermod -aG kvm firecracker
   sudo systemctl restart fc-agent
   ```

### Firecracker Not Found

1. Check Firecracker installation:
   ```bash
   which firecracker
   firecracker --version
   ```

2. Update path in config:
   ```bash
   sudo nano /etc/firecracker-agent/agent.yaml
   # Update binary_path to correct location
   ```

## Uninstalling

### Remove Package

```bash
# Remove package (keep configuration)
sudo apt-get remove firecracker-agent

# Purge package (remove everything)
sudo apt-get purge firecracker-agent
```

### Manual Cleanup

If needed, manually remove:

```bash
# Stop service
sudo systemctl stop fc-agent
sudo systemctl disable fc-agent

# Remove files
sudo rm -rf /var/lib/firecracker/vms/*
sudo rm -rf /var/log/firecracker-agent
sudo rm -rf /etc/firecracker-agent

# Remove user
sudo deluser firecracker
sudo delgroup firecracker
```

## Security Considerations

1. **Firewall**: Restrict access to gRPC port (50051)
   ```bash
   sudo ufw allow from 192.168.1.0/24 to any port 50051
   ```

2. **File Permissions**: Ensure configuration files are protected
   ```bash
   sudo chmod 640 /etc/firecracker-agent/agent.yaml
   ```

3. **User Isolation**: The service runs as `firecracker` user (non-root)

4. **Network Isolation**: Use separate bridges for different security zones

## Next Steps

After installation:

1. **Configure mikrom-go**: Update mikrom-go configuration to use this agent
2. **Test VM Creation**: Create a test VM through the API
3. **Monitor Performance**: Check logs and system resources
4. **Backup Configuration**: Save your configuration files

## Support

For issues or questions:

- GitHub Issues: https://github.com/apardo/firecracker-agent/issues
- Documentation: https://github.com/apardo/firecracker-agent/docs
- Email: apardo@spluca.org
