// Package main provides the not-env backend HTTP API server.
//
// The backend is a stateless Go HTTP server that:
//   - Stores environment variables encrypted at rest using AES-256-GCM
//   - Supports SQLite, PostgreSQL, and MySQL/MariaDB databases
//   - Exposes a JSON API on port 1212
//   - Auto-generates master encryption key and APP_ADMIN API key if not provided
//   - Handles graceful shutdown on SIGTERM/SIGINT signals
//
// Main initialization flow:
//   1. Validate environment variables (database type, connection details)
//   2. Initialize crypto service (with auto-generated master key if needed)
//   3. Connect to database and run migrations
//   4. Ensure default organization and APP_ADMIN key exist
//   5. Setup HTTP routes and middleware
//   6. Start server with graceful shutdown handling
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"not-env-backend/internal/api"
	"not-env-backend/internal/crypto"
	"not-env-backend/internal/db"
)

const (
	port = 1212
)

var version = "0.1.0"

func main() {
	// Validate environment variables
	masterKey, err := validateEnvVars()
	if err != nil {
		log.Fatalf("Environment validation failed: %v", err)
	}
	if masterKey != "" {
		log.Println("========================================")
		log.Printf("NOT_ENV_MASTER_KEY was auto-generated:")
		log.Printf("%s", masterKey)
		log.Println("")
		log.Println("IMPORTANT: Save this key securely!")
		log.Println("You'll need it to restart the backend.")
		log.Println("========================================")
	}

	// Initialize crypto
	cryptoService, err := crypto.NewCrypto()
	if err != nil {
		log.Fatalf("Failed to initialize crypto: %v", err)
	}

	// Connect to database
	conn, err := db.NewConnection()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close()

	// Run migrations
	if err := conn.RunMigrations(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Migrations completed successfully")

	// Ensure default organization exists
	orgID, err := ensureDefaultOrganization(conn.DB, cryptoService)
	if err != nil {
		log.Fatalf("Failed to ensure default organization: %v", err)
	}
	log.Printf("Default organization ready (ID: %d)", orgID)

	// Ensure APP_ADMIN key exists
	appAdminKeyEnv := os.Getenv("NOT_ENV_APP_ADMIN_KEY")
	appAdminKey, err := ensureAPPAdminKey(conn.DB, orgID, appAdminKeyEnv)
	if err != nil {
		log.Fatalf("Failed to ensure APP_ADMIN key: %v", err)
	}
	if appAdminKeyEnv != "" && appAdminKey != "[APP_ADMIN key already exists - check logs from first startup]" {
		log.Printf("APP_ADMIN key: %s (from NOT_ENV_APP_ADMIN_KEY)", appAdminKey)
	} else {
		log.Printf("APP_ADMIN key: %s", appAdminKey)
		log.Println("IMPORTANT: Save this APP_ADMIN key securely. It will not be shown again.")
	}

	// Initialize handlers
	handlers := api.NewHandlers(conn.DB, cryptoService)
	authMiddleware := api.NewAuthMiddleware(conn.DB)

	// Setup routes
	mux := http.NewServeMux()

	// Environment endpoints (APP_ADMIN)
	mux.HandleFunc("POST /environments", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission(api.KeyTypeAPPAdmin)(handlers.CreateEnvironment),
	))
	mux.HandleFunc("GET /environments", authMiddleware.RequireAuth(handlers.ListEnvironments))
	mux.HandleFunc("DELETE /environments/", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission(api.KeyTypeAPPAdmin)(handlers.DeleteEnvironment),
	))

	// Current environment endpoints
	mux.HandleFunc("GET /environment", authMiddleware.RequireAuth(handlers.GetEnvironment))
	mux.HandleFunc("PATCH /environment", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission(api.KeyTypeENVAdmin)(handlers.UpdateEnvironment),
	))
	mux.HandleFunc("GET /environment/keys", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission(api.KeyTypeENVAdmin)(handlers.GetEnvironmentKeys),
	))

	// Me endpoint (all authenticated users)
	mux.HandleFunc("GET /me", authMiddleware.RequireAuth(handlers.GetMe))

	// Variable endpoints
	mux.HandleFunc("GET /variables", authMiddleware.RequireAuth(handlers.ListVariables))
	mux.HandleFunc("GET /variables/", authMiddleware.RequireAuth(handlers.GetVariable))
	mux.HandleFunc("PUT /variables/", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission(api.KeyTypeENVAdmin)(handlers.SetVariable),
	))
	mux.HandleFunc("DELETE /variables/", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission(api.KeyTypeENVAdmin)(handlers.DeleteVariable),
	))

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Start server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on port %d", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	<-sigChan
	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server stopped")
}

