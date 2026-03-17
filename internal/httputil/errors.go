package httputil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// ServerError writes a 500 response with a correlation ID and logs the full
// error. The correlation ID appears in both the response body and the
// structured log entry so support requests can be matched to logs.
func ServerError(w http.ResponseWriter, msg string, err error, slogArgs ...any) {
	id := correlationID()
	args := append([]any{"error", err, "correlation_id", id}, slogArgs...)
	slog.Error(msg, args...)
	http.Error(w, fmt.Sprintf("Internal error [ref: %s]", id), http.StatusInternalServerError)
}

// IsNotFound returns true if the error message contains "not found".
// TODO: replace with sentinel errors (errors.Is) per service package.
func IsNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

func correlationID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}
