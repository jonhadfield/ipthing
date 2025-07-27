#!/bin/bash
set -e

# This script installs ipthing as a systemd service

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo "This script must be run as root"
   exit 1
fi

# Create user and group for the service
if ! id -u ipthing &>/dev/null; then
    useradd -r -s /bin/false -d /opt/ipthing -m ipthing
    echo "Created ipthing user"
fi

# Create directory structure
mkdir -p /opt/ipthing

# Build the binary
echo "Building ipthing..."
make build

# Copy files
cp .local_dist/ipthing_darwin_amd64 /opt/ipthing/
cp ./ipthing.service /etc/systemd/system/

# Set permissions
chown -R ipthing:ipthing /opt/ipthing
chmod 755 /opt/ipthing/ipthing

# Reload systemd
systemctl daemon-reload

echo "Installation complete!"
echo ""
echo "Available commands:"
echo "  systemctl start ipthing      # Start the service"
echo "  systemctl stop ipthing       # Stop the service"
echo "  systemctl restart ipthing    # Restart the service"
echo "  systemctl status ipthing     # Check service status"
echo "  systemctl enable ipthing     # Enable service at boot"
echo "  systemctl disable ipthing    # Disable service at boot"
echo "  journalctl -u ipthing -f     # View logs (follow mode)"
echo "  journalctl -u ipthing -n 50  # View last 50 log lines"
