package test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/upload"
)

func createTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding test PNG: %v", err)
	}
	return buf.Bytes()
}

func uploadFile(t *testing.T, handler http.Handler, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	part.Write(content)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestUploadValidImage(t *testing.T) {
	dir := t.TempDir()
	h := upload.NewHandler(dir)
	pngData := createTestPNG(t)

	rr := uploadFile(t, http.HandlerFunc(h.Upload), "test.png", pngData)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	filename := resp["filename"]
	if filename == "" {
		t.Fatal("empty filename in response")
	}
	if !strings.HasSuffix(filename, ".png") {
		t.Errorf("filename should end with .png, got %q", filename)
	}

	// Verify file exists on disk.
	if _, err := os.Stat(filepath.Join(dir, filename)); err != nil {
		t.Errorf("uploaded file not found: %v", err)
	}
}

func TestUploadInvalidMIME(t *testing.T) {
	dir := t.TempDir()
	h := upload.NewHandler(dir)

	rr := uploadFile(t, http.HandlerFunc(h.Upload), "test.txt", []byte("this is plain text content"))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rr.Code)
	}
}

func TestUploadSpoofedExtension(t *testing.T) {
	dir := t.TempDir()
	h := upload.NewHandler(dir)
	pngData := createTestPNG(t)

	rr := uploadFile(t, http.HandlerFunc(h.Upload), "malware.php", pngData)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)

	// Should use canonical .png extension, not .php.
	if !strings.HasSuffix(resp["filename"], ".png") {
		t.Errorf("filename should use .png extension, got %q", resp["filename"])
	}

	// Verify no .php files were created.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".php") {
			t.Errorf("found .php file: %s", e.Name())
		}
	}
}

func TestUploadFileTooLarge(t *testing.T) {
	dir := t.TempDir()
	h := upload.NewHandler(dir)

	// 10 MB limit: create content just over that.
	oversized := make([]byte, 11<<20)
	copy(oversized, createTestPNG(t))

	rr := uploadFile(t, http.HandlerFunc(h.Upload), "huge.png", oversized)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400 for oversized upload", rr.Code)
	}
}

func TestUploadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	h := upload.NewHandler(dir)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "empty.png")
	_ = part // write nothing
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	if rr.Code == http.StatusOK {
		t.Errorf("empty file should be rejected, got status 200")
	}
}
