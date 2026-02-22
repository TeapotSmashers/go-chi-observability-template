package handlers

import (
	"encoding/json"
	"net/http"
)

// WriteError writes a standardised JSON error response with the request ID.
func WriteError(w http.ResponseWriter, status int, msg, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":      msg,
		"request_id": requestID,
	})
}
