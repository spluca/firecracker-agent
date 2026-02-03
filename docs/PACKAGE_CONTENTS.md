# Firecracker Agent - Debian Package Contents

## Package Information

- **Package Name**: firecracker-agent
- **Version**: 0.1.0-1
- **Architecture**: amd64
- **Section**: admin
- **Priority**: optional

## Files Installed by the Package

### Binaries

```
/usr/bin/fc-agent                           # Main executable (755)
```

### Systemd Service

```
/lib/systemd/system/fc-agent.service        # Systemd unit file (644)
```

**Service Details:**
- Service Type: simple
- User: root (needs access to /dev/kvm and network configuration)
- Auto-restart: on-failure with 10s delay
- Capabilities: CAP_NET_ADMIN, CAP_NET_RAW (for TAP/bridge management)
- Resource Limits: 65536 file descriptors, 8192 processes

### Configuration

```
/etc/firecracker-agent/agent.yaml           # Main configuration file (640)
```

### Directories Created

```
/var/lib/firecracker/                       # Main data directory
/var/lib/firecracker/vms/                   # VM storage (755)
/var/lib/firecracker/images/                # Kernel and rootfs images (755)
/var/log/firecracker-agent/                 # Log files (755)
```

### System User & Group

The package creates:
- **User**: firecracker (system user)
- **Group**: firecracker (system group)
- **Home**: /var/lib/firecracker

## Installation Scripts

### postinst (Post-Installation)

Executed after package installation:
1. Creates `firecracker` user and group
2. Sets ownership on data directories
3. Configures file permissions (640 for config, 750 for config dir)
4. Loads kernel modules (tun, br_netfilter)
5. Enables IP forwarding via sysctl
6. Reloads systemd daemon

### prerm (Pre-Removal)

Executed before package removal:
1. Stops fc-agent service if running

### postrm (Post-Removal)

Executed after package removal:
- On **purge**:
  1. Removes all logs
  2. Removes VMs data (not images)
  3. Removes sysctl configuration
  4. Removes firecracker user and group

## Dependencies

### Build Dependencies

- debhelper-compat (>= 13)
- golang-go (>= 2:1.20~)
- dh-golang
- protobuf-compiler
- git

### Runtime Dependencies

**Required:**
- firecracker (>= 1.0.0) or firecracker-bin
- qemu-utils (for qcow2 overlay support)
- iproute2 (for TAP devices and bridge management)
- iptables (for NAT configuration)
- bridge-utils (for bridge management)

**Recommended:**
- linux-image-generic (>= 5.10) - For Firecracker kernel support

## Systemd Integration

### Service Commands

```bash
# Enable service (start at boot)
sudo systemctl enable fc-agent

# Start service
sudo systemctl start fc-agent

# Check status
sudo systemctl status fc-agent

# View logs
sudo journalctl -u fc-agent -f

# Stop service
sudo systemctl stop fc-agent

# Disable service
sudo systemctl disable fc-agent
```

### Service Features

1. **Automatic Restart**: Service restarts on failure
2. **Logging**: Integrated with systemd journal
3. **Resource Limits**: Configured for high-performance workloads
4. **Security**: 
   - ProtectHome=yes (home directories not accessible)
   - ProtectSystem=strict (system directories read-only)
   - DevicePolicy=closed (only specific devices allowed)
5. **Network Capabilities**: CAP_NET_ADMIN for TAP/bridge management
6. **KVM Access**: DeviceAllow for /dev/kvm and /dev/net/tun

## Post-Installation Tasks

After installing the package, you need to:

1. **Install Firecracker Binary** (if not already installed):
   ```bash
   wget https://github.com/firecracker-microvm/firecracker/releases/download/v1.7.0/firecracker-v1.7.0-x86_64.tgz
   tar -xzf firecracker-v1.7.0-x86_64.tgz
   sudo mv release-v1.7.0-x86_64/firecracker-v1.7.0-x86_64 /usr/local/bin/firecracker
   sudo chmod +x /usr/local/bin/firecracker
   ```

2. **Prepare VM Images**:
   ```bash
   # Copy kernel and rootfs to images directory
   sudo cp vmlinux.bin /var/lib/firecracker/images/
   sudo cp rootfs.ext4 /var/lib/firecracker/images/
   sudo chown -R firecracker:firecracker /var/lib/firecracker/images/
   ```

3. **Configure Network Bridge**:
   ```bash
   # Create bridge (or configure via /etc/network/interfaces)
   sudo ip link add name fc-br0 type bridge
   sudo ip addr add 172.16.0.1/24 dev fc-br0
   sudo ip link set fc-br0 up
   ```

4. **Update Configuration**:
   ```bash
   # Edit configuration to match your environment
   sudo nano /etc/firecracker-agent/agent.yaml
   ```

5. **Enable and Start Service**:
   ```bash
   sudo systemctl enable fc-agent
   sudo systemctl start fc-agent
   ```

## Building the Package

```bash
# Install build dependencies
sudo apt-get install -y build-essential debhelper golang-go protobuf-compiler git

# Clone repository
git clone https://github.com/apardo/firecracker-agent.git
cd firecracker-agent

# Build package
./build-deb.sh

# Install package
sudo dpkg -i ../firecracker-agent_0.1.0-1_amd64.deb
sudo apt-get install -f  # Fix any missing dependencies
```

## Package Files Summary

```
firecracker-agent_0.1.0-1_amd64.deb
├── DEBIAN/
│   ├── control          # Package metadata
│   ├── postinst         # Post-installation script
│   ├── prerm            # Pre-removal script
│   └── postrm           # Post-removal script
├── usr/
│   └── bin/
│       └── fc-agent     # Main executable
├── lib/
│   └── systemd/
│       └── system/
│           └── fc-agent.service  # Systemd unit
├── etc/
│   └── firecracker-agent/
│       └── agent.yaml   # Configuration
├── var/
│   ├── lib/
│   │   └── firecracker/
│   │       ├── vms/     # VM data directory
│   │       └── images/  # Images directory
│   └── log/
│       └── firecracker-agent/  # Log directory
```

## Verification

After installation, verify:

```bash
# Check binary
which fc-agent
fc-agent --version

# Check service
systemctl status fc-agent

# Check files
ls -la /etc/firecracker-agent/
ls -la /var/lib/firecracker/
ls -la /lib/systemd/system/fc-agent.service

# Check user
id firecracker

# Check logs
sudo journalctl -u fc-agent
```

## Troubleshooting

### Service Won't Start

1. Check logs:
   ```bash
   sudo journalctl -u fc-agent -n 50 --no-pager
   ```

2. Test binary manually:
   ```bash
   sudo /usr/bin/fc-agent --config /etc/firecracker-agent/agent.yaml
   ```

3. Verify permissions:
   ```bash
   sudo ls -la /etc/firecracker-agent/
   sudo ls -la /var/lib/firecracker/
   ```

### Missing Dependencies

```bash
# Check missing dependencies
dpkg -l | grep firecracker
dpkg -l | grep qemu-utils

# Install manually if needed
sudo apt-get install firecracker qemu-utils iproute2 iptables bridge-utils
```

## Uninstalling

```bash
# Remove but keep configuration
sudo apt-get remove firecracker-agent

# Remove everything (purge)
sudo apt-get purge firecracker-agent

# Remove dependencies no longer needed
sudo apt-get autoremove
```
