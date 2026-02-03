#!/bin/bash
set -e

echo "=== Building Firecracker Agent Debian Package ==="

# Check if we're in the right directory
if [ ! -f "go.mod" ] || [ ! -d "debian" ]; then
    echo "Error: Must be run from the firecracker-agent root directory"
    exit 1
fi

# Check for required tools
REQUIRED_TOOLS="dpkg-buildpackage debhelper go protoc"
for tool in $REQUIRED_TOOLS; do
    if ! command -v $tool &> /dev/null; then
        echo "Error: Required tool '$tool' not found"
        echo "Install with: sudo apt-get install build-essential debhelper golang protobuf-compiler"
        exit 1
    fi
done

# Clean previous builds
echo "Cleaning previous builds..."
rm -rf debian/firecracker-agent
rm -f ../firecracker-agent_*.deb
rm -f ../firecracker-agent_*.build*
rm -f ../firecracker-agent_*.changes
rm -f ../firecracker-agent_*.dsc
rm -f ../firecracker-agent_*.tar.*

# Generate protobuf files if needed
echo "Generating protobuf files..."
if [ -f "api/proto/firecracker/v1/firecracker.proto" ]; then
    mkdir -p api/proto/firecracker/v1
    protoc --go_out=. --go_opt=paths=source_relative \
        --go-grpc_out=. --go-grpc_opt=paths=source_relative \
        api/proto/firecracker/v1/firecracker.proto || echo "Note: protoc generation failed, will try during build"
fi

# Download Go dependencies
echo "Downloading Go dependencies..."
go mod download

# Build the package
echo "Building Debian package..."
dpkg-buildpackage -us -uc -b

# Check if build was successful
if [ $? -eq 0 ]; then
    echo ""
    echo "=== Build Successful ==="
    echo ""
    echo "Package created:"
    ls -lh ../firecracker-agent_*.deb
    echo ""
    echo "To install:"
    echo "  sudo dpkg -i ../firecracker-agent_*.deb"
    echo "  sudo apt-get install -f  # Fix dependencies if needed"
    echo ""
    echo "To enable and start the service:"
    echo "  sudo systemctl enable fc-agent"
    echo "  sudo systemctl start fc-agent"
    echo ""
else
    echo ""
    echo "=== Build Failed ==="
    exit 1
fi
