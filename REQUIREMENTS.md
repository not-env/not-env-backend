# not-env-backend Requirements

This document specifies the functional and non-functional requirements for not-env-backend.

## Functional Requirements

### FR1: Environment Variable Storage

**FR1.1:** The backend must store environment variable key-value pairs, grouped into Environments.

**FR1.2:** Each Environment is equivalent to a .env file and belongs to an Organization.

**FR1.3:** Environment variable values must be stored encrypted at rest.

**FR1.4:** Each variable must have a unique key within its Environment.

### FR2: Database Support

**FR2.1:** The backend must support PostgreSQL and PostgreSQL-compatible databases only.

**FR2.2:** Supported databases include:
- PostgreSQL
- AWS Aurora Postgres
- YugabyteDB (Postgres-compatible mode)
- CockroachDB (Postgres-compatible mode)

**FR2.3:** The backend must use PostgreSQL drivers and SQL features compatible across these databases.

**FR2.4:** Other database types (MySQL, SQLite, etc.) are explicitly out of scope for v1.

### FR3: Startup Flow

**FR3.1:** On startup, the backend must validate that all required environment variables are present:
- `pg_url`
- `pg_username`
- `pg_password`
- `pg_db_name`
- `NOT_ENV_MASTER_KEY`

**FR3.2:** If any required environment variable is missing, the backend must exit with a clear error message.

**FR3.3:** The backend must connect to the PostgreSQL server using `pg_url`, `pg_username`, and `pg_password`.

**FR3.4:** The backend must check if `pg_db_name` exists. If it doesn't exist, the backend must attempt to create it.

**FR3.5:** If database creation fails because the database already exists (race condition), this must be treated as success.

**FR3.6:** The backend must connect to `pg_db_name` after ensuring it exists.

**FR3.7:** The backend must acquire a PostgreSQL advisory lock before running migrations.

**FR3.8:** If the lock cannot be acquired immediately, the backend must wait until it becomes available.

**FR3.9:** Once the lock is acquired, the backend must run all pending migrations sequentially.

**FR3.10:** After migrations complete, the backend must release the advisory lock.

**FR3.11:** The backend must ensure a default organization exists:
- If no organization exists, create one with name "default"
- Generate a DEK (Data Encryption Key) for the organization
- Encrypt the DEK with the master key
- Store the encrypted DEK and nonce in the database
- If an organization already exists (race condition), use the existing one

**FR3.12:** The backend must ensure exactly one APP_ADMIN key exists:
- If no APP_ADMIN key exists, generate and insert one
- If an APP_ADMIN key already exists (race condition), use the existing one
- Display the APP_ADMIN key in logs on first startup

**FR3.13:** The backend must start an HTTP server listening on port 1212.

**FR3.14:** The port 1212 is not configurable.

### FR4: Database Schema

**FR4.1:** The backend must maintain a `schema_migrations` table with:
- `version` (VARCHAR, PRIMARY KEY)
- `applied_at` (TIMESTAMP)

**FR4.2:** The backend must maintain an `organizations` table with:
- `id` (SERIAL, PRIMARY KEY)
- `name` (VARCHAR, UNIQUE)
- `encrypted_data_key` (BYTEA)
- `data_key_nonce` (BYTEA)
- `created_at` (TIMESTAMP)

**FR4.3:** The backend must maintain an `environments` table with:
- `id` (SERIAL, PRIMARY KEY)
- `organization_id` (INTEGER, FK → organizations.id, ON DELETE CASCADE)
- `name` (VARCHAR)
- `description` (TEXT, nullable)
- `created_at` (TIMESTAMP)
- `updated_at` (TIMESTAMP)
- UNIQUE constraint on (organization_id, name)

