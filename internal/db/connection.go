package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

// Connection wraps database connection and provides methods for operations
type Connection struct {
	DB *sql.DB
}

// NewConnection creates a new database connection
func NewConnection() (*Connection, error) {
	pgURL := os.Getenv("pg_url")
	pgUsername := os.Getenv("pg_username")
	pgPassword := os.Getenv("pg_password")
	pgDBName := os.Getenv("pg_db_name")

	if pgURL == "" || pgUsername == "" || pgPassword == "" || pgDBName == "" {
		return nil, fmt.Errorf("missing required database environment variables")
	}

	// Connect to postgres database first to check/create target database
	connStr := fmt.Sprintf("host=%s user=%s password=%s dbname=postgres sslmode=disable", pgURL, pgUsername, pgPassword)
	adminDB, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres database: %w", err)
	}
	defer adminDB.Close()

	// Check if database exists, create if not
	var exists bool
	err = adminDB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", pgDBName).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check database existence: %w", err)
	}

	if !exists {
		_, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", pgDBName))
		if err != nil {
			// If database already exists (race condition), that's fine
			if !isDBExistsError(err) {
				return nil, fmt.Errorf("failed to create database: %w", err)
			}
		}
	}

	// Now connect to the target database
	connStr = fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable", pgURL, pgUsername, pgPassword, pgDBName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
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
	return contains(errStr, "already exists") || contains(errStr, "duplicate")
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
	return c.DB.Close()
}

