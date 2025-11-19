package models

import "time"

// AuthContext contains authentication information attached to requests
type AuthContext struct {
	KeyType        string
	OrganizationID int64
	EnvironmentID  *int64 // nil for APP_ADMIN
	APIKeyID       int64
}

// CreateEnvironmentRequest represents a request to create an environment
type CreateEnvironmentRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateEnvironmentResponse represents the response when creating an environment
type CreateEnvironmentResponse struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	CreatedAt      string `json:"created_at"`
	Keys           struct {
		EnvAdmin    string `json:"env_admin"`
		EnvReadOnly string `json:"env_read_only"`
	} `json:"keys"`
}

// EnvironmentResponse represents an environment in API responses
type EnvironmentResponse struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	Keys           struct {
		EnvAdmin    string `json:"env_admin,omitempty"`
		EnvReadOnly string `json:"env_read_only,omitempty"`
	} `json:"keys,omitempty"`
}

// ListEnvironmentsResponse represents the response when listing environments
type ListEnvironmentsResponse struct {
	Environments []EnvironmentResponse `json:"environments"`
}

// UpdateEnvironmentRequest represents a request to update an environment
type UpdateEnvironmentRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// EnvironmentKeysResponse represents the API keys for an environment
type EnvironmentKeysResponse struct {
	EnvAdmin    string `json:"env_admin"`
	EnvReadOnly string `json:"env_read_only"`
}

// VariableResponse represents a single environment variable
type VariableResponse struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListVariablesResponse represents the response when listing variables
type ListVariablesResponse struct {
	Variables []VariableResponse `json:"variables"`
}

// SetVariableRequest represents a request to set a variable
type SetVariableRequest struct {
	Value string `json:"value"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// APIKeyInfo represents API key information (without the actual key)
type APIKeyInfo struct {
	ID            int64     `json:"id"`
	Type          string    `json:"type"`
	OrganizationID int64    `json:"organization_id"`
	EnvironmentID *int64   `json:"environment_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

