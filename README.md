# not-env-backend

not-env-backend is the HTTP(S) API server for not-env, a self-hosted environment variable management system. It provides a secure, horizontally scalable backend that stores environment variables encrypted at rest.

## Overview

The backend is a stateless Go HTTP server that:
- Stores environment variable key-value pairs centrally, grouped into Environments
- Exposes a secure HTTPS + JSON API on port 1212
- Persists data in PostgreSQL or PostgreSQL-compatible databases
- Encrypts environment variable values at rest with organization-level data keys
- Supports horizontal scaling (multiple backend containers behind a load balancer)

## Supported Databases

**PostgreSQL and PostgreSQL-compatible databases only.**

This includes:
- PostgreSQL
- AWS Aurora Postgres
- YugabyteDB (Postgres-compatible mode)
- CockroachDB (Postgres-compatible mode)

Other database types (MySQL, SQLite, etc.) are not supported in v1.

## Prerequisites

- Go 1.21 or later
- PostgreSQL 12+ or compatible database
- A base64-encoded 32-byte (256-bit) master key for encryption

## Configuration

The backend requires the following environment variables:

- `pg_url` - PostgreSQL server host and port (e.g., `postgres:5432`)
- `pg_username` - PostgreSQL username
- `pg_password` - PostgreSQL password
- `pg_db_name` - Database name (will be created if it doesn't exist)
- `NOT_ENV_MASTER_KEY` - Base64-encoded 32-byte master key for encryption

### Generating a Master Key

Generate a secure master key:

```bash
# Generate 32 random bytes and base64 encode
openssl rand -base64 32
```

Save this key securely. You'll need it to start the backend.

## Quick Start with Docker Compose

1. **Generate a master key:**

```bash
export NOT_ENV_MASTER_KEY=$(openssl rand -base64 32)
echo "Master key: $NOT_ENV_MASTER_KEY"
```

2. **Start the services:**

```bash
docker-compose up -d
```

3. **Check the logs to get your APP_ADMIN key:**

```bash
docker-compose logs backend | grep "APP_ADMIN key"
```

You should see output like:
```
not-env-backend | APP_ADMIN key: <your-app-admin-key-here>
not-env-backend | IMPORTANT: Save this APP_ADMIN key securely. It will not be shown again.
```

**Important:** Save this APP_ADMIN key immediately. You'll need it to create environments and manage the system.

4. **Verify the backend is running:**

```bash
curl http://localhost:1212/health
```

If this works correctly, you should see:
```
OK
```

## Manual Setup

1. **Set environment variables:**

```bash
export pg_url="localhost:5432"
export pg_username="notenv"
export pg_password="notenv_password"
export pg_db_name="notenv"
export NOT_ENV_MASTER_KEY="<your-base64-encoded-32-byte-key>"
```

2. **Ensure PostgreSQL is running and accessible**

3. **Run the backend:**

```bash
go run main.go
```

4. **Check the console output for the APP_ADMIN key**

The backend will:
- Connect to PostgreSQL
- Create the database if it doesn't exist
- Run migrations
- Create a default organization
- Generate and display an APP_ADMIN key

## API Usage Examples

All API requests require authentication via Bearer token in the Authorization header:

```
Authorization: Bearer <API_KEY>
```

### 1. Create an Environment

Using the APP_ADMIN key:

```bash
curl -X POST http://localhost:1212/environments \
  -H "Authorization: Bearer <APP_ADMIN_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "development",
    "description": "Development environment"
  }'
```

**Expected response:**

```json
{
  "id": 1,
  "organization_id": 1,
  "name": "development",
  "description": "Development environment",
  "created_at": "2024-01-15T10:30:00Z",
  "keys": {
    "env_admin": "dGVzdF9lbnZfYWRtaW5fa2V5X2hlcmU...",
    "env_read_only": "dGVzdF9lbnZfcmVhZG9ubHlfa2V5X2hlcmU..."
  }
}
```

**If this works correctly, you should see:**
- A JSON response with the environment ID
- Two API keys: `env_admin` and `env_read_only`
- Save these keys - you'll need them to manage variables in this environment

### 2. List All Environments

```bash
curl -X GET http://localhost:1212/environments \
  -H "Authorization: Bearer <APP_ADMIN_KEY>"
```

**Expected response:**

```json
{
  "environments": [
    {
      "id": 1,
      "organization_id": 1,
      "name": "development",
      "description": "Development environment",
      "created_at": "2024-01-15T10:30:00Z",
      "updated_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

**If this works correctly, you should see:**
- A list of all environments in your organization
- Each environment includes its ID, name, and timestamps

### 3. Set a Variable

Using an ENV_ADMIN key for the environment:

```bash
curl -X PUT http://localhost:1212/variables/DB_HOST \
  -H "Authorization: Bearer <ENV_ADMIN_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "value": "localhost"
  }'
