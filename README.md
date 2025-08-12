# DNSMasq OpenWRT API

REST API service for managing dnsmasq address entries on OpenWrt systems via UCI configuration interface, with accompanying command-line client for administrative operations.

## Architecture

### API Server (`dnsmassq-api-driver.py`)
Flask-based REST service providing CRUD operations for DNS record management through OpenWrt's Unified Configuration Interface (UCI). Runs as an OpenWrt `init.d` service using `procd`.

### CLI Client (`dnscli`)
Go-based command-line tool for remote administration of the DNS API. Supports configuration persistence, table-formatted output, and simple authentication.

## Project Structure

```
.
├── client
│   └── main.go                 # CLI client (Go)
└── server
    ├── dnsmassq-api-driver.py  # Python API service
    └── dnsmassq-driver         # OpenWrt init.d script
```

## Installation

### Prerequisites

- OpenWrt system with UCI support
- Python 3 and pip
- Go 1.19+ (for building CLI client)

### OpenWrt Deployment

#### 1. Install Dependencies

```bash
opkg update
opkg install python3 python3-pip
pip3 install flask
```

#### 2. Install API Service

```bash
# Copy init.d service script
mv server/dnsmassq-driver /etc/init.d/dnsmassq-driver
chmod +x /etc/init.d/dnsmassq-driver

# Enable and start on boot
/etc/init.d/dnsmassq-driver enable
/etc/init.d/dnsmassq-driver start
```

#### 3. Init Script Configuration

Create `/etc/init.d/dnsmassq-driver`:

```sh
#!/bin/sh /etc/rc.common
START=99
STOP=10
USE_PROCD=1

start_service() {
    procd_open_instance
    procd_set_param command /usr/bin/python3 /root/dnsmasq-driver/dnsmassq-api-driver.py
    procd_set_param respawn
    procd_close_instance
}
```

**Note:** Ensure the path `/root/dnsmasq-driver/dnsmassq-api-driver.py` matches your API script location. The `procd_set_param respawn` directive automatically restarts the service on failure.

#### 4. API Key Configuration

```bash
# Environment variable method
export API_KEY="your-secret-key-here"

# Or via .env file (in same directory as dnsmassq-api-driver.py)
echo "API_KEY=your-secret-key-here" > .env
```

#### 5. Service Verification

```bash
curl -s http://<router-ip>:8080/health
# Expected response: {"status": "ok"}
```

### CLI Client Setup

#### 1. Build Client

```bash
cd client
go build -o dnscli main.go
```

#### 2. Client Configuration

```bash
./dnscli --setup
```

This command prompts for server URL and API key, storing configuration in `~/.dnscli/config.json`.

#### 3. Usage Examples

```bash
# List all DNS records
./dnscli --list

# Add a DNS record
./dnscli --add --domain api.local --ip 192.168.1.100

# Update existing record
./dnscli --update --domain api.local --new-ip 192.168.1.101

# Delete a record
./dnscli --delete --domain api.local
```

## API Reference

### Endpoints

| Method | Endpoint  | Description            | Authentication |
| ------ | --------- | ---------------------- | -------------- |
| GET    | `/health` | Health check           | No             |
| GET    | `/dns`    | List DNS records       | Required       |
| POST   | `/dns`    | Add new record         | Required       |
| PUT    | `/dns`    | Update existing record | Required       |
| DELETE | `/dns`    | Delete record          | Required       |

### Authentication

All endpoints except `/health` require the `X-API-Key` header.

### Request Examples

```bash
# Add DNS record
curl -X POST http://192.168.1.1:8080/dns \
  -H "X-API-Key: your-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"domain": "api.local", "ip": "192.168.1.100"}'

# List records
curl -X GET http://192.168.1.1:8080/dns \
  -H "X-API-Key: your-secret-key"

# Update record
curl -X PUT http://192.168.1.1:8080/dns \
  -H "X-API-Key: your-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"domain": "api.local", "ip": "192.168.1.101"}'

# Delete record
curl -X DELETE http://192.168.1.1:8080/dns \
  -H "X-API-Key: your-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"domain": "api.local"}'
```

## Technical Implementation

### Backend Operations
- Utilizes UCI commands to manage dnsmasq static address entries (`dhcp.@dnsmasq[0].address`)
- Automatically commits configuration changes and reloads dnsmasq service
- Thread-safe API operations implemented using Python threading locks
- Configuration persistence through OpenWrt's UCI system

### Client Features
- Configuration stored in `~/.dnscli/config.json`
- Tabular output formatting for record listings
- Error handling and validation for API communications

## Security Considerations

- Use cryptographically strong API keys
- Deploy behind HTTPS proxy for WAN-accessible installations
- Implement IP allowlists or additional authentication layers for production use
- Regular API key rotation recommended

## Development Roadmap

- HTTPS support integration (native or via reverse proxy)
- Syslog integration for OpenWrt logging
- Batch operations support in CLI client
- Record validation and conflict resolution
- Configuration backup and restore functionality

## Contributing

1. Fork the repository
2. Create feature branch (`git checkout -b feature/new-feature`)
3. Commit changes (`git commit -am 'Add new feature'`)
4. Push to branch (`git push origin feature/new-feature`)
5. Create Pull Request

## License

This project is licensed under the MIT License. See LICENSE file for details.

## Support

For issues and feature requests, please use the GitHub issue tracker.