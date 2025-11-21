package db

import (
	"fmt"
	"os"
	"time"
)

const (
	// MigrationLockID is a unique ID for the migration advisory lock (PostgreSQL only)
	MigrationLockID = 1234567890
)

// RunMigrations runs all pending migrations with database-specific locking
func (c *Connection) RunMigrations() error {
	dbType := os.Getenv("DB_TYPE")

	// Handle migration locking based on database type
	if dbType == "postgres" {
		// Use PostgreSQL advisory locks
		sqlDB, err := c.DB.DB()
		if err != nil {
			return fmt.Errorf("failed to get underlying sql.DB: %w", err)
		}
		_, err = sqlDB.Exec("SELECT pg_advisory_lock($1)", MigrationLockID)
		if err != nil {
			return fmt.Errorf("failed to acquire migration lock: %w", err)
		}
		defer func() {
			sqlDB.Exec("SELECT pg_advisory_unlock($1)", MigrationLockID)
		}()
	} else if dbType == "mysql" {
		// Use table-based locking for MySQL
		sqlDB, err := c.DB.DB()
		if err != nil {
			return fmt.Errorf("failed to get underlying sql.DB: %w", err)
		}
		// Create lock table if it doesn't exist
		_, err = sqlDB.Exec(`
			CREATE TABLE IF NOT EXISTS migration_lock (
				id INT PRIMARY KEY,
				locked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			) ENGINE=InnoDB
		`)
		if err != nil {
			return fmt.Errorf("failed to create migration lock table: %w", err)
		}

		// Try to acquire lock (insert with unique constraint)
		_, err = sqlDB.Exec("INSERT INTO migration_lock (id) VALUES (1)")
		if err != nil {
			// Lock already held, wait a bit and check
			time.Sleep(100 * time.Millisecond)
			var count int
			err = sqlDB.QueryRow("SELECT COUNT(*) FROM migration_lock WHERE id = 1").Scan(&count)
			if err != nil || count == 0 {
				return fmt.Errorf("failed to acquire migration lock")
			}
			// Another instance is running migrations, wait for it
			return fmt.Errorf("migration lock already held by another instance")
		}
		defer func() {
			sqlDB.Exec("DELETE FROM migration_lock WHERE id = 1")
		}()
	}
	// SQLite: No locking needed (single instance limitation)

	// Ensure schema_migrations table exists (GORM will create it via AutoMigrate)
	if err := c.DB.AutoMigrate(&SchemaMigration{}); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Get applied migrations
	applied, err := c.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Check if V1 migration needs to be run
	const migrationVersion = "V1"
	if applied[migrationVersion] {
		// Migration already applied, just ensure schema is up to date
		if err := c.DB.AutoMigrate(&Organization{}, &Environment{}, &EnvironmentVariable{}, &APIKey{}); err != nil {
			return fmt.Errorf("failed to run AutoMigrate: %w", err)
		}
		return nil
	}

	// Run initial migration using GORM AutoMigrate
	if err := c.DB.AutoMigrate(&Organization{}, &Environment{}, &EnvironmentVariable{}, &APIKey{}); err != nil {
		return fmt.Errorf("migration %s failed: %w", migrationVersion, err)
	}

	// Record migration
	migration := SchemaMigration{
		Version:   migrationVersion,
		AppliedAt: time.Now(),
	}
	if err := c.DB.Create(&migration).Error; err != nil {
		return fmt.Errorf("failed to record migration %s: %w", migrationVersion, err)
	}

	return nil
}

func (c *Connection) getAppliedMigrations() (map[string]bool, error) {
	var migrations []SchemaMigration
	if err := c.DB.Find(&migrations).Error; err != nil {
		// Table doesn't exist yet, return empty map
		return make(map[string]bool), nil
	}

	applied := make(map[string]bool)
	for _, m := range migrations {
		applied[m.Version] = true
	}
	return applied, nil
}
