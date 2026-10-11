package httputil

import (
	"encoding/json"
	"net/http"
)

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// APIError is the body of every API error response.
type APIError struct {
	Error string `json:"error"`
}

// JSONError writes an API error response.
func JSONError(w http.ResponseWriter, msg string, status int) {
	WriteJSON(w, status, APIError{Error: msg})
}