**FR4.4:** The backend must maintain an `environment_variables` table with:
- `id` (SERIAL, PRIMARY KEY)
- `environment_id` (INTEGER, FK → environments.id, ON DELETE CASCADE)
- `key` (VARCHAR)
- `value_encrypted` (BYTEA)
- `created_at` (TIMESTAMP)
- `updated_at` (TIMESTAMP)
- UNIQUE constraint on (environment_id, key)

**FR4.5:** The backend must maintain an `api_keys` table with:
- `id` (SERIAL, PRIMARY KEY)
- `key_hash` (VARCHAR, UNIQUE)
- `type` (VARCHAR, CHECK constraint: 'APP_ADMIN', 'ENV_ADMIN', 'ENV_READ_ONLY')
- `organization_id` (INTEGER, FK → organizations.id, ON DELETE CASCADE)
- `environment_id` (INTEGER, FK → environments.id, ON DELETE CASCADE, nullable)
- `created_at` (TIMESTAMP)
- `revoked_at` (TIMESTAMP, nullable)

**FR4.6:** The backend must create indexes on:
- `environments.organization_id`
- `environment_variables.environment_id`
- `api_keys.organization_id`
- `api_keys.environment_id`
- `api_keys.type`

### FR5: Encryption

**FR5.1:** The backend must use a master key from `NOT_ENV_MASTER_KEY` environment variable.

**FR5.2:** The master key must be base64-encoded and 32 bytes (256 bits) when decoded.

**FR5.3:** The backend must generate a unique DEK (Data Encryption Key) for each organization.

**FR5.4:** Each DEK must be 32 bytes (256 bits).

**FR5.5:** The backend must encrypt each organization's DEK with the master key using AES-GCM-256.

**FR5.6:** The backend must store the encrypted DEK and nonce in the `organizations` table.

**FR5.7:** The backend must encrypt environment variable values using the organization's DEK with AES-GCM-256.

**FR5.8:** The backend must store encrypted values with their nonces (prepended to ciphertext).

**FR5.9:** Plaintext values and keys must never be logged.

**FR5.10:** The master key must never be stored in the database.

### FR6: Authentication

**FR6.1:** All API requests must include authentication via Bearer token in the Authorization header:
```
Authorization: Bearer <API_KEY>
```

**FR6.2:** The backend must validate the API key on every request.

**FR6.3:** The backend must resolve the API key to:
- Key identity (from `api_keys` table)
- Key type (APP_ADMIN, ENV_ADMIN, ENV_READ_ONLY)
- Organization ID
- Environment ID (if applicable)

**FR6.4:** API keys must be hashed with bcrypt before storage.

**FR6.5:** The backend must compare incoming API keys against stored hashes.

**FR6.6:** Revoked keys (where `revoked_at` is not NULL) must be rejected.

### FR7: Permission Model

**FR7.1:** APP_ADMIN keys:
- Scope: organization-level
- Can create/list/delete environments
- Can see environment-level keys
- Cannot manage variables directly

**FR7.2:** ENV_ADMIN keys:
- Scope: single environment
- Can read/update environment metadata
- Can see ENV_ADMIN and ENV_READ_ONLY keys for that environment
- Can create/read/update/delete variables in that environment

**FR7.3:** ENV_READ_ONLY keys:
- Scope: single environment
- Can read environment metadata
- Can read variables
- Cannot modify anything
- Cannot see any API keys

### FR8: API Endpoints

**FR8.1:** POST /environments (APP_ADMIN)
- Create a new environment
- Return environment ID and generated ENV_ADMIN/ENV_READ_ONLY keys

**FR8.2:** GET /environments (APP_ADMIN)
- List all environments in the organization

**FR8.3:** DELETE /environments/{id} (APP_ADMIN)
- Delete an environment and all its variables and keys

**FR8.4:** GET /environment (ENV_*)
- Get current environment metadata

**FR8.5:** PATCH /environment (ENV_ADMIN)
- Update environment name and/or description

**FR8.6:** GET /environment/keys (ENV_ADMIN)
- Get ENV_ADMIN and ENV_READ_ONLY keys for current environment

