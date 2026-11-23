package media

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp"
)

const MaxUploadBytes = 15 * 1024 * 1024 // 15 MB

var allowedExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// StoredMedia describes a media file stored locally in the media directory.
type StoredMedia struct {
	ID         string `json:"id"`
	StoredName string `json:"storedName"`
	Filename   string `json:"filename"`
	MimeType   string `json:"mimeType"`
	SizeBytes  int64  `json:"sizeBytes"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	URL        string `json:"url"`
}

// Manager manages reading and writing uploaded media files on disk.
type Manager struct {
	dir string
}

// NewManager initializes the media directory and returns a Manager.
func NewManager(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create media directory: %w", err)
	}
	return &Manager{dir: dir}, nil
}

// SaveUploadedFile validates, detects dimensions, and persists an uploaded image.
func (m *Manager) SaveUploadedFile(r io.Reader, originalFilename string) (StoredMedia, error) {
	limited := io.LimitReader(r, MaxUploadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return StoredMedia{}, fmt.Errorf("read upload: %w", err)
	}
	if len(data) == 0 {
		return StoredMedia{}, errors.New("uploaded file is empty")
	}
	if len(data) > MaxUploadBytes {
		return StoredMedia{}, fmt.Errorf("file exceeds %d MB limit", MaxUploadBytes/(1024*1024))
	}

	mimeType := http.DetectContentType(data)
	ext, ok := allowedExtensions[mimeType]
	if !ok {
		return StoredMedia{}, fmt.Errorf("unsupported image type %q (allowed: JPEG, PNG, WebP, GIF)", mimeType)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return StoredMedia{}, fmt.Errorf("decode image dimensions: %w", err)
	}

	id, err := randomHex(12)
	if err != nil {
		return StoredMedia{}, fmt.Errorf("generate media id: %w", err)
	}

	storedName := id + ext
	destPath := filepath.Join(m.dir, storedName)
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return StoredMedia{}, fmt.Errorf("write media file: %w", err)
	}

	cleanOriginal := filepath.Base(strings.TrimSpace(originalFilename))
	if cleanOriginal == "" || cleanOriginal == "." {
		cleanOriginal = storedName
	}

	return StoredMedia{
		ID:         id,
		StoredName: storedName,
		Filename:   cleanOriginal,
		MimeType:   mimeType,
		SizeBytes:  int64(len(data)),
		Width:      cfg.Width,
		Height:     cfg.Height,
		URL:        "/api/media/" + storedName,
	}, nil
}

// ReadMediaBytes reads the raw bytes and detected MIME type for a stored media filename.
func (m *Manager) ReadMediaBytes(storedName string) ([]byte, string, error) {
	safeName := filepath.Base(storedName)
	if safeName == "" || safeName == "." || safeName == ".." || safeName != storedName {
		return nil, "", errors.New("invalid media filename")
	}

	fullPath := filepath.Join(m.dir, safeName)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, "", err
	}

	mimeType := http.DetectContentType(data)
	return data, mimeType, nil
}

func randomHex(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
