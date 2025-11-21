package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"

	"not-env-backend/internal/crypto"
	"not-env-backend/internal/db"
	"not-env-backend/internal/models"
)

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

	// Encryption/Decryption Flow:
	// 1. Each organization has a Data Encryption Key (DEK) encrypted with the master key
	// 2. Variable values are encrypted with the organization's DEK using AES-GCM-256
	// 3. The nonce (12 bytes) is prepended to the ciphertext for storage
	// 4. To decrypt: extract nonce, decrypt DEK with master key, decrypt value with DEK

	// Get organization DEK (encrypted with master key)
	var org db.Organization
	if err := h.db.Where("id = ?", authCtx.OrganizationID).First(&org).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Decrypt the organization's DEK using the master key
	dek, err := h.crypto.DecryptDEK(org.EncryptedDataKey, org.DataKeyNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "decryption error")
		return
	}

	// Get variables from database (stored encrypted)
	var dbVars []db.EnvironmentVariable
	if err := h.db.Where("environment_id = ?", *authCtx.EnvironmentID).
		Order("key").
		Find(&dbVars).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	var variables []models.VariableResponse
	for _, dbVar := range dbVars {
		// Extract nonce (first NonceSize bytes) and ciphertext from stored value
		if len(dbVar.ValueEncrypted) < crypto.NonceSize {
			continue // Skip invalid entries
		}
		valueNonce := dbVar.ValueEncrypted[:crypto.NonceSize]
		valueCiphertext := dbVar.ValueEncrypted[crypto.NonceSize:]

		// Decrypt value using organization's DEK
		value, err := crypto.DecryptValue(valueCiphertext, valueNonce, dek)
		if err != nil {
			continue // Skip entries that can't be decrypted (corrupted data)
		}

		v := models.VariableResponse{
			Key:       dbVar.Key,
			Value:     value,
			CreatedAt: dbVar.CreatedAt.Format(time.RFC3339),
			UpdatedAt: dbVar.UpdatedAt.Format(time.RFC3339),
		}
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

	key := extractPathParam(r, "/variables/")
	if key == "" {
		respondError(w, http.StatusBadRequest, "invalid variable key")
		return
	}

	// Get organization DEK (encrypted with master key)
	var org db.Organization
	if err := h.db.Where("id = ?", authCtx.OrganizationID).First(&org).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Decrypt the organization's DEK using the master key
	dek, err := h.crypto.DecryptDEK(org.EncryptedDataKey, org.DataKeyNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "decryption error")
		return
	}

	// Get variable from database (stored encrypted)
	var dbVar db.EnvironmentVariable
	if err := h.db.Where("environment_id = ? AND key = ?", *authCtx.EnvironmentID, key).
		First(&dbVar).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(w, http.StatusNotFound, fmt.Sprintf("variable '%s' not found in environment with ID %d", key, *authCtx.EnvironmentID))
			return
		}
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Extract nonce (first NonceSize bytes) and ciphertext from stored value
	if len(dbVar.ValueEncrypted) < crypto.NonceSize {
		respondError(w, http.StatusInternalServerError, "invalid encrypted value")
		return
	}
	valueNonce := dbVar.ValueEncrypted[:crypto.NonceSize]
	valueCiphertext := dbVar.ValueEncrypted[crypto.NonceSize:]

	// Decrypt value using organization's DEK
	value, err := crypto.DecryptValue(valueCiphertext, valueNonce, dek)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "decryption error")
		return
	}

	v := models.VariableResponse{
		Key:       key,
		Value:     value,
		CreatedAt: dbVar.CreatedAt.Format(time.RFC3339),
		UpdatedAt: dbVar.UpdatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// SetVariable handles PUT /variables/{key}
func (h *Handlers) SetVariable(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != KeyTypeENVAdmin {
		respondError(w, http.StatusForbidden, "ENV_ADMIN permission required")
		return
	}

	if authCtx.EnvironmentID == nil {
		respondError(w, http.StatusBadRequest, "environment context required")
		return
	}

	key := extractPathParam(r, "/variables/")
	if key == "" {
		respondError(w, http.StatusBadRequest, "invalid variable key")
		return
	}

	var req models.SetVariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get organization DEK (encrypted with master key)
	var org db.Organization
	if err := h.db.Where("id = ?", authCtx.OrganizationID).First(&org).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Decrypt the organization's DEK using the master key
	dek, err := h.crypto.DecryptDEK(org.EncryptedDataKey, org.DataKeyNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "decryption error")
		return
	}

	// Encrypt value using organization's DEK (AES-GCM-256)
	// Returns ciphertext and nonce (12 bytes)
	valueEncrypted, valueNonce, err := crypto.EncryptValue(req.Value, dek)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "encryption error")
		return
	}

	// Store nonce prepended to ciphertext for easy retrieval during decryption
	valueToStore := append(valueNonce, valueEncrypted...)

	// Insert or update variable
	var dbVar db.EnvironmentVariable
	if err := h.db.Where("environment_id = ? AND key = ?", *authCtx.EnvironmentID, key).
		First(&dbVar).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new variable
			dbVar = db.EnvironmentVariable{
				EnvironmentID:  *authCtx.EnvironmentID,
				Key:            key,
				ValueEncrypted: valueToStore,
			}
			if err := h.db.Create(&dbVar).Error; err != nil {
				respondError(w, http.StatusInternalServerError, "failed to save variable")
				return
			}
		} else {
			respondError(w, http.StatusInternalServerError, "database error")
			return
		}
	} else {
		// Update existing variable
		dbVar.ValueEncrypted = valueToStore
		dbVar.UpdatedAt = time.Now()
		if err := h.db.Save(&dbVar).Error; err != nil {
			respondError(w, http.StatusInternalServerError, "failed to save variable")
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteVariable handles DELETE /variables/{key}
func (h *Handlers) DeleteVariable(w http.ResponseWriter, r *http.Request) {
	authCtx, err := GetAuthContext(r)
	if err != nil || authCtx.KeyType != KeyTypeENVAdmin {
		respondError(w, http.StatusForbidden, "ENV_ADMIN permission required")
		return
	}

	if authCtx.EnvironmentID == nil {
		respondError(w, http.StatusBadRequest, "environment context required")
		return
	}

	key := extractPathParam(r, "/variables/")
	if key == "" {
		respondError(w, http.StatusBadRequest, "invalid variable key")
		return
	}

	var dbVar db.EnvironmentVariable
	if err := h.db.Where("environment_id = ? AND key = ?", *authCtx.EnvironmentID, key).
		First(&dbVar).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(w, http.StatusNotFound, fmt.Sprintf("variable '%s' not found in environment with ID %d", key, *authCtx.EnvironmentID))
			return
		}
		respondError(w, http.StatusInternalServerError, "database error")
		return
	}

	if err := h.db.Delete(&dbVar).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete variable")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

