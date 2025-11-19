package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/bcrypt"

	"not-env-backend/internal/api"
	"not-env-backend/internal/crypto"
	"not-env-backend/internal/db"
)

const (
	port = 1212
)

func main() {
	// Validate environment variables
	if err := validateEnvVars(); err != nil {
		log.Fatalf("Environment validation failed: %v", err)
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
	appAdminKey, err := ensureAPPAdminKey(conn.DB, orgID)
	if err != nil {
		log.Fatalf("Failed to ensure APP_ADMIN key: %v", err)
	}
	log.Printf("APP_ADMIN key: %s", appAdminKey)
	log.Println("IMPORTANT: Save this APP_ADMIN key securely. It will not be shown again.")

	// Initialize handlers
	handlers := api.NewHandlers(conn.DB, cryptoService)
	authMiddleware := api.NewAuthMiddleware(conn.DB)

	// Setup routes
	mux := http.NewServeMux()

	// Environment endpoints (APP_ADMIN)
	mux.HandleFunc("POST /environments", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission("APP_ADMIN")(handlers.CreateEnvironment),
	))
	mux.HandleFunc("GET /environments", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission("APP_ADMIN")(handlers.ListEnvironments),
	))
	mux.HandleFunc("DELETE /environments/", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission("APP_ADMIN")(handlers.DeleteEnvironment),
	))

	// Current environment endpoints
	mux.HandleFunc("GET /environment", authMiddleware.RequireAuth(handlers.GetEnvironment))
	mux.HandleFunc("PATCH /environment", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission("ENV_ADMIN")(handlers.UpdateEnvironment),
	))
	mux.HandleFunc("GET /environment/keys", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission("ENV_ADMIN")(handlers.GetEnvironmentKeys),
	))

	// Variable endpoints
	mux.HandleFunc("GET /variables", authMiddleware.RequireAuth(handlers.ListVariables))
	mux.HandleFunc("GET /variables/", authMiddleware.RequireAuth(handlers.GetVariable))
	mux.HandleFunc("PUT /variables/", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission("ENV_ADMIN")(handlers.SetVariable),
	))
	mux.HandleFunc("DELETE /variables/", authMiddleware.RequireAuth(
		authMiddleware.RequirePermission("ENV_ADMIN")(handlers.DeleteVariable),
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
}

func validateEnvVars() error {
	required := []string{"pg_url", "pg_username", "pg_password", "pg_db_name", "NOT_ENV_MASTER_KEY"}
	for _, key := range required {
		if os.Getenv(key) == "" {
			return fmt.Errorf("missing required environment variable: %s", key)
		}
	}
	return nil
}

func ensureDefaultOrganization(db *sql.DB, cryptoService *crypto.Crypto) (int64, error) {
	// Check if organization exists
	var orgID int64
	err := db.QueryRow("SELECT id FROM organizations LIMIT 1").Scan(&orgID)
	if err == nil {
		return orgID, nil
	}
	if err != sql.ErrNoRows {
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

	err = db.QueryRow(`
		INSERT INTO organizations (name, encrypted_data_key, data_key_nonce)
		VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE SET name = organizations.name
		RETURNING id
	`, "default", encryptedDEK, nonce).Scan(&orgID)
	if err != nil {
		// If another instance created it, fetch it
		if err := db.QueryRow("SELECT id FROM organizations WHERE name = $1", "default").Scan(&orgID); err != nil {
			return 0, fmt.Errorf("failed to create or fetch organization: %w", err)
		}
	}

	return orgID, nil
}

func ensureAPPAdminKey(db *sql.DB, orgID int64) (string, error) {
	// Check if APP_ADMIN key exists
	var keyID int64
	var keyHash string
	err := db.QueryRow(`
		SELECT id, key_hash
		FROM api_keys
		WHERE type = 'APP_ADMIN' AND organization_id = $1 AND revoked_at IS NULL
		LIMIT 1
	`, orgID).Scan(&keyID, &keyHash)
	if err == nil {
		// Key exists, but we can't return it since it's hashed
		// For v1, we'll generate a new one only if none exists
		// Actually, we should return an error or indicate it exists
		return "[APP_ADMIN key already exists - check logs from first startup]", nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("failed to check APP_ADMIN key: %w", err)
	}

	// Generate new APP_ADMIN key
	keyBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
		return "", fmt.Errorf("failed to generate key: %w", err)
	}
	apiKey := base64.URLEncoding.EncodeToString(keyBytes)

	// Hash and store
	keyHashBytes, err := bcrypt.GenerateFromPassword([]byte(apiKey), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash key: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO api_keys (key_hash, type, organization_id, environment_id)
		VALUES ($1, 'APP_ADMIN', $2, NULL)
		ON CONFLICT DO NOTHING
	`, string(keyHashBytes), orgID)
	if err != nil {
		// Check if another instance created it
		var existingID int64
		if err2 := db.QueryRow(`
			SELECT id FROM api_keys
			WHERE type = 'APP_ADMIN' AND organization_id = $1 AND revoked_at IS NULL
			LIMIT 1
		`, orgID).Scan(&existingID); err2 == nil {
			return "[APP_ADMIN key already exists - check logs from first startup]", nil
		}
		return "", fmt.Errorf("failed to create APP_ADMIN key: %w", err)
	}

	return apiKey, nil
}

