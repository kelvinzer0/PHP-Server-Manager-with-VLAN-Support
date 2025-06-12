#!/bin/bash

# Setup script for PHP Server Manager with VLAN support
echo "Setting up PHP Server Manager with VLAN support..."

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run this script as root (sudo)"
    exit 1
fi

# Install required packages
echo "Installing required packages..."
apt-get update
apt-get install -y iproute2 vlan net-tools

# Load VLAN kernel module
echo "Loading VLAN kernel module..."
modprobe 8021q

# Add VLAN module to load at boot
echo "8021q" >> /etc/modules

# Enable IPv6 forwarding
echo "Enabling IPv6 forwarding..."
echo "net.ipv6.conf.all.forwarding=1" >> /etc/sysctl.conf
sysctl -p

# Create systemd service file
echo "Creating systemd service..."
cat > /etc/systemd/system/php-server-manager.service << EOF
[Unit]
Description=PHP Server Manager with VLAN
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/php-server-manager
ExecStart=/opt/php-server-manager/php-server-manager
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Create installation directory
mkdir -p /opt/php-server-manager

echo "Setup completed!"
echo "To install the application:"
echo "1. Copy the compiled binary to /opt/php-server-manager/"
echo "2. Run: systemctl enable php-server-manager"
echo "3. Run: systemctl start php-server-manager"
echo ""
echo "The application will be available at http://localhost"
echo "Default password: admin123"
