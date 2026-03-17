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

func TestSplitImageCaption(t *testing.T) {
	tests := []struct {
		entry       string
		wantFile    string
		wantCaption string
	}{
		{"abc.png", "abc.png", ""},
		{"abc.png|My caption", "abc.png", "My caption"},
		{"abc.png|Caption with|extra pipes", "abc.png", "Caption with|extra pipes"},
		{" abc.png | spaced ", "abc.png", "spaced"},
		{"", "", ""},
		{"abc.png|", "abc.png", ""},
		{"|orphan-caption", "", "orphan-caption"},
	}
	for _, tt := range tests {
		file, caption := httputil.SplitImageCaption(tt.entry)
		if file != tt.wantFile || caption != tt.wantCaption {
			t.Errorf("SplitImageCaption(%q) = (%q, %q), want (%q, %q)",
				tt.entry, file, caption, tt.wantFile, tt.wantCaption)
		}
	}
}

func TestJoinImageCaption(t *testing.T) {
	tests := []struct {
		file    string
		caption string
		want    string
	}{
		{"abc.png", "", "abc.png"},
		{"abc.png", "My caption", "abc.png|My caption"},
		{" abc.png ", " spaced ", "abc.png|spaced"},
		{"abc.png", "   ", "abc.png"},
	}
	for _, tt := range tests {
		got := httputil.JoinImageCaption(tt.file, tt.caption)
		if got != tt.want {
			t.Errorf("JoinImageCaption(%q, %q) = %q, want %q",
				tt.file, tt.caption, got, tt.want)
		}
	}
}

func TestSanitiseCaption(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Normal caption", "Normal caption"},
		{"Has|pipe", "Haspipe"},
		{"Has,comma", "Hascomma"},
		{"Has]bracket", "Hasbracket"},
		{"Has<angle>brackets", "Hasanglebrackets"},
		{`Has"quotes"`, "Hasquotes"},
		{"Mixed|,]<>\"chars", "Mixedchars"},
		{"  spaces  ", "spaces"},
	}
	for _, tt := range tests {
		got := httputil.SanitiseCaption(tt.input)
		if got != tt.want {
			t.Errorf("SanitiseCaption(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitiseCaptionTruncation(t *testing.T) {
	long := strings.Repeat("a", 250)
	got := httputil.SanitiseCaption(long)
	if len(got) != 200 {
		t.Errorf("expected length 200, got %d", len(got))
	}
}

func TestReconstructImages(t *testing.T) {
	form := strings.NewReader("images=abc.png%2Cdef.jpg&caption-0=First+caption&caption-1=Second+caption")
	r, _ := http.NewRequest("POST", "/", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ParseForm()

	got := httputil.ReconstructImages(r)
	if len(got) != 2 {
		t.Fatalf("expected 2 images, got %d", len(got))
	}
	if got[0] != "abc.png|First caption" {
		t.Errorf("image 0: got %q, want %q", got[0], "abc.png|First caption")
	}
	if got[1] != "def.jpg|Second caption" {
		t.Errorf("image 1: got %q, want %q", got[1], "def.jpg|Second caption")
	}
}

func TestReconstructImagesNoCaptions(t *testing.T) {
	form := strings.NewReader("images=abc.png%2Cdef.jpg")
	r, _ := http.NewRequest("POST", "/", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ParseForm()

	got := httputil.ReconstructImages(r)
	if len(got) != 2 {
		t.Fatalf("expected 2 images, got %d", len(got))
	}
	if got[0] != "abc.png" {
		t.Errorf("image 0: got %q, want %q", got[0], "abc.png")
	}
	if got[1] != "def.jpg" {
		t.Errorf("image 1: got %q, want %q", got[1], "def.jpg")
	}
}

func TestReconstructImagesPipesStrippedFromImagesField(t *testing.T) {
	form := strings.NewReader("images=abc.png%7Cinjected")
	r, _ := http.NewRequest("POST", "/", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ParseForm()

	got := httputil.ReconstructImages(r)
	if len(got) != 1 {
		t.Fatalf("expected 1 image, got %d", len(got))
	}
	if got[0] != "abc.pnginjected" {
		t.Errorf("pipes should be stripped from images field: got %q", got[0])
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
