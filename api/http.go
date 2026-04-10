package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/travisbale/knowhere/identity"
)

// ErrorResponse represents a generic error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// RespondJSON sends JSON response with given status code
func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.ErrorContext(context.Background(), "Failed to encode JSON response", "error", err)
	}
}

// RespondError sends an error response with structured logging
func RespondError(w http.ResponseWriter, status int, message string, err error) {
	if err != nil {
		slog.ErrorContext(context.Background(), "API error", "message", message, "error", err, "status", status)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if encodeErr := json.NewEncoder(w).Encode(ErrorResponse{Error: message}); encodeErr != nil {
		slog.ErrorContext(context.Background(), "Failed to encode error response", "error", encodeErr)
	}
}

// DecodeJSON decodes JSON from request body, rejects unknown fields
func DecodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	defer r.Body.Close() //nolint:errcheck

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Catch typos in client requests

	if err := decoder.Decode(v); err != nil {
		return err
	}

	return nil
}

// Validator is an interface for types that can validate themselves
type Validator interface {
	Validate() error
}

// DecodeAndValidateJSON decodes and validates JSON, returns false if error response was sent
func DecodeAndValidateJSON(w http.ResponseWriter, r *http.Request, req Validator) bool {
	if err := DecodeJSON(r, req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid request body", nil)
		return false
	}

	if err := req.Validate(); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error(), nil)
		return false
	}

	return true
}

// ParseUUID parses UUID from string, returns uuid.Nil on invalid input
func ParseUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// ParseDate parses a date string in YYYY-MM-DD format
func ParseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}

// GetAuthenticatedActorID extracts the actor ID from context, returns false if unauthorized
func GetAuthenticatedActorID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	actorID, err := identity.GetActor(r.Context())
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return uuid.Nil, false
	}
	return actorID, true
}
