package db

import (
	"database/sql"
	"time"
)

// Schema defines the database schema for not-env
type Schema struct{}

// Organization represents an organization in the system
type Organization struct {
	ID              int64     `db:"id"`
	Name            string    `db:"name"`
	EncryptedDataKey []byte   `db:"encrypted_data_key"`
	DataKeyNonce    []byte   `db:"data_key_nonce"`
	CreatedAt       time.Time `db:"created_at"`
}

// Environment represents an environment (equivalent to a .env file)
type Environment struct {
	ID             int64     `db:"id"`
	OrganizationID int64    `db:"organization_id"`
	Name           string    `db:"name"`
	Description    sql.NullString `db:"description"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

// EnvironmentVariable represents a key-value pair in an environment
type EnvironmentVariable struct {
	ID            int64     `db:"id"`
	EnvironmentID int64    `db:"environment_id"`
	Key           string    `db:"key"`
	ValueEncrypted []byte   `db:"value_encrypted"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// APIKey represents an API key for authentication
type APIKey struct {
	ID            int64      `db:"id"`
	KeyHash       string     `db:"key_hash"`
	Type          string     `db:"type"` // APP_ADMIN, ENV_ADMIN, ENV_READ_ONLY
	OrganizationID int64     `db:"organization_id"`
	EnvironmentID  sql.NullInt64 `db:"environment_id"`
	CreatedAt      time.Time `db:"created_at"`
	RevokedAt      sql.NullTime `db:"revoked_at"`
}

// SchemaMigration tracks applied migrations
type SchemaMigration struct {
	Version   string    `db:"version"`
	AppliedAt time.Time `db:"applied_at"`
}

