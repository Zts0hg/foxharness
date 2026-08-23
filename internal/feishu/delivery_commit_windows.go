//go:build windows

package feishu

import "golang.org/x/sys/windows"

func commitDeliveryStoreFile(temporaryPath, targetPath string) (bool, error) {
	from, err := windows.UTF16PtrFromString(temporaryPath)
	if err != nil {
		return false, err
	}
	to, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return false, err
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return false, err
	}
	return true, nil
}
