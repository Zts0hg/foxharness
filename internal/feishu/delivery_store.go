package feishu

import (
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

type memoryDeliveryStore struct {
	mu       sync.Mutex
	accepted map[string]struct{}
}

func newMemoryDeliveryStore() DeliveryStore {
	return &memoryDeliveryStore{accepted: make(map[string]struct{})}
}

func (s *memoryDeliveryStore) Reserve(messageID string) (bool, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.accepted, strings.TrimSpace(messageID))
	return nil
}

// FileDeliveryStore persists accepted IDs with an atomic file replacement.
type FileDeliveryStore struct {
	path string
	mu   sync.Mutex
}

type deliveryStoreFile struct {
	Version    int      `json:"version"`
	MessageIDs []string `json:"message_ids"`
}

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
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false, errors.New("delivery message ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	accepted, err := s.load()
	if err != nil {
		return false, err
	}
	if _, exists := accepted[messageID]; exists {
		return false, nil
	}
	accepted[messageID] = struct{}{}
	if err := s.save(accepted); err != nil {
		return false, err
	}
	return true, nil
}

// Release rolls back a reservation when the live process cannot enqueue it.
func (s *FileDeliveryStore) Release(messageID string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	accepted, err := s.load()
	if err != nil {
		return err
	}
	if _, exists := accepted[messageID]; !exists {
		return nil
	}
	delete(accepted, messageID)
	return s.save(accepted)
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

func (s *FileDeliveryStore) save(accepted map[string]struct{}) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create delivery store directory: %w", err)
	}
	messageIDs := make([]string, 0, len(accepted))
	for messageID := range accepted {
		messageIDs = append(messageIDs, messageID)
	}
	sort.Strings(messageIDs)
	data, err := json.Marshal(deliveryStoreFile{Version: deliveryStoreVersion, MessageIDs: messageIDs})
	if err != nil {
		return fmt.Errorf("encode delivery store: %w", err)
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".deliveries-*.tmp")
	if err != nil {
		return fmt.Errorf("create delivery store temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("set delivery store permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write delivery store: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync delivery store: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("close delivery store: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("replace delivery store: %w", err)
	}
	return nil
}

var _ DeliveryStore = (*FileDeliveryStore)(nil)
