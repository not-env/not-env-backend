package db

import (
	"database/sql"
	"time"
)

// Organization represents an organization in the system
type Organization struct {
	ID              int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name            string    `gorm:"type:varchar(255);uniqueIndex;not null" json:"name"`
	EncryptedDataKey []byte   `gorm:"type:blob;not null" json:"encrypted_data_key"`
	DataKeyNonce    []byte   `gorm:"type:blob;not null" json:"data_key_nonce"`
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
	
	// Relationships
	Environments []Environment `gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE" json:"environments,omitempty"`
	APIKeys      []APIKey      `gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE" json:"api_keys,omitempty"`
}

// TableName specifies the table name for Organization
func (Organization) TableName() string {
	return "organizations"
}

// Environment represents an environment (equivalent to a .env file)
type Environment struct {
	ID             int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	OrganizationID int64          `gorm:"not null;index" json:"organization_id"`
	Name           string         `gorm:"type:varchar(255);not null" json:"name"`
	Description    sql.NullString `gorm:"type:text" json:"description,omitempty"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	
	// Relationships
	Organization      Organization          `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	Variables         []EnvironmentVariable `gorm:"foreignKey:EnvironmentID;constraint:OnDelete:CASCADE" json:"variables,omitempty"`
	APIKeys           []APIKey              `gorm:"foreignKey:EnvironmentID;constraint:OnDelete:CASCADE" json:"api_keys,omitempty"`
}

// TableName specifies the table name for Environment
func (Environment) TableName() string {
	return "environments"
}

// EnvironmentVariable represents a key-value pair in an environment
type EnvironmentVariable struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	EnvironmentID int64    `gorm:"not null;index" json:"environment_id"`
	Key           string    `gorm:"type:varchar(255);not null" json:"key"`
	ValueEncrypted []byte   `gorm:"type:blob;not null" json:"value_encrypted"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	
	// Relationships
	Environment Environment `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
}

// TableName specifies the table name for EnvironmentVariable
func (EnvironmentVariable) TableName() string {
	return "environment_variables"
}

// APIKey represents an API key for authentication
type APIKey struct {
	ID            int64         `gorm:"primaryKey;autoIncrement" json:"id"`
	KeyHash       string        `gorm:"type:varchar(255);uniqueIndex;not null" json:"key_hash"`
	Type          string        `gorm:"type:varchar(50);not null;check:type IN ('APP_ADMIN', 'ENV_ADMIN', 'ENV_READ_ONLY')" json:"type"`
	OrganizationID int64        `gorm:"not null;index" json:"organization_id"`
	EnvironmentID  sql.NullInt64 `gorm:"index" json:"environment_id,omitempty"`
	CreatedAt      time.Time    `gorm:"autoCreateTime" json:"created_at"`
	RevokedAt      sql.NullTime `json:"revoked_at,omitempty"`
	
	// Relationships
	Organization Organization `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	Environment  *Environment `gorm:"foreignKey:EnvironmentID" json:"environment,omitempty"`
}

// TableName specifies the table name for APIKey
func (APIKey) TableName() string {
	return "api_keys"
}

// SchemaMigration tracks applied migrations
type SchemaMigration struct {
	Version   string    `gorm:"primaryKey;type:varchar(255)" json:"version"`
	AppliedAt time.Time `gorm:"autoCreateTime" json:"applied_at"`
}

// TableName specifies the table name for SchemaMigration
func (SchemaMigration) TableName() string {
	return "schema_migrations"
}

