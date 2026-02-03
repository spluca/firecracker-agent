#!/bin/bash
set -e

echo "=== Firecracker Agent Installation Script ==="
echo

# Check if running as root
if [ "$EUID" -ne 0 ]; then
  echo "❌ Please run as root or with sudo"
  exit 1
fi

# Check prerequisites
echo "✓ Checking prerequisites..."

if ! command -v firecracker &> /dev/null; then
  echo "⚠️  Warning: firecracker not found in PATH"
  echo "   Install from: https://github.com/firecracker-microvm/firecracker/releases"
fi

if ! command -v jailer &> /dev/null; then
  echo "⚠️  Warning: jailer not found in PATH"
fi

# Create directories
echo "✓ Creating directories..."
mkdir -p /etc/fc-agent
mkdir -p /srv/firecracker/vms
mkdir -p /var/lib/firecracker/kernels
mkdir -p /var/lib/firecracker/rootfs
mkdir -p /var/log/fc-agent

# Copy binary
echo "✓ Installing binary..."
if [ -f "bin/fc-agent" ]; then
  cp bin/fc-agent /usr/local/bin/
  chmod +x /usr/local/bin/fc-agent
else
  echo "❌ Binary not found. Run 'make build' first"
  exit 1
fi

# Copy config
echo "✓ Installing configuration..."
if [ -f "configs/agent.yaml" ]; then
  cp configs/agent.yaml /etc/fc-agent/
else
  echo "❌ Configuration not found"
  exit 1
fi

# Install systemd service
echo "✓ Installing systemd service..."
cp scripts/fc-agent.service /etc/systemd/system/
systemctl daemon-reload

echo
echo "=== Installation Complete ==="
echo
echo "Next steps:"
echo "  1. Edit configuration: /etc/fc-agent/agent.yaml"
echo "  2. Enable service: sudo systemctl enable fc-agent"
echo "  3. Start service: sudo systemctl start fc-agent"
echo "  4. Check status: sudo systemctl status fc-agent"
echo "  5. View logs: sudo journalctl -u fc-agent -f"
echo
