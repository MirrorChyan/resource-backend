# Resource Backend

A high-performance Go-based resource distribution backend service for managing versioned software releases with support for both full and incremental updates, multi-region CDN distribution, and flexible version comparison strategies.

## Features

- **Versioned Resource Management** - Manage software resources with semantic versioning or datetime-based versioning
- **Full & Incremental Updates** - Support both complete package distribution and delta updates
- **Multi-Region CDN** - Weighted region-based distribution with configurable CDN/direct download ratios
- **Release Channels** - Separate stable/beta/alpha channels for gradual rollouts
- **Platform Support** - Multi-platform (OS/Arch) package management
- **Caching Layer** - Ristretto in-memory cache with singleflight pattern for high performance
- **Async Task Queue** - Asynq-based background job processing
- **Health Monitoring** - Prometheus metrics and health check endpoints
- **Service Discovery** - Optional Consul integration for cluster mode

## Tech Stack

- **Framework**: Fiber v2 (high-performance HTTP framework)
- **Database**: MySQL with Ent ORM (Facebook's entity framework)
- **Cache**: Redis + Ristretto (in-memory)
- **Task Queue**: Asynq (Redis-backed)
- **DI**: Google Wire (compile-time dependency injection)
- **Logging**: Uber Zap
- **JSON**: Sonic (bytedance's high-performance JSON library)
- **Version Parsing**: SemVer, DateTime formats (pluggable parser system)

## Prerequisites

- Go 1.25.3+
- MySQL 8.0+
- Redis 6.0+
- (Optional) Consul for service discovery

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/MirrorChyan/resource-backend.git
cd resource-backend
```

### 2. Configure the Application

Edit `config/config.yaml`:

```yaml
instance:
  port: 8000
  only_local: true  # Set to false for cluster mode

database:
  host: "localhost"
  port: 3306
  username: "root"
  password: "your_password"
  name: "resource_db"

redis:
  addr: "localhost:6379"
  db: 0
  asynq_db: 1
```

For cluster mode, set environment variables:
```bash
export INSTANCE_IP="your_instance_ip"
export SERVICE_ID="unique_service_id"
export REGION_ID="region_name"  # Optional, defaults to "default"
```

### 3. Generate Code

```bash
# Generate Ent ORM code
make entgen

# Generate Wire dependency injection code
make wiregen
```

### 4. Build and Run

```bash
# Build binary
make build

# Or run directly
go run .
```

The service will start on `http://localhost:8000`

## API Endpoints

### Public Endpoints

#### Get Latest Version
```http
GET /resources/:rid/latest?channel=stable&system=windows&arch=amd64&current=1.0.0
```

**Query Parameters:**
- `channel` - Release channel: `stable`, `beta`, `alpha`
- `system` - Operating system: `windows`, `linux`, `darwin`, etc.
- `arch` - Architecture: `amd64`, `arm64`, `386`, etc.
- `current` - Current version (optional, for incremental updates)
- `cdk` - CDK token (optional, for authentication)

**Response:**
```json
{
  "code": 0,
  "data": {
    "resource_id": "my-app",
    "version_name": "1.2.0",
    "version_number": 120,
    "channel": "stable",
    "update_type": "incremental",
    "download_url": "https://cdn.example.com/...",
    "release_note": "What's new in 1.2.0...",
    "file_size": 1048576,
    "file_hash": "sha256:abc123..."
  }
}
```

#### Download Resource
```http
GET /resources/download/:key
```

Redirects to the actual download URL (CDN or direct).

#### Head Download Info
```http
HEAD /resources/download/:key
```

Returns file metadata headers:
- `Content-Length`
- `Content-Type`

### Admin Endpoints (Require Authentication)

#### Create Resource
```http
POST /resources
Authorization: Bearer <token>

{
  "resource_id": "my-app",
  "name": "My Application",
  "description": "Description here",
  "update_type": "incremental"
}
```

#### Create Version
```http
POST /resources/:rid/versions
Authorization: Bearer <token>
Content-Type: multipart/form-data

version_name=1.0.0
channel=stable
system=windows
arch=amd64
file=@package.zip
old_version=0.9.0  # For incremental updates
```

#### Update Release Note
```http
PUT /resources/:rid/versions/release-note
Authorization: Bearer <token>

{
  "version_name": "1.0.0",
  "channel": "stable",
  "content": "Release notes..."
}
```

#### Update Custom Data
```http
PUT /resources/:rid/versions/custom-data
Authorization: Bearer <token>

{
  "version_name": "1.0.0",
  "channel": "stable",
  "content": "{\"custom\": \"json data\"}"
}
```

#### Health Check
```http
GET /health
```

#### Metrics
```http
GET /metrics
```

## Architecture

### Layer Structure

```
main.go
  ↓
internal/application/app.go (Adapter pattern)
  ↓
internal/interfaces/rest (HTTP handlers)
  ↓
internal/logic (Business logic)
  ↓
internal/repo (Data access)
  ↓
internal/ent (ORM entities)
```

### Database Schema

**Resource** (Top-level entity)
- `id` - Resource identifier
- `name` - Display name
- `description` - Description
- `update_type` - Default update strategy (full/incremental)

**Version** (Release versions)
- `channel` - Release channel (stable/beta/alpha)
- `name` - Version string (e.g., "1.0.0")
- `number` - Numeric version for comparison
- `release_note` - Changelog
- `custom_data` - Arbitrary JSON data

**Storage** (Platform-specific packages)
- `update_type` - Full or incremental
- `os` - Operating system
- `arch` - Architecture
- `package_path` - File location
- `package_hash_sha256` - Package checksum
- `file_type` - Archive format (zip, tgz, etc.)
- `file_size` - Size in bytes
- `file_hashes` - Hash map of files (for full updates)
- `old_version` - Source version (for incremental updates)

### Configuration Modes

#### Standalone Mode (`only_local: true`)
- Single instance deployment
- Configuration from local YAML files
- No service discovery

#### Cluster Mode (`only_local: false`)
- Multi-instance deployment
- Consul service discovery
- Remote configuration management
- Health check registration
- Automatic service deregistration on shutdown

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run specific test
go test ./internal/pkg/vercomp/vercomp_test.go

# Run with race detector
go test -race ./...
```

### Code Generation

After modifying schemas or dependencies:

```bash
# Regenerate Ent ORM code
make entgen

# Regenerate Wire DI code
make wiregen

# Build
make build
```

### Project Structure

```
resource-backend/
├── cmd/                      # (Optional) Command-line tools
├── config/                   # Configuration files
│   ├── config.yaml          # Default config
│   ├── config.local.yaml    # Local overrides
│   └── config.cluster.yaml  # Cluster config
├── internal/
│   ├── application/         # App lifecycle management
│   ├── cache/               # Cache implementations
│   ├── config/              # Configuration loading
│   ├── db/                  # Database connections
│   ├── ent/                 # Ent ORM (generated)
│   │   └── schema/          # Database schemas
│   ├── interfaces/          # External interfaces
│   │   └── rest/            # HTTP handlers & routing
│   ├── logic/               # Business logic
│   ├── middleware/          # HTTP middleware
│   ├── model/               # Data models & DTOs
│   ├── oss/                 # Object storage integration
│   ├── pkg/                 # Utility packages
│   │   ├── archiver/        # Archive operations
│   │   ├── filehash/        # File hashing
│   │   ├── fileops/         # File operations
│   │   ├── patcher/         # Incremental patching
│   │   ├── validator/       # Request validation
│   │   └── vercomp/         # Version comparison
│   ├── repo/                # Data repositories
│   ├── tasks/               # Async task queue
│   └── wire/                # Dependency injection
├── bin/                     # Build output
├── main.go                  # Application entry point
├── Makefile                 # Build commands
└── CLAUDE.md                # AI development guide
```

## Version Comparison

The service supports pluggable version parsers:

### SemVer Parser
```
1.0.0 < 1.0.1 < 1.1.0 < 2.0.0
v1.0.0 (automatically strips 'v' prefix)
```

### DateTime Parser
```
2024-01-15 < 2024-01-16
2024-01-15T10:30:00Z
20240115103000
```

### Custom Parsers
Implement the `Parser` interface in `internal/pkg/vercomp/`:
```go
type Parser interface {
    Name() string
    CanParse(version string) bool
    Parse(version string) (interface{}, error)
    Compare(a, b interface{}) int
}
```

## CDN Distribution

The service supports weighted multi-region CDN distribution:

### Configuration Example

```yaml
extra:
  cdn_prefix: "https://cdn.example.com"
  distribute_cdn_ratio: 70  # 70% CDN, 30% direct
  distribute_cdn_region: ["default", "hk", "kr"]
  download_prefix_info:
    default:
      - url: "https://download1.example.com"
        weight: 1
      - url: "https://download2.example.com"
        weight: 2
    hk:
      - url: "https://hk-download.example.com"
        weight: 1
```

### How It Works

1. Client requests `/resources/:rid/latest`
2. Server generates temporary download key (10-minute TTL)
3. Server selects distribution method:
   - CDN: Generates authenticated URL with MD5 token
   - Direct: Selects region based on weighted random
4. Client receives download URL
5. Client uses HEAD request to check file size before download

## Monitoring

### Prometheus Metrics

Available at `/metrics`:
- HTTP request counts and durations
- Go runtime metrics (goroutines, memory, GC)
- Custom business metrics

### Health Check

Available at `/health`:
```json
{
  "status": "ok"
}
```

### Logging

Structured logging with Zap:
- Request/response logging (via fiberzap middleware)
- Error tracking with context
- Configurable log levels (debug/info/warn/error)

## Security

- **Authentication**: Bearer token validation via middleware
- **Rate Limiting**: Configurable download limits
- **CDN Token**: MD5-based authentication for CDN URLs
- **Path Validation**: Prevents directory traversal attacks
- **CORS**: Configurable cross-origin policies

## Performance Optimizations

- **Connection Pooling**: MySQL (100 max, 50 idle, 25min lifetime)
- **In-Memory Cache**: Ristretto with singleflight pattern
- **Redis Cache**: Multi-level caching for version metadata
- **Distributed Locks**: Redsync for concurrent operations
- **Async Processing**: Background tasks via Asynq
- **JSON Performance**: Sonic encoder/decoder
- **HTTP Performance**: Fiber framework with fasthttp

## Deployment

### Docker Deployment

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN make entgen && make wiregen && make build

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/bin/app .
COPY --from=builder /app/config ./config
CMD ["./app"]
```

### Environment Variables (Cluster Mode)

```bash
INSTANCE_IP=10.0.1.5
SERVICE_ID=resource-backend-1
REGION_ID=us-west
```

### Database Migration

Schema auto-migration on startup:
```go
if err := mysql.Schema.Create(context.Background()); err != nil {
    // Handle error
}
```

For production, consider using migration tools like `atlas` or `migrate`.

## Troubleshooting

### Common Issues

**Q: Build fails with "package X is not in std"**
A: Ensure Go version 1.25.3+ is installed and GOROOT is set correctly.

**Q: Wire generation fails**
A: Run `go install github.com/google/wire/cmd/wire@latest` first.

**Q: Database connection fails**
A: Check MySQL is running and credentials in `config/config.yaml` are correct.

**Q: Redis connection fails**
A: Verify Redis is running on the configured address (`redis.addr`).

**Q: Consul registration fails (cluster mode)**
A: Ensure `INSTANCE_IP` and `SERVICE_ID` environment variables are set.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Run code generation if needed (`make entgen && make wiregen`)
4. Run tests (`go test ./...`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## License

This project is licensed under the GNU Affero General Public License v3.0 (AGPL-3.0) - see the [LICENSE.md](LICENSE.md) file for details.

### AGPL-3.0 Summary

- ✅ Commercial use
- ✅ Modification
- ✅ Distribution
- ✅ Private use
- ⚠️ **Network use is distribution** - If you run a modified version on a server, you must make the source code available to users
- ⚠️ Disclose source
- ⚠️ License and copyright notice
- ⚠️ Same license (copyleft)
- ⚠️ State changes

For commercial licensing or questions, please contact the project maintainers.

## Contact & Support

- **GitHub**: [https://github.com/MirrorChyan/resource-backend](https://github.com/MirrorChyan/resource-backend)
- **Issues**: [GitHub Issues](https://github.com/MirrorChyan/resource-backend/issues)

## Acknowledgments

- [Fiber](https://gofiber.io/) - Express-inspired web framework
- [Ent](https://entgo.io/) - Entity framework for Go
- [Wire](https://github.com/google/wire) - Compile-time dependency injection
- [Asynq](https://github.com/hibiken/asynq) - Redis-based task queue
- [Sonic](https://github.com/bytedance/sonic) - High-performance JSON library
- [Zap](https://github.com/uber-go/zap) - Blazing fast, structured logging
