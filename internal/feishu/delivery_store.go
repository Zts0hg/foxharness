package feishu

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const deliveryStoreVersion = 1

// DeliveryStore is the durable at-most-once acceptance authority for Feishu
// message IDs. Reserve returns true only for the first accepted delivery.
type DeliveryStore interface {
	Reserve(messageID string) (bool, error)
	Release(messageID string) error
}

type contextDeliveryStore interface {
	ReserveContext(context.Context, string) (bool, error)
	ReleaseContext(context.Context, string) error
}

type memoryDeliveryStore struct {
	mu       sync.Mutex
	accepted map[string]struct{}
}

func newMemoryDeliveryStore() DeliveryStore {
	return &memoryDeliveryStore{accepted: make(map[string]struct{})}
}

func (s *memoryDeliveryStore) Reserve(messageID string) (bool, error) {
	return s.ReserveContext(context.Background(), messageID)
}

func (s *memoryDeliveryStore) ReserveContext(ctx context.Context, messageID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false, errors.New("delivery message ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.accepted[messageID]; exists {
		return false, nil
	}
	s.accepted[messageID] = struct{}{}
	return true, nil
}

func (s *memoryDeliveryStore) Release(messageID string) error {
	return s.ReleaseContext(context.Background(), messageID)
}

func (s *memoryDeliveryStore) ReleaseContext(ctx context.Context, messageID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.accepted, strings.TrimSpace(messageID))
	return nil
}

// FileDeliveryStore persists accepted IDs with an atomic file replacement.
type FileDeliveryStore struct {
	rootPath string
	path     string
	lockKey  string
}

type deliveryStoreFile struct {
	Version    int      `json:"version"`
	MessageIDs []string `json:"message_ids"`
}

var commitDeliveryStoreFileFunc = commitDeliveryStoreFile

// NewFileDeliveryStore creates a file-backed authority beneath an explicit
// trusted root without reading ambient HOME or configuration.
func NewFileDeliveryStore(rootPath, path string) (*FileDeliveryStore, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return nil, errors.New("delivery store root is required")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("delivery store path is required")
	}
	if !filepath.IsLocal(path) || filepath.Clean(path) == "." {
		return nil, errors.New("delivery store path must be local to its root")
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve delivery store root: %w", err)
	}
	root, err := os.OpenRoot(absRoot)
	if err != nil {
		return nil, fmt.Errorf("open delivery store root: %w", err)
	}
	if err := root.Close(); err != nil {
		return nil, fmt.Errorf("close delivery store root: %w", err)
	}
	path = filepath.Clean(path)
	return &FileDeliveryStore{
		rootPath: filepath.Clean(absRoot),
		path:     path,
		lockKey:  filepath.Clean(absRoot) + "\x00" + path,
	}, nil
}

// Reserve atomically persists a previously unseen message ID.
func (s *FileDeliveryStore) Reserve(messageID string) (bool, error) {
	return s.ReserveContext(context.Background(), messageID)
}

func (s *FileDeliveryStore) ReserveContext(ctx context.Context, messageID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false, errors.New("delivery message ID is required")
	}
	var reserved bool
	err := s.withLock(ctx, func(root *os.Root) error {
		accepted, err := s.load(root)
		if err != nil {
			return err
		}
		if _, exists := accepted[messageID]; exists {
			return nil
		}
		accepted[messageID] = struct{}{}
		committed, err := s.save(root, accepted)
		if err != nil {
			if committed {
				reserved = true
			}
			return err
		}
		reserved = true
		return nil
	})
	return reserved, err
}

// Release rolls back a reservation when the live process cannot enqueue it.
func (s *FileDeliveryStore) Release(messageID string) error {
	return s.ReleaseContext(context.Background(), messageID)
}

func (s *FileDeliveryStore) ReleaseContext(ctx context.Context, messageID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil
	}
	return s.withLock(ctx, func(root *os.Root) error {
		accepted, err := s.load(root)
		if err != nil {
			return err
		}
		if _, exists := accepted[messageID]; !exists {
			return nil
		}
		delete(accepted, messageID)
		_, err = s.save(root, accepted)
		return err
	})
}

