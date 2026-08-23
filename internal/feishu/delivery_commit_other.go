//go:build !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package feishu

import "os"

func commitDeliveryStoreFile(temporaryPath, targetPath string) (bool, error) {
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return false, err
	}
	return true, nil
}
