# not-env-backend

HTTP(S) API server for not-env, a self-hosted environment variable management system. Provides a secure, stateless backend that stores environment variables encrypted at rest.

## Quick Reference

| Task | Command |
|------|---------|
| **Start standalone (SQLite)** | `docker run -d -p 1212:1212 -v not-env-data:/data ghcr.io/not-env/not-env-standalone:latest` |
| **Get APP_ADMIN key** | `docker logs not-env-backend | grep "APP_ADMIN key"` |
| **Health check** | `curl http://localhost:1212/health` |

## Overview

Stateless Go HTTP server that:
- Stores environment variables encrypted at rest
- Supports SQLite, PostgreSQL, and MySQL/MariaDB
- Exposes JSON API on port 1212
- Auto-generates master key and APP_ADMIN key if not provided

## Supported Databases

- **SQLite** - Recommended for MVP (standalone image available)
- **PostgreSQL** - Production-ready
- **MySQL/MariaDB** - Production-ready

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `NOT_ENV_MASTER_KEY` | No | Auto-generated | Master encryption key (32 bytes, base64) |
| `NOT_ENV_APP_ADMIN_KEY` | No | Auto-generated | APP_ADMIN API key (for horizontal scaling) |
| `DB_TYPE` | Yes | - | Database type: `sqlite`, `postgres`, or `mysql` |
| `DB_PATH` | Yes (SQLite only) | - | SQLite database file path |
| `DB_HOST` | Yes (PostgreSQL/MySQL) | - | Database host |
| `DB_PORT` | Yes (PostgreSQL/MySQL) | - | Database port |
| `DB_USER` | Yes (PostgreSQL/MySQL) | - | Database user |
| `DB_PASSWORD` | Yes (PostgreSQL/MySQL) | - | Database password |
| `DB_NAME` | Yes (PostgreSQL/MySQL) | - | Database name |

**Note:** Auto-generated keys are displayed in logs on first startup. Save them securely.

## Quick Start

### Standalone SQLite (Recommended)

```bash
docker run -d --name not-env-backend -p 1212:1212 \
  -v not-env-data:/data \
  ghcr.io/not-env/not-env-standalone:latest

# Get APP_ADMIN key
docker logs not-env-backend | grep "APP_ADMIN key"

# Verify health
curl http://localhost:1212/health
```

**Note:** The `-v not-env-data:/data` volume persists the SQLite database.

### Restarting Standalone Container

To restart with the same master key (preserving encrypted data):

```bash
# Stop and remove container (keeps volume)
docker stop not-env-backend
docker rm not-env-backend

# Restart with saved master key
docker run -d --name not-env-backend -p 1212:1212 \
  -v not-env-data:/data \
  -e NOT_ENV_MASTER_KEY="<your-saved-master-key>" \
  ghcr.io/not-env/not-env-standalone:latest
```

**Warning:** If you lose the master key, all encrypted data becomes unrecoverable. Always save the master key from first startup logs.

## Advanced Configuration

### PostgreSQL

```bash
docker run -d --name not-env-backend -p 1212:1212 \
  -e DB_TYPE=postgres \
  -e DB_HOST=postgres.example.com \
  -e DB_PORT=5432 \
  -e DB_USER=notenv \
  -e DB_PASSWORD=secret \
  -e DB_NAME=notenv \
  ghcr.io/not-env/not-env:latest
```

### MySQL/MariaDB

```bash
docker run -d --name not-env-backend -p 1212:1212 \
  -e DB_TYPE=mysql \
  -e DB_HOST=mysql.example.com \
  -e DB_PORT=3306 \
  -e DB_USER=notenv \
  -e DB_PASSWORD=secret \
  -e DB_NAME=notenv \
  ghcr.io/not-env/not-env:latest
```

### From Source

```bash
git clone https://github.com/not-env/not-env-backend.git
cd not-env-backend
export DB_TYPE=sqlite DB_PATH=/tmp/not-env.db
go run main.go
```

## Common Tasks

### Horizontal Scaling (Multiple Instances)

For horizontal scaling, all instances must share the same master key and APP_ADMIN key:

```bash
# Instance 1
docker run -d --name not-env-backend-1 -p 1212:1212 \
  -e DB_TYPE=postgres \
  -e DB_HOST=postgres.example.com \
  -e DB_PORT=5432 \
  -e DB_USER=notenv \
  -e DB_PASSWORD=secret \
  -e DB_NAME=notenv \
  -e NOT_ENV_MASTER_KEY="<shared-master-key>" \
  -e NOT_ENV_APP_ADMIN_KEY="<shared-app-admin-key>" \
  ghcr.io/not-env/not-env:latest

# Instance 2 (same keys, different port)
docker run -d --name not-env-backend-2 -p 1213:1212 \
  -e DB_TYPE=postgres \
  -e DB_HOST=postgres.example.com \
  -e DB_PORT=5432 \
  -e DB_USER=notenv \
  -e DB_PASSWORD=secret \
  -e DB_NAME=notenv \
  -e NOT_ENV_MASTER_KEY="<shared-master-key>" \
  -e NOT_ENV_APP_ADMIN_KEY="<shared-app-admin-key>" \
  ghcr.io/not-env/not-env:latest
```

**Important:**
- All instances must use the same `NOT_ENV_MASTER_KEY` to decrypt data
- All instances must use the same `NOT_ENV_APP_ADMIN_KEY` for consistent admin access
- Use external database (PostgreSQL/MySQL) for multi-instance deployments
- Store keys securely using secrets management (Docker secrets, Kubernetes secrets, etc.)

