//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package feishu

import (
	"errors"
	"os"
	"path/filepath"
)

func commitDeliveryStoreFile(root *os.Root, temporaryPath, targetPath string) (bool, error) {
	if err := root.Rename(temporaryPath, targetPath); err != nil {
		return false, err
	}
	directory, err := root.Open(filepath.Dir(targetPath))
	if err != nil {
		return true, err
	}
	return true, errors.Join(directory.Sync(), directory.Close())
}
