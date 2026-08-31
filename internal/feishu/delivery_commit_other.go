//go:build !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package feishu

import "os"

func commitDeliveryStoreFile(root *os.Root, temporaryPath, targetPath string) (bool, error) {
	if err := root.Rename(temporaryPath, targetPath); err != nil {
		return false, err
	}
	return true, nil
}
