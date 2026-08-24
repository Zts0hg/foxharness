//go:build !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package feishu

import (
	"context"
	"errors"
	"os"
)

func withDeliveryStoreFileLock(context.Context, *os.Root, string, func() error) error {
	return errors.New("file delivery store locking is unsupported on this platform")
}
