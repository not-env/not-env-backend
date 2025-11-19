package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"not-env-backend/internal/models"
)

// AuthMiddleware validates API keys and attaches auth context to requests
type AuthMiddleware struct {
	db *sql.DB
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(db *sql.DB) *AuthMiddleware {
	return &AuthMiddleware{db: db}
}

// RequireAuth wraps a handler with authentication
func (m *AuthMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondError(w, http.StatusUnauthorized, "missing Authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			respondError(w, http.StatusUnauthorized, "invalid Authorization header format")
			return
		}

		apiKey := parts[1]
		if apiKey == "" {
			respondError(w, http.StatusUnauthorized, "missing API key")
			return
		}

		// Check all keys to find a match (we need to check all because we store hashes)
		rows, err := m.db.Query(`
			SELECT id, key_hash, type, organization_id, environment_id
			FROM api_keys
			WHERE revoked_at IS NULL
		`)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		var authCtx *models.AuthContext
		for rows.Next() {
			var id int64
			var hash string
			var ktype string
			var org int64
			var env sql.NullInt64

			if err := rows.Scan(&id, &hash, &ktype, &org, &env); err != nil {
				continue
			}

			// Compare API key with hash
			if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(apiKey)); err == nil {
				authCtx = &models.AuthContext{
					KeyType:        ktype,
					OrganizationID: org,
					APIKeyID:       id,
				}
				if env.Valid {
					envID := env.Int64
					authCtx.EnvironmentID = &envID
				}
				break
			}
		}

		if authCtx == nil {
			respondError(w, http.StatusUnauthorized, "invalid API key")
			return
		}

		// Attach auth context to request
		ctx := context.WithValue(r.Context(), "auth", authCtx)
		next(w, r.WithContext(ctx))
	}
}

// RequirePermission wraps a handler with permission checking
func (m *AuthMiddleware) RequirePermission(requiredTypes ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authCtx, ok := r.Context().Value("auth").(*models.AuthContext)
			if !ok || authCtx == nil {
				respondError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			allowed := false
			for _, t := range requiredTypes {
				if authCtx.KeyType == t {
					allowed = true
					break
				}
			}

			if !allowed {
				respondError(w, http.StatusForbidden, fmt.Sprintf("insufficient permissions: requires %v", requiredTypes))
				return
			}

			next(w, r)
		}
	}
}

// GetAuthContext extracts auth context from request
func GetAuthContext(r *http.Request) (*models.AuthContext, error) {
	authCtx, ok := r.Context().Value("auth").(*models.AuthContext)
	if !ok || authCtx == nil {
		return nil, fmt.Errorf("authentication context not found")
	}
	return authCtx, nil
}

// respondError sends an error response
func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(models.ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}

