package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"not-env-backend/internal/db"
	"not-env-backend/internal/models"
)

// CreateEnvironment handles POST /environments
func (h *Handlers) CreateEnvironment(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != KeyTypeAPPAdmin {
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
	env := db.Environment{
		OrganizationID: authCtx.OrganizationID,
		Name:           req.Name,
	}
	if req.Description != "" {
		env.Description = sql.NullString{String: req.Description, Valid: true}
	}

	if err := h.db.Create(&env).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			respondError(w, http.StatusConflict, "environment with this name already exists")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create environment")
		return
	}

	// Generate ENV_ADMIN and ENV_READ_ONLY keys
	// Error handling: If key generation fails, return error immediately
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
	// Error handling: bcrypt hashing can fail (rarely), so we check for errors
	// API keys are hashed before storage for security (we can't retrieve plaintext keys)
	envAdminHash, err := bcrypt.GenerateFromPassword([]byte(envAdminKey), bcrypt.DefaultCost)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash API key")
		return
	}
	envReadOnlyHash, err := bcrypt.GenerateFromPassword([]byte(envReadOnlyKey), bcrypt.DefaultCost)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash API key")
		return
	}

	envAdminAPIKey := db.APIKey{
		KeyHash:       string(envAdminHash),
		Type:          KeyTypeENVAdmin,
		OrganizationID: authCtx.OrganizationID,
		EnvironmentID:  sql.NullInt64{Int64: env.ID, Valid: true},
	}

	envReadOnlyAPIKey := db.APIKey{
		KeyHash:       string(envReadOnlyHash),
		Type:          KeyTypeENVReadOnly,
		OrganizationID: authCtx.OrganizationID,
		EnvironmentID:  sql.NullInt64{Int64: env.ID, Valid: true},
	}

	if err := h.db.Create(&envAdminAPIKey).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create API keys")
		return
	}

	if err := h.db.Create(&envReadOnlyAPIKey).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create API keys")
		return
	}

	// Return response
	resp := models.CreateEnvironmentResponse{
		ID:             env.ID,
		OrganizationID: authCtx.OrganizationID,
		Name:           req.Name,
		Description:    req.Description,
		CreatedAt:      env.CreatedAt.Format(time.RFC3339),
	}
	resp.Keys.EnvAdmin = envAdminKey
	resp.Keys.EnvReadOnly = envReadOnlyKey

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ListEnvironments handles GET /environments
func (h *Handlers) ListEnvironments(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// If ENV_ADMIN or ENV_READ_ONLY, return only their environment
	if authCtx.KeyType == KeyTypeENVAdmin || authCtx.KeyType == KeyTypeENVReadOnly {
		if authCtx.EnvironmentID == nil {
			respondError(w, http.StatusBadRequest, "environment context required")
			return
		}

		var dbEnv db.Environment
		if err := h.db.Where("id = ?", *authCtx.EnvironmentID).First(&dbEnv).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondError(w, http.StatusNotFound, fmt.Sprintf("environment with ID %d not found", *authCtx.EnvironmentID))
				return
			}
			respondError(w, http.StatusInternalServerError, "database error")
			return
		}

		env := models.EnvironmentResponse{
			ID:             dbEnv.ID,
			OrganizationID: dbEnv.OrganizationID,
			Name:           dbEnv.Name,
			CreatedAt:      dbEnv.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      dbEnv.UpdatedAt.Format(time.RFC3339),
		}
		if dbEnv.Description.Valid {
			env.Description = dbEnv.Description.String
		}

		resp := models.ListEnvironmentsResponse{Environments: []models.EnvironmentResponse{env}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// APP_ADMIN: return all environments
	if authCtx.KeyType != KeyTypeAPPAdmin {
		respondError(w, http.StatusForbidden, "APP_ADMIN permission required")
		return
	}

	var dbEnvs []db.Environment
	if err := h.db.Where("organization_id = ?", authCtx.OrganizationID).
		Order("created_at DESC").
		Find(&dbEnvs).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	var environments []models.EnvironmentResponse
	for _, dbEnv := range dbEnvs {
		env := models.EnvironmentResponse{
			ID:             dbEnv.ID,
			OrganizationID: dbEnv.OrganizationID,
			Name:           dbEnv.Name,
			CreatedAt:      dbEnv.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      dbEnv.UpdatedAt.Format(time.RFC3339),
		}
		if dbEnv.Description.Valid {
			env.Description = dbEnv.Description.String
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
	if err != nil || authCtx.KeyType != KeyTypeAPPAdmin {
		respondError(w, http.StatusForbidden, "APP_ADMIN permission required")
		return
	}

	envIDStr := extractPathParam(r, "/environments/")
	if envIDStr == "" {
		respondError(w, http.StatusBadRequest, "invalid environment ID")
		return
	}

	envID, err := strconv.ParseInt(envIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid environment ID")
		return
	}

	// Verify environment belongs to organization and delete
	var env db.Environment
	if err := h.db.Where("id = ? AND organization_id = ?", envID, authCtx.OrganizationID).
		First(&env).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(w, http.StatusNotFound, fmt.Sprintf("environment with ID %d not found", envID))
			return
		}
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Delete environment (cascade will delete variables and keys)
	if err := h.db.Delete(&env).Error; err != nil {
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

	var dbEnv db.Environment
	if err := h.db.Where("id = ?", *authCtx.EnvironmentID).First(&dbEnv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(w, http.StatusNotFound, fmt.Sprintf("environment with ID %d not found", *authCtx.EnvironmentID))
			return
		}
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	env := models.EnvironmentResponse{
		ID:             dbEnv.ID,
		OrganizationID: dbEnv.OrganizationID,
		Name:           dbEnv.Name,
		CreatedAt:      dbEnv.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      dbEnv.UpdatedAt.Format(time.RFC3339),
	}
	if dbEnv.Description.Valid {
		env.Description = dbEnv.Description.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(env)
}

// UpdateEnvironment handles PATCH /environment
func (h *Handlers) UpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != KeyTypeENVAdmin {
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

	var dbEnv db.Environment
	if err := h.db.Where("id = ?", *authCtx.EnvironmentID).First(&dbEnv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(w, http.StatusNotFound, fmt.Sprintf("environment with ID %d not found", *authCtx.EnvironmentID))
			return
		}
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = sql.NullString{String: *req.Description, Valid: true}
	}

	if len(updates) == 0 {
		respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	updates["updated_at"] = time.Now()

	if err := h.db.Model(&dbEnv).Updates(updates).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update environment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetEnvironmentKeys handles GET /environment/keys
func (h *Handlers) GetEnvironmentKeys(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != KeyTypeENVAdmin {
		respondError(w, http.StatusForbidden, "ENV_ADMIN permission required")
		return
	}

	if authCtx.EnvironmentID == nil {
		respondError(w, http.StatusBadRequest, "environment context required")
		return
	}

	// Get keys for this environment
	var apiKeys []db.APIKey
	if err := h.db.Where("environment_id = ? AND revoked_at IS NULL AND type IN (?)", *authCtx.EnvironmentID, []string{KeyTypeENVAdmin, KeyTypeENVReadOnly}).
		Find(&apiKeys).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	var envAdminHash, envReadOnlyHash string
	for _, key := range apiKeys {
		if key.Type == KeyTypeENVAdmin {
			envAdminHash = key.KeyHash
		} else if key.Type == KeyTypeENVReadOnly {
			envReadOnlyHash = key.KeyHash
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

// GetMe handles GET /me - returns current API key information
func (h *Handlers) GetMe(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	resp := map[string]interface{}{
		"key_type":        authCtx.KeyType,
		"organization_id": authCtx.OrganizationID,
	}
	if authCtx.EnvironmentID != nil {
		resp["environment_id"] = *authCtx.EnvironmentID
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

