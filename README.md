# PHP Server Manager with VLAN Support

A comprehensive PHP development server manager with IPv6 VLAN support for Linux systems.

## Features

- **Multi-server Management**: Create, start, stop, and manage multiple PHP servers
- **VLAN Integration**: Automatic VLAN interface creation with IPv6 addressing
- **IPv6 Support**: Uses prefix `2a0e:b107:384:ee25::/64` with port-based suffixes
- **Authentication**: Password-protected API and web interface
- **Web Interface**: Modern, responsive web UI
- **Port 80**: Fixed to run on standard HTTP port
- **Linux Optimized**: Designed specifically for Linux environments

## IPv6 VLAN Configuration

Each server automatically gets:
- VLAN interface: `vlan{port_number}`
- IPv6 address: `2a0e:b107:384:ee25::{port_number}/64`

Example: Server on port 8080 gets VLAN interface `vlan8080` with IPv6 `2a0e:b107:384:ee25::8080/64`

## Installation

### Prerequisites

- Linux system with root access
- Go 1.21 or later
- FrankenPHP binary

### Quick Setup

1. Run the setup script:
\`\`\`bash
sudo bash scripts/setup.sh
\`\`\`

2. Build the application:
\`\`\`bash
go build -o php-server-manager .
\`\`\`

3. Copy to installation directory:
\`\`\`bash
sudo cp php-server-manager /opt/php-server-manager/
sudo cp -r static /opt/php-server-manager/
\`\`\`

4. Start the service:
\`\`\`bash
sudo systemctl enable php-server-manager
sudo systemctl start php-server-manager
\`\`\`

### Docker Installation

\`\`\`bash
docker-compose up -d
\`\`\`

## Usage

1. Access the web interface at `http://localhost`
2. Login with password: `admin123` (change this!)
3. Create servers with automatic VLAN configuration
4. Start/stop servers as needed

## API Endpoints

### Authentication
- `POST /api/auth/login` - Login with password
- `POST /api/auth/logout` - Logout

### Server Management
- `GET /api/servers` - List all servers
- `POST /api/servers` - Create server (with VLAN)
- `PUT /api/servers/{id}` - Update server
- `DELETE /api/servers/{id}` - Delete server (removes VLAN)
- `POST /api/servers/{id}/start` - Start server
- `POST /api/servers/{id}/stop` - Stop server

### VLAN Management
- `GET /api/vlan/interfaces` - List VLAN interfaces
- `GET /api/vlan/status` - Get VLAN status

## Security

- Password authentication required for all operations
- Session-based authentication with 24-hour expiry
- CORS protection
- Input validation

## Configuration

The application stores configuration in `~/.php-server-manager/config.json`

## Requirements

- Linux kernel with VLAN support (8021q module)
- iproute2 package
- Root privileges for VLAN management
- FrankenPHP for PHP server functionality

## Troubleshooting

1. **VLAN creation fails**: Ensure 8021q module is loaded
2. **Permission denied**: Run with sudo/root privileges
3. **Port conflicts**: Check for existing services on port 80
4. **IPv6 issues**: Verify IPv6 is enabled on the system

## License

MIT License - see LICENSE file for details
