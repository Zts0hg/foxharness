package checkpoint

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// File is the writable file handle returned by FS.OpenFile.
type File interface {
	fs.File
	io.Reader
	io.Writer
}

// FS abstracts filesystem operations for checkpoint tests.
type FS interface {
	Stat(name string) (os.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	CopyFile(dstPath, srcPath string, perm os.FileMode) error
	Remove(name string) error
	MkdirAll(path string, perm os.FileMode) error
	Open(name string) (fs.File, error)
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
}

type osFS struct{}

func (osFS) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (osFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (osFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return err
	}
	return os.WriteFile(name, data, perm)
}

func (osFS) CopyFile(dstPath, srcPath string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(dstPath, perm)
}

func (osFS) Remove(name string) error {
	return os.Remove(name)
}

func (osFS) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osFS) Open(name string) (fs.File, error) {
	return os.Open(name)
}

func (osFS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	return os.OpenFile(name, flag, perm)
}
