package db

import (
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Connection wraps database connection and provides methods for operations
type Connection struct {
	DB *gorm.DB
}

// NewConnection creates a new database connection using GORM
func NewConnection() (*Connection, error) {
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		return nil, fmt.Errorf("DB_TYPE environment variable is required (sqlite, postgres, mysql)")
	}

	var db *gorm.DB
	var err error

	switch dbType {
	case "sqlite":
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			return nil, fmt.Errorf("DB_PATH environment variable is required for SQLite")
		}

		// Create directory if it doesn't exist
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}

		// Open SQLite database (creates file if doesn't exist)
		db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
		if err != nil {
			return nil, fmt.Errorf("failed to connect to SQLite database: %w", err)
		}

	case "postgres":
		dbHost := os.Getenv("DB_HOST")
		dbPort := os.Getenv("DB_PORT")
		dbUser := os.Getenv("DB_USER")
		dbPassword := os.Getenv("DB_PASSWORD")
		dbName := os.Getenv("DB_NAME")

		if dbHost == "" || dbPort == "" || dbUser == "" || dbPassword == "" || dbName == "" {
			return nil, fmt.Errorf("missing required PostgreSQL environment variables (DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME)")
		}

		// Build DSN for PostgreSQL
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			dbHost, dbPort, dbUser, dbPassword, dbName)

		// Try to connect to target database
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			// If database doesn't exist, try to create it
			adminDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
				dbHost, dbPort, dbUser, dbPassword)
			adminDB, adminErr := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
			if adminErr != nil {
				return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
			}

			// Create database
			sqlDB, _ := adminDB.DB()
			createDB := fmt.Sprintf("CREATE DATABASE %s", dbName)
			if _, err := sqlDB.Exec(createDB); err != nil {
				// If database already exists (race condition), that's fine
				if !isDBExistsError(err) {
					return nil, fmt.Errorf("failed to create database: %w", err)
				}
			}
			sqlDB.Close()

			// Now connect to the target database
			db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
			if err != nil {
				return nil, fmt.Errorf("failed to connect to database: %w", err)
			}
		}

	case "mysql":
		dbHost := os.Getenv("DB_HOST")
		dbPort := os.Getenv("DB_PORT")
		dbUser := os.Getenv("DB_USER")
		dbPassword := os.Getenv("DB_PASSWORD")
		dbName := os.Getenv("DB_NAME")

		if dbHost == "" || dbPort == "" || dbUser == "" || dbPassword == "" || dbName == "" {
			return nil, fmt.Errorf("missing required MySQL environment variables (DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME)")
		}

		// Build DSN for MySQL
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			dbUser, dbPassword, dbHost, dbPort, dbName)

		// Try to connect to target database
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err != nil {
			// If database doesn't exist, try to create it
			adminDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/?charset=utf8mb4&parseTime=True&loc=Local",
				dbUser, dbPassword, dbHost, dbPort)
			adminDB, adminErr := gorm.Open(mysql.Open(adminDSN), &gorm.Config{})
			if adminErr != nil {
				return nil, fmt.Errorf("failed to connect to MySQL: %w", err)
			}

			// Create database
			sqlDB, _ := adminDB.DB()
			createDB := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", dbName)
			if _, err := sqlDB.Exec(createDB); err != nil {
				return nil, fmt.Errorf("failed to create database: %w", err)
			}
			sqlDB.Close()

			// Now connect to the target database
			db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
			if err != nil {
				return nil, fmt.Errorf("failed to connect to database: %w", err)
			}
		}

	default:
		return nil, fmt.Errorf("unsupported DB_TYPE: %s (supported: sqlite, postgres, mysql)", dbType)
	}

	// Test connection
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Connection{DB: db}, nil
}

// isDBExistsError checks if error is due to database already existing
func isDBExistsError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "already exists") || contains(errStr, "duplicate") || contains(errStr, "already exist")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Close closes the database connection
func (c *Connection) Close() error {
	sqlDB, err := c.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

