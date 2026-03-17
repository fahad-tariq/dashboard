package upload

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/fahad/dashboard/internal/httputil"
)

const maxUploadSize = 10 << 20 // 10 MB

var mimeToExt = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

type Handler struct {
	uploadsDir string
}

func NewHandler(uploadsDir string) *Handler {
	return &Handler{uploadsDir: uploadsDir}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	file, _, err := r.FormFile("file")
	if err != nil {
		slog.Error("reading upload", "error", err)
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file upload"})
		return
	}
	defer file.Close()

	// Read first 512 bytes for MIME detection.
	header := make([]byte, 512)
	n, err := io.ReadFull(file, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		slog.Error("reading upload header", "error", err)
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read file"})
		return
	}
	header = header[:n]

	mime := http.DetectContentType(header)
	ext, ok := mimeToExt[mime]
	if !ok {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unsupported image type: %s", mime)})
		return
	}

	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		slog.Error("generating filename", "error", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	filename := hex.EncodeToString(randBytes) + ext

	destPath := filepath.Join(h.uploadsDir, filename)
	dest, err := os.Create(destPath)
	if err != nil {
		slog.Error("creating upload file", "error", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	defer dest.Close()

	// Write the header bytes we already read, then copy the rest.
	if _, err := dest.Write(header); err != nil {
		os.Remove(destPath)
		slog.Error("writing upload", "error", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if _, err := io.Copy(dest, file); err != nil {
		os.Remove(destPath)
		slog.Error("writing upload", "error", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"filename": filename})
}
