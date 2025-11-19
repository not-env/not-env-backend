package db

import (
	"database/sql"
	"fmt"
	"time"
)

const (
	// MigrationLockID is a unique ID for the migration advisory lock
	MigrationLockID = 1234567890
)

// RunMigrations runs all pending migrations with advisory lock coordination
func (c *Connection) RunMigrations() error {
	// Acquire advisory lock
	_, err := c.DB.Exec("SELECT pg_advisory_lock($1)", MigrationLockID)
	if err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer func() {
		c.DB.Exec("SELECT pg_advisory_unlock($1)", MigrationLockID)
	}()

	// Create schema_migrations table if it doesn't exist
	_, err = c.DB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Get applied migrations
	applied, err := c.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Run migrations in order
	migrations := []Migration{
		{
			Version: "V1",
			Up:      migrationV1,
		},
	}

	for _, migration := range migrations {
		if applied[migration.Version] {
			continue
		}

		if err := migration.Up(c.DB); err != nil {
			return fmt.Errorf("migration %s failed: %w", migration.Version, err)
		}

		_, err = c.DB.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2)", migration.Version, time.Now())
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", migration.Version, err)
		}
	}

	return nil
}

// Migration represents a database migration
type Migration struct {
	Version string
	Up      func(*sql.DB) error
}

func (c *Connection) getAppliedMigrations() (map[string]bool, error) {
	rows, err := c.DB.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

// migrationV1 creates the initial schema
func migrationV1(db *sql.DB) error {
	queries := []string{
		// organizations table
		`CREATE TABLE IF NOT EXISTS organizations (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			encrypted_data_key BYTEA NOT NULL,
			data_key_nonce BYTEA NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,

		// environments table
		`CREATE TABLE IF NOT EXISTS environments (
			id SERIAL PRIMARY KEY,
			organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			UNIQUE(organization_id, name)
		)`,

		// environment_variables table
		`CREATE TABLE IF NOT EXISTS environment_variables (
			id SERIAL PRIMARY KEY,
			environment_id INTEGER NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
			key VARCHAR(255) NOT NULL,
			value_encrypted BYTEA NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			UNIQUE(environment_id, key)
		)`,

		// api_keys table
		`CREATE TABLE IF NOT EXISTS api_keys (
			id SERIAL PRIMARY KEY,
			key_hash VARCHAR(255) NOT NULL UNIQUE,
			type VARCHAR(50) NOT NULL CHECK (type IN ('APP_ADMIN', 'ENV_ADMIN', 'ENV_READ_ONLY')),
			organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			environment_id INTEGER REFERENCES environments(id) ON DELETE CASCADE,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			revoked_at TIMESTAMP
		)`,

		// Indexes
		`CREATE INDEX IF NOT EXISTS idx_environments_organization_id ON environments(organization_id)`,
		`CREATE INDEX IF NOT EXISTS idx_environment_variables_environment_id ON environment_variables(environment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_organization_id ON api_keys(organization_id)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_environment_id ON api_keys(environment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_type ON api_keys(type)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute migration query: %w", err)
		}
	}

	return nil
}

