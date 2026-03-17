package test

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/httputil"
)

func TestServerErrorWritesCorrelationID(t *testing.T) {
	rr := httptest.NewRecorder()

	// Capture structured log output.
	var logBuf bytes.Buffer
	origHandler := slog.Default().Handler()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
	defer slog.SetDefault(slog.New(origHandler))

	httputil.ServerError(rr, "test failure", errors.New("db connection lost"))

	// Response should be 500 with correlation ID.
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Internal error [ref: ") {
		t.Errorf("expected correlation ID in body, got: %s", body)
	}

	// Extract the ref from the body.
	start := strings.Index(body, "[ref: ") + len("[ref: ")
	end := strings.Index(body[start:], "]")
	ref := body[start : start+end]
	if len(ref) != 8 {
		t.Errorf("expected 8-char correlation ID, got %q (len %d)", ref, len(ref))
	}

	// Log should contain the correlation ID and original error.
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, ref) {
		t.Errorf("expected correlation ID %q in log, got: %s", ref, logOutput)
	}
	if !strings.Contains(logOutput, "db connection lost") {
		t.Errorf("expected original error in log, got: %s", logOutput)
	}
}

func TestServerErrorWithExtraArgs(t *testing.T) {
	rr := httptest.NewRecorder()

	var logBuf bytes.Buffer
	origHandler := slog.Default().Handler()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
	defer slog.SetDefault(slog.New(origHandler))

	httputil.ServerError(rr, "item op failed", errors.New("io error"), "slug", "test-item")

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "slug") || !strings.Contains(logOutput, "test-item") {
		t.Errorf("expected extra args in log, got: %s", logOutput)
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("item not found"), true},
		{errors.New("slug not found in file"), true},
		{errors.New("failed to write file"), false},
		{nil, false},
	}
	for _, tt := range tests {
		got := httputil.IsNotFound(tt.err)
		if got != tt.want {
			t.Errorf("IsNotFound(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