## Common Scenarios

### Scenario 1: First-time Setup (Standalone)

```bash
# Start container
docker run -d --name not-env-backend -p 1212:1212 \
  -v not-env-data:/data \
  ghcr.io/not-env/not-env-standalone:latest

# Save master key and APP_ADMIN key from logs
docker logs not-env-backend | grep -E "(NOT_ENV_MASTER_KEY|APP_ADMIN key)"
```

**Important:** Save both keys immediately - you'll need the master key to restart, and the APP_ADMIN key to manage environments.

### Scenario 2: Restart Container (Same Data)

```bash
# Stop and remove container (keeps volume)
docker stop not-env-backend && docker rm not-env-backend

# Restart with saved master key
docker run -d --name not-env-backend -p 1212:1212 \
  -v not-env-data:/data \
  -e NOT_ENV_MASTER_KEY="<saved-master-key>" \
  ghcr.io/not-env/not-env-standalone:latest
```

**Note:** The APP_ADMIN key is stored in the database, so you don't need to set it again. However, if you set `NOT_ENV_APP_ADMIN_KEY` explicitly, use the same value.

### Scenario 3: Horizontal Scaling (Multiple Instances)

```bash
# Instance 1
docker run -d --name not-env-1 -p 1212:1212 \
  -e DB_TYPE=postgres \
  -e DB_HOST=postgres.example.com \
  -e DB_PORT=5432 \
  -e DB_USER=notenv \
  -e DB_PASSWORD=secret \
  -e DB_NAME=notenv \
  -e NOT_ENV_MASTER_KEY="<shared-key>" \
  -e NOT_ENV_APP_ADMIN_KEY="<shared-key>" \
  ghcr.io/not-env/not-env:latest

# Instance 2 (same keys, different port)
docker run -d --name not-env-2 -p 1213:1212 \
  -e DB_TYPE=postgres \
  -e DB_HOST=postgres.example.com \
  -e DB_PORT=5432 \
  -e DB_USER=notenv \
  -e DB_PASSWORD=secret \
  -e DB_NAME=notenv \
  -e NOT_ENV_MASTER_KEY="<shared-key>" \
  -e NOT_ENV_APP_ADMIN_KEY="<shared-key>" \
  ghcr.io/not-env/not-env:latest
```

**Load balancing:** Use a load balancer (nginx, HAProxy, cloud load balancer) to distribute traffic across instances.

### Scenario 4: Migration from Standalone to External Database

```bash
# 1. Export data from SQLite (if needed, use CLI to export variables)
# 2. Set up PostgreSQL/MySQL database
# 3. Start new backend with external database and same master key
docker run -d --name not-env-backend -p 1212:1212 \
  -e DB_TYPE=postgres \
  -e DB_HOST=postgres.example.com \
  -e DB_PORT=5432 \
  -e DB_USER=notenv \
  -e DB_PASSWORD=secret \
  -e DB_NAME=notenv \
  -e NOT_ENV_MASTER_KEY="<same-master-key>" \
  -e NOT_ENV_APP_ADMIN_KEY="<same-app-admin-key>" \
  ghcr.io/not-env/not-env:latest
# 4. Recreate environments and variables (master key allows decryption)
```

## API Reference

All requests require `Authorization: Bearer <API_KEY>` header.

### Create Environment

```bash
curl -X POST http://localhost:1212/environments \
  -H "Authorization: Bearer <APP_ADMIN_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"name": "development", "description": "Dev environment"}'
```

**Response:**
```json
{
  "id": 1,
  "name": "development",
  "keys": {
    "env_admin": "...",
    "env_read_only": "..."
  }
}
```

### Set Variable

```bash
curl -X PUT http://localhost:1212/variables/DB_HOST \
  -H "Authorization: Bearer <ENV_ADMIN_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"value": "localhost"}'
```

### Get Variable

```bash
curl -X GET http://localhost:1212/variables/DB_HOST \
  -H "Authorization: Bearer <ENV_READ_ONLY_KEY>"
```

### List Variables

```bash
curl -X GET http://localhost:1212/variables \
  -H "Authorization: Bearer <ENV_READ_ONLY_KEY>"
```

## Troubleshooting

**Backend won't start:**
- Check all required environment variables are set
- Verify database is running and accessible
- Check master key is valid base64 (or let it auto-generate)

**Can't connect to database:**
- Verify `DB_TYPE` is `sqlite`, `postgres`, or `mysql`
- For SQLite: Check `DB_PATH` is writable
- For PostgreSQL/MySQL: Verify connection details and network access

**API returns 401:**
- Check `Authorization: Bearer <API_KEY>` header format
- Verify API key is correct and not revoked
- Ensure key type matches endpoint requirements

## Security Notes

- Store `NOT_ENV_MASTER_KEY` securely - if compromised, all encrypted data is at risk
- Treat API keys like passwords - never commit to version control
- Use HTTPS in production (via reverse proxy or TLS termination)
- Secure your database with strong passwords and network restrictions

## Next Steps

- Use the [CLI](../not-env-cli/README.md) to manage environments and variables
- Use the [JavaScript SDK](../SDKs/not-env-sdk-js/README.md) or [Python SDK](../SDKs/not-env-sdk-python/README.md) in your applications
