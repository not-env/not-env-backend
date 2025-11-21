package api

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"not-env-backend/internal/crypto"
)

// Handlers contains all HTTP handlers
// Environment-related handlers are in environment_handlers.go
// Variable-related handlers are in variable_handlers.go
type Handlers struct {
	db     *gorm.DB
	crypto *crypto.Crypto
}

// NewHandlers creates a new handlers instance
func NewHandlers(db *gorm.DB, crypto *crypto.Crypto) *Handlers {
	return &Handlers{db: db, crypto: crypto}
}

// extractPathParam extracts the last path parameter from a URL path after a prefix
// Example: extractPathParam("/variables/DB_HOST", "/variables/") returns "DB_HOST"
func extractPathParam(r *http.Request, prefix string) string {
	return strings.TrimPrefix(r.URL.Path, prefix)
}

// generateAPIKey generates a secure random API key
func generateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