**FR8.7:** GET /variables (ENV_*)
- List all variables in current environment (decrypted)

**FR8.8:** GET /variables/{key} (ENV_*)
- Get a single variable by key (decrypted)

**FR8.9:** PUT /variables/{key} (ENV_ADMIN)
- Create or update a variable

**FR8.10:** DELETE /variables/{key} (ENV_ADMIN)
- Delete a variable

**FR8.11:** GET /health
- Health check endpoint (no authentication required)
- Returns 200 OK

### FR9: API Key Generation

**FR9.1:** When creating an environment, the backend must automatically generate:
- One ENV_ADMIN key
- One ENV_READ_ONLY key

**FR9.2:** API keys must be cryptographically secure random values (32+ bytes, base64-encoded).

**FR9.3:** Generated keys must be returned in the create environment response.

### FR10: Multi-Instance Support

**FR10.1:** The backend must be stateless and support horizontal scaling.

**FR10.2:** Multiple backend instances must be able to run simultaneously behind a load balancer.

**FR10.3:** All instances must share the same database configuration and master key.

**FR10.4:** Migrations must be coordinated using PostgreSQL advisory locks.

**FR10.5:** Only one instance should run migrations at a time; others must wait.

**FR10.6:** Organization and APP_ADMIN key creation must be idempotent:
- Use database constraints and/or check-before-insert logic
- Handle duplicate insert attempts from concurrent instances cleanly
- Treat "already exists" as success

## Non-Functional Requirements

### NFR1: Performance

**NFR1.1:** The backend must handle at least 100 requests per second per instance.

**NFR1.2:** API response times should be under 100ms for 95% of requests (excluding network latency).

### NFR2: Security

**NFR2.1:** All client connections must use HTTPS (enforced at external interface or reverse proxy).

**NFR2.2:** Environment variable values must be encrypted at rest.

**NFR2.3:** API keys must be hashed before storage.

**NFR2.4:** Plaintext values and keys must never be logged.

**NFR2.5:** The backend must validate all input and sanitize database queries.

### NFR3: Reliability

**NFR3.1:** The backend must handle database connection failures gracefully.

**NFR3.2:** The backend must retry database operations where appropriate.

**NFR3.3:** The backend must provide clear error messages for all failure modes.

### NFR4: Scalability

**NFR4.1:** The backend must support horizontal scaling (multiple instances).

**NFR4.2:** Adding capacity must be done by starting additional backend containers.

**NFR4.3:** All instances must share the same database and master key.

### NFR5: Observability

**NFR5.1:** The backend must log startup information (APP_ADMIN key, organization ID).

**NFR5.2:** The backend must log errors with sufficient context.

**NFR5.3:** The backend must provide a health check endpoint.

### NFR6: Compatibility

**NFR6.1:** The backend must use PostgreSQL features compatible with:
- PostgreSQL 12+
- AWS Aurora Postgres
- YugabyteDB (Postgres-compatible mode)
- CockroachDB (Postgres-compatible mode)

**NFR6.2:** The backend must use standard HTTP/JSON for all API communication.

## Implementation Constraints

### IC1: Technology Stack

**IC1.1:** Language: Go 1.21+

**IC1.2:** Database driver: `github.com/lib/pq`

**IC1.3:** Encryption: `crypto/aes` with GCM mode

**IC1.4:** Key hashing: `golang.org/x/crypto/bcrypt`

**IC1.5:** HTTP server: Standard library `net/http`

### IC2: Port Configuration

**IC2.1:** The backend must listen on port 1212.

**IC2.2:** Port 1212 is not configurable.

**IC2.3:** External port mapping is done via Docker/Kubernetes/load balancer.

### IC3: Data Model

**IC3.1:** In v1, there is exactly one organization per deployment.

**IC3.2:** The schema must support multiple organizations for future multi-tenant support.

