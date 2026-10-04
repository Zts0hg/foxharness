/* Package media persists validated image artifacts beneath a session root. */
package media

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

/* MaxImageBytes bounds encoded image storage and request hydration. */
const MaxImageBytes = 20 * 1024 * 1024

/* Store resolves only content-addressed images beneath an existing session root. */
type Store struct{ dir string }

/* NewStore constructs an artifact store without creating or opening files. */
func NewStore(sessionDir string) *Store { return &Store{dir: sessionDir} }

/* Put validates and durably writes an image before returning its reference. */
func (s *Store) Put(mime string, data []byte) (schema.ContentPart, error) {
	data = append([]byte(nil), data...)
	ext, err := validate(mime, data)
	if err != nil {
		return schema.ContentPart{}, err
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + ext
	root, err := s.openMedia(true)
	if err != nil {
		return schema.ContentPart{}, err
	}
	defer root.Close()
	part := schema.ContentPart{Type: "image", MIMEType: mime, ArtifactKey: "media/" + name}
	if _, err := root.Lstat(name); err == nil {
		if _, err := read(root, name, mime); err != nil {
			return schema.ContentPart{}, err
		}
		return part, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return schema.ContentPart{}, err
	}
	tmp := ".image-" + rand.Text()
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return schema.ContentPart{}, err
	}
	defer root.Remove(tmp)
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return schema.ContentPart{}, err
	}
	if err := root.Rename(tmp, name); err != nil {
		return schema.ContentPart{}, err
	}
	return part, nil
}

/* Resolve loads a confined image and checks its format and content hash. */
func (s *Store) Resolve(part schema.ContentPart) ([]byte, error) {
	name, err := referenceName(part)
	if err != nil {
		return nil, fmt.Errorf("image unavailable: %w", err)
	}
	root, err := s.openMedia(false)
	if err != nil {
		return nil, fmt.Errorf("image unavailable: %w", err)
	}
	defer root.Close()
	data, err := read(root, name, part.MIMEType)
	if err != nil {
		return nil, fmt.Errorf("image unavailable: %w", err)
	}
	return data, nil
}

func (s *Store) openMedia(create bool) (*os.Root, error) {
	if s == nil || s.dir == "" {
		return nil, fmt.Errorf("image store is not configured")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err := root.Mkdir("media", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	info, err := root.Lstat("media")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("media directory is not a regular directory")
	}
	return root.OpenRoot("media")
}

func referenceName(part schema.ContentPart) (string, error) {
	ext := extension(part.MIMEType)
	name := strings.TrimPrefix(part.ArtifactKey, "media/")
	if part.Type != "image" || ext == "" || part.ArtifactKey != "media/"+name || path.Base(name) != name || !strings.HasSuffix(name, ext) {
		return "", fmt.Errorf("invalid image reference")
	}
	hash := strings.TrimSuffix(name, ext)
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != sha256.Size || hash != strings.ToLower(hash) {
		return "", fmt.Errorf("invalid image digest")
	}
	return name, nil
}

func read(root *os.Root, name, mime string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("image artifact is not a regular file")
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || opened.Size() > MaxImageBytes {
		return nil, fmt.Errorf("invalid image artifact size or type")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	ext, err := validate(mime, data)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:])+ext != name {
		return nil, fmt.Errorf("image digest mismatch")
	}
	return data, nil
}

func extension(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	default:
		return ""
	}
}

func validate(mime string, data []byte) (string, error) {
	ext := extension(mime)
	if ext == "" {
		return "", fmt.Errorf("unsupported image MIME type")
	}
	if len(data) == 0 || len(data) > MaxImageBytes {
		return "", fmt.Errorf("image exceeds size limit or is empty")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || "image/"+format != mime || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 100_000_000 {
		return "", fmt.Errorf("invalid image header, format, or dimensions")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("invalid image data: %w", err)
	}
	return ext, nil
}
