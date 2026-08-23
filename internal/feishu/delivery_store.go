package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	path string
}

type deliveryStoreFile struct {
	Version    int      `json:"version"`
	MessageIDs []string `json:"message_ids"`
}

var commitDeliveryStoreFileFunc = commitDeliveryStoreFile

// NewFileDeliveryStore creates a file-backed authority without reading
// ambient HOME or configuration.
func NewFileDeliveryStore(path string) (*FileDeliveryStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("delivery store path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve delivery store path: %w", err)
	}
	return &FileDeliveryStore{path: filepath.Clean(abs)}, nil
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
	err := withDeliveryStoreLock(ctx, s.path, func() error {
		accepted, err := s.load()
		if err != nil {
			return err
		}
		if _, exists := accepted[messageID]; exists {
			return nil
		}
		accepted[messageID] = struct{}{}
		committed, err := s.save(accepted)
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
	return withDeliveryStoreLock(ctx, s.path, func() error {
		accepted, err := s.load()
		if err != nil {
			return err
		}
		if _, exists := accepted[messageID]; !exists {
			return nil
		}
		delete(accepted, messageID)
		_, err = s.save(accepted)
		return err
	})
}

func (s *FileDeliveryStore) load() (map[string]struct{}, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]struct{}), nil
		}
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

func (s *FileDeliveryStore) save(accepted map[string]struct{}) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return false, fmt.Errorf("create delivery store directory: %w", err)
	}
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

	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".deliveries-*.tmp")
	if err != nil {
		return false, fmt.Errorf("create delivery store temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		return false, fmt.Errorf("set delivery store permissions: %w", err)
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
		_ = os.Remove(temporaryPath)
		return false, fmt.Errorf("close delivery store: %w", err)
	}
	committed, err := commitDeliveryStoreFileFunc(temporaryPath, s.path)
	if err != nil {
		if !committed {
			_ = os.Remove(temporaryPath)
		}
		return committed, fmt.Errorf("replace delivery store: %w", err)
	}
	return committed, nil
}

var _ DeliveryStore = (*FileDeliveryStore)(nil)
