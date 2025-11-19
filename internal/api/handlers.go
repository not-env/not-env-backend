package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"not-env-backend/internal/crypto"
	"not-env-backend/internal/models"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	db     *sql.DB
	crypto *crypto.Crypto
}

// NewHandlers creates a new handlers instance
func NewHandlers(db *sql.DB, crypto *crypto.Crypto) *Handlers {
	return &Handlers{db: db, crypto: crypto}
}

// CreateEnvironment handles POST /environments
func (h *Handlers) CreateEnvironment(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != "APP_ADMIN" {
		respondError(w, http.StatusForbidden, "APP_ADMIN permission required")
		return
	}

	var req models.CreateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Create environment
	var envID int64
	var createdAt time.Time
	err = h.db.QueryRow(`
		INSERT INTO environments (organization_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`, authCtx.OrganizationID, req.Name, sql.NullString{String: req.Description, Valid: req.Description != ""}).Scan(&envID, &createdAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			respondError(w, http.StatusConflict, "environment with this name already exists")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create environment")
		return
	}

	// Generate ENV_ADMIN and ENV_READ_ONLY keys
	envAdminKey, err := generateAPIKey()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate keys")
		return
	}

	envReadOnlyKey, err := generateAPIKey()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate keys")
		return
	}

	// Hash and store keys
	envAdminHash, _ := bcrypt.GenerateFromPassword([]byte(envAdminKey), bcrypt.DefaultCost)
	envReadOnlyHash, _ := bcrypt.GenerateFromPassword([]byte(envReadOnlyKey), bcrypt.DefaultCost)

	_, err = h.db.Exec(`
		INSERT INTO api_keys (key_hash, type, organization_id, environment_id)
		VALUES ($1, 'ENV_ADMIN', $2, $3), ($4, 'ENV_READ_ONLY', $2, $3)
	`, envAdminHash, authCtx.OrganizationID, envID, envReadOnlyHash)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create API keys")
		return
	}

	// Return response
	resp := models.CreateEnvironmentResponse{
		ID:             envID,
		OrganizationID: authCtx.OrganizationID,
		Name:           req.Name,
		Description:    req.Description,
		CreatedAt:      createdAt.Format(time.RFC3339),
	}
	resp.Keys.EnvAdmin = envAdminKey
	resp.Keys.EnvReadOnly = envReadOnlyKey

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ListEnvironments handles GET /environments
func (h *Handlers) ListEnvironments(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != "APP_ADMIN" {
		respondError(w, http.StatusForbidden, "APP_ADMIN permission required")
		return
	}

	rows, err := h.db.Query(`
		SELECT e.id, e.organization_id, e.name, e.description, e.created_at, e.updated_at
		FROM environments e
		WHERE e.organization_id = $1
		ORDER BY e.created_at DESC
	`, authCtx.OrganizationID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	var environments []models.EnvironmentResponse
	for rows.Next() {
		var env models.EnvironmentResponse
		var desc sql.NullString
		if err := rows.Scan(&env.ID, &env.OrganizationID, &env.Name, &desc, &env.CreatedAt, &env.UpdatedAt); err != nil {
			continue
		}
		if desc.Valid {
			env.Description = desc.String
		}
		environments = append(environments, env)
	}

	resp := models.ListEnvironmentsResponse{Environments: environments}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// DeleteEnvironment handles DELETE /environments/{id}
func (h *Handlers) DeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != "APP_ADMIN" {
		respondError(w, http.StatusForbidden, "APP_ADMIN permission required")
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		respondError(w, http.StatusBadRequest, "invalid environment ID")
		return
	}

	envID, err := strconv.ParseInt(pathParts[len(pathParts)-1], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid environment ID")
		return
	}

	// Verify environment belongs to organization
	var orgID int64
	err = h.db.QueryRow("SELECT organization_id FROM environments WHERE id = $1", envID).Scan(&orgID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondError(w, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	if orgID != authCtx.OrganizationID {
		respondError(w, http.StatusForbidden, "environment not found")
		return
	}

	// Delete environment (cascade will delete variables and keys)
	_, err = h.db.Exec("DELETE FROM environments WHERE id = $1", envID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete environment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetEnvironment handles GET /environment
func (h *Handlers) GetEnvironment(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	if authCtx.EnvironmentID == nil {
		respondError(w, http.StatusBadRequest, "environment context required")
		return
	}

	var env models.EnvironmentResponse
	var desc sql.NullString
	err = h.db.QueryRow(`
		SELECT id, organization_id, name, description, created_at, updated_at
		FROM environments
		WHERE id = $1
	`, *authCtx.EnvironmentID).Scan(&env.ID, &env.OrganizationID, &env.Name, &desc, &env.CreatedAt, &env.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			respondError(w, http.StatusNotFound, "environment not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	if desc.Valid {
		env.Description = desc.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(env)
}

// UpdateEnvironment handles PATCH /environment
func (h *Handlers) UpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != "ENV_ADMIN" {
		respondError(w, http.StatusForbidden, "ENV_ADMIN permission required")
		return
	}

	if authCtx.EnvironmentID == nil {
		respondError(w, http.StatusBadRequest, "environment context required")
		return
	}

	var req models.UpdateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updates := []string{}
	args := []interface{}{}
	argPos := 1

	if req.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argPos))
		args = append(args, *req.Name)
		argPos++
	}

	if req.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argPos))
		args = append(args, *req.Description)
		argPos++
	}

	if len(updates) == 0 {
		respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	updates = append(updates, fmt.Sprintf("updated_at = $%d", argPos))
	args = append(args, time.Now())
	argPos++

	args = append(args, *authCtx.EnvironmentID)

	query := fmt.Sprintf("UPDATE environments SET %s WHERE id = $%d", strings.Join(updates, ", "), argPos)
	_, err = h.db.Exec(query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update environment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetEnvironmentKeys handles GET /environment/keys
func (h *Handlers) GetEnvironmentKeys(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != "ENV_ADMIN" {
		respondError(w, http.StatusForbidden, "ENV_ADMIN permission required")
		return
	}

	if authCtx.EnvironmentID == nil {
		respondError(w, http.StatusBadRequest, "environment context required")
		return
	}

	// Get keys for this environment
	rows, err := h.db.Query(`
		SELECT key_hash, type
		FROM api_keys
		WHERE environment_id = $1 AND revoked_at IS NULL AND type IN ('ENV_ADMIN', 'ENV_READ_ONLY')
	`, *authCtx.EnvironmentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	var envAdminHash, envReadOnlyHash string
	for rows.Next() {
		var hash string
		var ktype string
		if err := rows.Scan(&hash, &ktype); err != nil {
			continue
		}
		if ktype == "ENV_ADMIN" {
			envAdminHash = hash
		} else if ktype == "ENV_READ_ONLY" {
			envReadOnlyHash = hash
		}
	}

	// We can't return the actual keys since we only store hashes
	// This endpoint should return key IDs or indicate keys exist
	// For v1, we'll return a message indicating keys exist
	envAdminMsg := "[key exists - use CLI to view]"
	envReadOnlyMsg := "[key exists - use CLI to view]"
	if envAdminHash == "" {
		envAdminMsg = "[no key found]"
	}
	if envReadOnlyHash == "" {
		envReadOnlyMsg = "[no key found]"
	}

	resp := models.EnvironmentKeysResponse{
		EnvAdmin:    envAdminMsg,
		EnvReadOnly: envReadOnlyMsg,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ListVariables handles GET /variables
func (h *Handlers) ListVariables(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	if authCtx.EnvironmentID == nil {
		respondError(w, http.StatusBadRequest, "environment context required")
		return
	}

	// Get organization DEK
	var encryptedDEK, nonce []byte
	err = h.db.QueryRow(`
		SELECT encrypted_data_key, data_key_nonce
		FROM organizations
		WHERE id = $1
	`, authCtx.OrganizationID).Scan(&encryptedDEK, &nonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	dek, err := h.crypto.DecryptDEK(encryptedDEK, nonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "decryption error")
		return
	}

	// Get variables
	rows, err := h.db.Query(`
		SELECT key, value_encrypted, created_at, updated_at
		FROM environment_variables
		WHERE environment_id = $1
		ORDER BY key
	`, *authCtx.EnvironmentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	var variables []models.VariableResponse
	for rows.Next() {
		var v models.VariableResponse
		var valueEncrypted []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&v.Key, &valueEncrypted, &createdAt, &updatedAt); err != nil {
			continue
		}

		// Decrypt value
		// Note: We need to store nonce with each encrypted value
		// For now, assume nonce is prepended or we need to add a nonce column
		// Let's add a nonce column to the schema - but for now, let's assume it's stored
		// Actually, looking at the encryption code, we return nonce separately
		// We need to store nonce per variable. Let me check the schema...

		// For v1, let's store nonce with each value. We'll need to update the schema.
		// Actually, let me check if we can store it differently. GCM nonce is 12 bytes.
		// We could prepend it to the ciphertext or store separately.
		// Let's store it separately for clarity - but the schema doesn't have it yet.
		// For now, let's assume we'll add a value_nonce column or prepend it.

		// Temporary: assume nonce is prepended (first 12 bytes)
		if len(valueEncrypted) < 12 {
			continue
		}
		valueNonce := valueEncrypted[:12]
		valueCiphertext := valueEncrypted[12:]

		value, err := crypto.DecryptValue(valueCiphertext, valueNonce, dek)
		if err != nil {
			continue
		}

		v.Value = value
		v.CreatedAt = createdAt.Format(time.RFC3339)
		v.UpdatedAt = updatedAt.Format(time.RFC3339)
		variables = append(variables, v)
	}

	resp := models.ListVariablesResponse{Variables: variables}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetVariable handles GET /variables/{key}
func (h *Handlers) GetVariable(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	if authCtx.EnvironmentID == nil {
		respondError(w, http.StatusBadRequest, "environment context required")
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	key := pathParts[len(pathParts)-1]

	// Get organization DEK
	var encryptedDEK, nonce []byte
	err = h.db.QueryRow(`
		SELECT encrypted_data_key, data_key_nonce
		FROM organizations
		WHERE id = $1
	`, authCtx.OrganizationID).Scan(&encryptedDEK, &nonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	dek, err := h.crypto.DecryptDEK(encryptedDEK, nonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "decryption error")
		return
	}

	// Get variable
	var valueEncrypted []byte
	var createdAt, updatedAt time.Time
	err = h.db.QueryRow(`
		SELECT value_encrypted, created_at, updated_at
		FROM environment_variables
		WHERE environment_id = $1 AND key = $2
	`, *authCtx.EnvironmentID, key).Scan(&valueEncrypted, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			respondError(w, http.StatusNotFound, "variable not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Decrypt value (assuming nonce prepended)
	if len(valueEncrypted) < 12 {
		respondError(w, http.StatusInternalServerError, "invalid encrypted value")
		return
	}
	valueNonce := valueEncrypted[:12]
	valueCiphertext := valueEncrypted[12:]

	value, err := crypto.DecryptValue(valueCiphertext, valueNonce, dek)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "decryption error")
		return
	}

	v := models.VariableResponse{
		Key:       key,
		Value:     value,
		CreatedAt: createdAt.Format(time.RFC3339),
		UpdatedAt: updatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// SetVariable handles PUT /variables/{key}
func (h *Handlers) SetVariable(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != "ENV_ADMIN" {
		respondError(w, http.StatusForbidden, "ENV_ADMIN permission required")
		return
	}

	if authCtx.EnvironmentID == nil {
		respondError(w, http.StatusBadRequest, "environment context required")
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	key := pathParts[len(pathParts)-1]

	var req models.SetVariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get organization DEK
	var encryptedDEK, nonce []byte
	err = h.db.QueryRow(`
		SELECT encrypted_data_key, data_key_nonce
		FROM organizations
		WHERE id = $1
	`, authCtx.OrganizationID).Scan(&encryptedDEK, &nonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	dek, err := h.crypto.DecryptDEK(encryptedDEK, nonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "decryption error")
		return
	}

	// Encrypt value
	valueEncrypted, valueNonce, err := crypto.EncryptValue(req.Value, dek)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "encryption error")
		return
	}

	// Store nonce prepended to ciphertext
	valueToStore := append(valueNonce, valueEncrypted...)

	// Insert or update variable
	_, err = h.db.Exec(`
		INSERT INTO environment_variables (environment_id, key, value_encrypted, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (environment_id, key) DO UPDATE
		SET value_encrypted = $3, updated_at = $4
	`, *authCtx.EnvironmentID, key, valueToStore, time.Now())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save variable")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteVariable handles DELETE /variables/{key}
func (h *Handlers) DeleteVariable(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != "ENV_ADMIN" {
		respondError(w, http.StatusForbidden, "ENV_ADMIN permission required")
		return
	}

	if authCtx.EnvironmentID == nil {
		respondError(w, http.StatusBadRequest, "environment context required")
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	key := pathParts[len(pathParts)-1]

	_, err = h.db.Exec(`
		DELETE FROM environment_variables
		WHERE environment_id = $1 AND key = $2
	`, *authCtx.EnvironmentID, key)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete variable")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// generateAPIKey generates a secure random API key
func generateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