func validateEnvVars() (string, error) {
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		return "", fmt.Errorf("missing required environment variable: DB_TYPE")
	}

	if dbType == "sqlite" {
		if os.Getenv("DB_PATH") == "" {
			return "", fmt.Errorf("missing required environment variable: DB_PATH")
		}
	} else if dbType == "postgres" || dbType == "mysql" {
		required := []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME"}
		for _, key := range required {
			if os.Getenv(key) == "" {
				return "", fmt.Errorf("missing required environment variable: %s", key)
			}
		}
	} else {
		return "", fmt.Errorf("unsupported DB_TYPE: %s (supported: sqlite, postgres, mysql)", dbType)
	}

	// Auto-generate master key if not provided
	masterKey := os.Getenv("NOT_ENV_MASTER_KEY")
	if masterKey == "" {
		keyBytes := make([]byte, 32)
		if _, err := rand.Read(keyBytes); err != nil {
			return "", fmt.Errorf("failed to generate master key: %w", err)
		}
		masterKey = base64.StdEncoding.EncodeToString(keyBytes)
		os.Setenv("NOT_ENV_MASTER_KEY", masterKey)
		return masterKey, nil
	}

	return "", nil
}

func ensureDefaultOrganization(dbConn *gorm.DB, cryptoService *crypto.Crypto) (int64, error) {
	// Check if organization exists
	var org db.Organization
	err := dbConn.First(&org).Error
	if err == nil {
		return org.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, fmt.Errorf("failed to check organization: %w", err)
	}

	// Create default organization
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return 0, fmt.Errorf("failed to generate DEK: %w", err)
	}

	encryptedDEK, nonce, err := cryptoService.EncryptDEK(dek)
	if err != nil {
		return 0, fmt.Errorf("failed to encrypt DEK: %w", err)
	}

	org = db.Organization{
		Name:            "default",
		EncryptedDataKey: encryptedDEK,
		DataKeyNonce:    nonce,
	}

	if err := dbConn.Where("name = ?", "default").FirstOrCreate(&org).Error; err != nil {
		return 0, fmt.Errorf("failed to create or fetch organization: %w", err)
	}

	return org.ID, nil
}

func ensureAPPAdminKey(dbConn *gorm.DB, orgID int64, providedKey string) (string, error) {
	// Check if APP_ADMIN key exists
	var existingKey db.APIKey
	err := dbConn.Where("type = ? AND organization_id = ? AND revoked_at IS NULL", api.KeyTypeAPPAdmin, orgID).
		First(&existingKey).Error
	if err == nil {
		// Key exists, but we can't return it since it's hashed
		return "[APP_ADMIN key already exists - check logs from first startup]", nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("failed to check APP_ADMIN key: %w", err)
	}

	var apiKey string
	if providedKey != "" {
		// Use provided key
		apiKey = providedKey
	} else {
		// Generate new APP_ADMIN key
		keyBytes := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
			return "", fmt.Errorf("failed to generate key: %w", err)
		}
		apiKey = base64.URLEncoding.EncodeToString(keyBytes)
	}

	// Hash and store
	keyHashBytes, err := bcrypt.GenerateFromPassword([]byte(apiKey), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash key: %w", err)
	}

	newKey := db.APIKey{
		KeyHash:       string(keyHashBytes),
		Type:          api.KeyTypeAPPAdmin,
		OrganizationID: orgID,
		EnvironmentID:  sql.NullInt64{Valid: false},
	}

	if err := dbConn.Create(&newKey).Error; err != nil {
		// Check if another instance created it
		var existingKey2 db.APIKey
		if err2 := dbConn.Where("type = ? AND organization_id = ? AND revoked_at IS NULL", "APP_ADMIN", orgID).
			First(&existingKey2).Error; err2 == nil {
			return "[APP_ADMIN key already exists - check logs from first startup]", nil
		}
		return "", fmt.Errorf("failed to create APP_ADMIN key: %w", err)
	}

	return apiKey, nil
}

