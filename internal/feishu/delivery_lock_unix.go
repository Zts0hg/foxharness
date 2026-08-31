//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package feishu

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const deliveryStoreLockPollInterval = 10 * time.Millisecond

func withDeliveryStoreFileLock(ctx context.Context, root *os.Root, path string, operation func() error) error {
	lock, err := openRootedRegularFile(root, path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open delivery store lock: %w", err)
	}
	if err := acquireDeliveryStoreFileLock(ctx, int(lock.Fd())); err != nil {
		_ = lock.Close()
		return err
	}
	operationErr := operation()
	unlockErr := unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	closeErr := lock.Close()
	return errors.Join(operationErr, unlockErr, closeErr)
}

func acquireDeliveryStoreFileLock(ctx context.Context, fd int) error {
	for {
		err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fmt.Errorf("lock delivery store: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(deliveryStoreLockPollInterval):
		}
	}
}
