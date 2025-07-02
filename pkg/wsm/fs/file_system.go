package fs

import (
	"io/fs"
	"os"
	"path/filepath"
)

// FileSystem abstracts filesystem operations for testability and flexibility
type FileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	RemoveAll(path string) error
	WriteFile(filename string, data []byte, perm os.FileMode) error
	ReadFile(filename string) ([]byte, error)
	ReadDir(dirname string) ([]os.DirEntry, error)
	Stat(name string) (fs.FileInfo, error)
	UserConfigDir() (string, error)
	UserHomeDir() (string, error)
	Exists(path string) bool
	Join(elem ...string) string
}

// OSFileSystem is the production implementation using the real filesystem
type OSFileSystem struct{}

func NewOSFileSystem() FileSystem {
	return &OSFileSystem{}
}

func (OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (OSFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (OSFileSystem) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (OSFileSystem) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (OSFileSystem) ReadDir(dirname string) ([]os.DirEntry, error) {
	return os.ReadDir(dirname)
}

func (OSFileSystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (OSFileSystem) UserConfigDir() (string, error) {
	return os.UserConfigDir()
}

func (OSFileSystem) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

func (f OSFileSystem) Exists(path string) bool {
	_, err := f.Stat(path)
	return err == nil
}

func (OSFileSystem) Join(elem ...string) string {
	return filepath.Join(elem...)
}
