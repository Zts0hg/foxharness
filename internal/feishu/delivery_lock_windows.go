//go:build windows

package feishu

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

const deliveryStoreLockPollInterval = 10 * time.Millisecond

func withDeliveryStoreFileLock(ctx context.Context, path string, operation func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create delivery store directory: %w", err)
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open delivery store lock: %w", err)
	}
	overlapped := &windows.Overlapped{}
	if err := acquireDeliveryStoreFileLock(ctx, windows.Handle(lock.Fd()), overlapped); err != nil {
		_ = lock.Close()
		return err
	}
	operationErr := operation()
	unlockErr := windows.UnlockFileEx(windows.Handle(lock.Fd()), 0, 1, 0, overlapped)
	closeErr := lock.Close()
	return errors.Join(operationErr, unlockErr, closeErr)
}

func acquireDeliveryStoreFileLock(ctx context.Context, handle windows.Handle, overlapped *windows.Overlapped) error {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	for {
		err := windows.LockFileEx(handle, flags, 0, 1, 0, overlapped)
		if err == nil {
			return nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return fmt.Errorf("lock delivery store: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(deliveryStoreLockPollInterval):
		}
	}
}
