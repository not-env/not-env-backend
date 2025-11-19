package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"
	_ "github.com/mattn/go-sqlite3"

	"not-env-backend/internal/models"
)

func TestRequireAuth(t *testing.T) {
	// Create a test handler that checks auth context
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCtx, err := GetAuthContext(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(authCtx)
	})

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		setupDB        func() *sql.DB
	}{
		{
			name:           "missing Authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid Authorization format",
			authHeader:     "InvalidFormat",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "missing Bearer token",
			authHeader:     "Bearer",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "empty API key",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid API key",
			authHeader:     "Bearer invalid-key",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an in-memory SQLite database for testing
			db, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Skipf("SQLite not available: %v", err)
			}
			defer db.Close()

			// Setup schema
			_, err = db.Exec(`
				CREATE TABLE api_keys (
					id INTEGER PRIMARY KEY,
					key_hash TEXT NOT NULL,
					type TEXT NOT NULL,
					organization_id INTEGER NOT NULL,
					environment_id INTEGER,
					revoked_at DATETIME
				)
			`)
			if err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			// For tests that need a valid key, add one
			if tt.name == "invalid API key" {
				// Add a test key to the database
				testKey := "test-api-key-123"
				keyHash, _ := bcrypt.GenerateFromPassword([]byte(testKey), bcrypt.DefaultCost)
				_, err = db.Exec(`
					INSERT INTO api_keys (key_hash, type, organization_id, environment_id)
					VALUES (?, ?, ?, ?)
				`, string(keyHash), "APP_ADMIN", 1, nil)
				if err != nil {
					t.Fatalf("failed to insert test key: %v", err)
				}
			}

			// Create middleware
			middleware := NewAuthMiddleware(db)
			handler := middleware.RequireAuth(testHandler)

			// Create request
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Execute
			handler.ServeHTTP(rr, req)

			// Check status
			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestRequirePermission(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	tests := []struct {
		name           string
		authCtx        *models.AuthContext
		requiredTypes  []string
		expectedStatus int
	}{
		{
			name:           "no auth context",
			authCtx:        nil,
			requiredTypes:  []string{"APP_ADMIN"},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "correct permission",
			authCtx: &models.AuthContext{
				KeyType: "APP_ADMIN",
			},
			requiredTypes:  []string{"APP_ADMIN"},
			expectedStatus: http.StatusOK,
		},
		{
			name: "wrong permission",
			authCtx: &models.AuthContext{
				KeyType: "ENV_READ_ONLY",
			},
			requiredTypes:  []string{"APP_ADMIN"},
			expectedStatus: http.StatusForbidden,
		},
		{
			name: "multiple required types - match",
			authCtx: &models.AuthContext{
				KeyType: "ENV_ADMIN",
			},
			requiredTypes:  []string{"ENV_ADMIN", "ENV_READ_ONLY"},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Skipf("SQLite not available: %v", err)
			}
			defer db.Close()

			middleware := NewAuthMiddleware(db)
			permissionCheck := middleware.RequirePermission(tt.requiredTypes...)
			handler := permissionCheck(testHandler)

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authCtx != nil {
				ctx := context.WithValue(req.Context(), "auth", tt.authCtx)
				req = req.WithContext(ctx)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestGetAuthContext(t *testing.T) {
	tests := []struct {
		name    string
		authCtx *models.AuthContext
		wantErr bool
	}{
		{
			name:    "valid auth context",
			authCtx: &models.AuthContext{KeyType: "APP_ADMIN", OrganizationID: 1},
			wantErr: false,
		},
		{
			name:    "no auth context",
			authCtx: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authCtx != nil {
				ctx := context.WithValue(req.Context(), "auth", tt.authCtx)
				req = req.WithContext(ctx)
			}

			got, err := GetAuthContext(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAuthContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.KeyType != tt.authCtx.KeyType {
				t.Errorf("GetAuthContext() = %v, want %v", got, tt.authCtx)
			}
		})
	}
}

func TestRespondError(t *testing.T) {
	rr := httptest.NewRecorder()
	respondError(rr, http.StatusBadRequest, "test error")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var errResp models.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.Message != "test error" {
		t.Errorf("expected message 'test error', got %q", errResp.Message)
	}
}