```

**Expected response:**
- HTTP 204 No Content (success)

**If this works correctly, you should see:**
- No response body, just a 204 status code
- The variable is now stored encrypted in the database

### 4. Get a Variable

Using an ENV_READ_ONLY or ENV_ADMIN key:

```bash
curl -X GET http://localhost:1212/variables/DB_HOST \
  -H "Authorization: Bearer <ENV_READ_ONLY_KEY>"
```

**Expected response:**

```json
{
  "key": "DB_HOST",
  "value": "localhost",
  "created_at": "2024-01-15T10:35:00Z",
  "updated_at": "2024-01-15T10:35:00Z"
}
```

**If this works correctly, you should see:**
- The variable key and decrypted value
- Creation and update timestamps

### 5. List All Variables

```bash
curl -X GET http://localhost:1212/variables \
  -H "Authorization: Bearer <ENV_READ_ONLY_KEY>"
```

**Expected response:**

```json
{
  "variables": [
    {
      "key": "DB_HOST",
      "value": "localhost",
      "created_at": "2024-01-15T10:35:00Z",
      "updated_at": "2024-01-15T10:35:00Z"
    },
    {
      "key": "DB_PASSWORD",
      "value": "secret123",
      "created_at": "2024-01-15T10:36:00Z",
      "updated_at": "2024-01-15T10:36:00Z"
    }
  ]
}
```

**If this works correctly, you should see:**
- All variables in the environment
- Each variable with its decrypted value
- Variables sorted alphabetically by key

### 6. Get Current Environment Metadata

```bash
curl -X GET http://localhost:1212/environment \
  -H "Authorization: Bearer <ENV_READ_ONLY_KEY>"
```

**Expected response:**

```json
{
  "id": 1,
  "organization_id": 1,
  "name": "development",
  "description": "Development environment",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

**If this works correctly, you should see:**
- The environment metadata for the key you're using
- This endpoint works with any ENV_* key type

## Horizontal Scaling

The backend is stateless and supports horizontal scaling:

1. **Start multiple backend containers** with the same configuration:
   - Same `pg_url`, `pg_username`, `pg_password`, `pg_db_name`
   - Same `NOT_ENV_MASTER_KEY`

2. **Place them behind a load balancer** (nginx, HAProxy, cloud load balancer, etc.)

3. **All instances share:**
   - The same PostgreSQL database
   - The same encryption keys
   - The same API keys

4. **Migrations are coordinated** using PostgreSQL advisory locks - only one instance runs migrations, others wait

## Database Connectivity

To verify PostgreSQL connectivity:

```bash
# Test connection
psql -h localhost -U notenv -d notenv -c "SELECT 1;"
```

If using Docker Compose:

```bash
docker-compose exec postgres psql -U notenv -d notenv -c "SELECT 1;"
```

## Troubleshooting

### Backend won't start

- **Check environment variables:** Ensure all 5 required variables are set
- **Check PostgreSQL:** Verify the database is running and accessible
- **Check master key:** Ensure `NOT_ENV_MASTER_KEY` is a valid base64-encoded 32-byte value

### Can't connect to database

- **Check `pg_url`:** Should be `host:port` format (e.g., `localhost:5432`)
- **Check credentials:** Verify `pg_username` and `pg_password` are correct
- **Check network:** Ensure the backend can reach the PostgreSQL server

### Migrations failing

- **Check logs:** Look for specific migration errors
- **Check database permissions:** Ensure the user can create tables and use advisory locks
- **Check for locks:** If migrations are stuck, verify no other instance is holding the lock

### API returns 401 Unauthorized

- **Check Authorization header:** Must be `Bearer <API_KEY>`
- **Verify API key:** Ensure the key is correct and not revoked
- **Check key type:** Some endpoints require specific key types (APP_ADMIN, ENV_ADMIN, etc.)

## Security Notes

- **Master key:** Store `NOT_ENV_MASTER_KEY` securely. If compromised, all encrypted data is at risk.
- **API keys:** Treat API keys like passwords. Never commit them to version control.
- **HTTPS:** In production, use HTTPS (via reverse proxy or TLS termination) to encrypt traffic in transit.
- **Database:** Secure your PostgreSQL instance with strong passwords and network restrictions.

## Next Steps

- Use the [CLI](../not-env-cli/README.md) to manage environments and variables
- Use the [JavaScript SDK](../not-env-sdk-js/README.md) to load variables in your applications