func (s *FileDeliveryStore) withLock(ctx context.Context, operation func(*os.Root) error) error {
	return withDeliveryStoreLock(ctx, s.lockKey, func() (operationErr error) {
		root, err := os.OpenRoot(s.rootPath)
		if err != nil {
			return fmt.Errorf("open delivery store root: %w", err)
		}
		defer func() {
			operationErr = errors.Join(operationErr, root.Close())
		}()
		if err := root.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
			return fmt.Errorf("create delivery store directory: %w", err)
		}
		return withDeliveryStoreFileLock(ctx, root, s.path+".lock", func() error {
			return operation(root)
		})
	})
}

func (s *FileDeliveryStore) load(root *os.Root) (map[string]struct{}, error) {
	fileHandle, err := openRootedRegularFile(root, s.path, os.O_RDONLY, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]struct{}), nil
		}
		return nil, fmt.Errorf("read delivery store: %w", err)
	}
	data, readErr := io.ReadAll(fileHandle)
	closeErr := fileHandle.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, fmt.Errorf("read delivery store: %w", err)
	}
	var file deliveryStoreFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode delivery store: %w", err)
	}
	if file.Version != deliveryStoreVersion {
		return nil, fmt.Errorf("unsupported delivery store version %d", file.Version)
	}
	accepted := make(map[string]struct{}, len(file.MessageIDs))
	for _, messageID := range file.MessageIDs {
		messageID = strings.TrimSpace(messageID)
		if messageID == "" {
			return nil, errors.New("delivery store contains an empty message ID")
		}
		accepted[messageID] = struct{}{}
	}
	return accepted, nil
}

func (s *FileDeliveryStore) save(root *os.Root, accepted map[string]struct{}) (bool, error) {
	messageIDs := make([]string, 0, len(accepted))
	for messageID := range accepted {
		messageIDs = append(messageIDs, messageID)
	}
	sort.Strings(messageIDs)
	data, err := json.Marshal(deliveryStoreFile{Version: deliveryStoreVersion, MessageIDs: messageIDs})
	if err != nil {
		return false, fmt.Errorf("encode delivery store: %w", err)
	}
	data = append(data, '\n')

	temporary, temporaryPath, err := createRootedDeliveryStoreTemp(root, filepath.Dir(s.path))
	if err != nil {
		return false, fmt.Errorf("create delivery store temporary file: %w", err)
	}
	cleanup := func() {
		_ = temporary.Close()
		_ = root.Remove(temporaryPath)
	}
	if _, err := temporary.Write(data); err != nil {
		cleanup()
		return false, fmt.Errorf("write delivery store: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return false, fmt.Errorf("sync delivery store: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = root.Remove(temporaryPath)
		return false, fmt.Errorf("close delivery store: %w", err)
	}
	committed, err := commitDeliveryStoreFileFunc(root, temporaryPath, s.path)
	if err != nil {
		if !committed {
			_ = root.Remove(temporaryPath)
		}
		return committed, fmt.Errorf("replace delivery store: %w", err)
	}
	return committed, nil
}

func openRootedRegularFile(root *os.Root, path string, flag int, perm os.FileMode) (*os.File, error) {
	fileHandle, err := root.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	closeWithError := func(err error) (*os.File, error) {
		return nil, errors.Join(err, fileHandle.Close())
	}
	pathInfo, err := root.Lstat(path)
	if err != nil {
		return closeWithError(err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return closeWithError(errors.New("delivery store authority must not be a symbolic link"))
	}
	openedInfo, err := fileHandle.Stat()
	if err != nil {
		return closeWithError(err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) {
		return closeWithError(errors.New("delivery store authority must be the rooted regular file opened"))
	}
	return fileHandle, nil
}

func createRootedDeliveryStoreTemp(root *os.Root, directory string) (*os.File, string, error) {
	for attempt := 0; attempt < 100; attempt++ {
		var entropy [8]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return nil, "", err
		}
		path := filepath.Join(directory, fmt.Sprintf(".deliveries-%x.tmp", entropy))
		fileHandle, err := openRootedRegularFile(root, path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return fileHandle, path, err
	}
	return nil, "", errors.New("exhausted delivery store temporary file names")
}

var _ DeliveryStore = (*FileDeliveryStore)(nil)
