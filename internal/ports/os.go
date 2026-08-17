package ports

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// OSFS is the real-filesystem implementation of FS. It is the only
// place (besides cmd wiring) where the os package is used for file
// operations.
type OSFS struct{}

func (OSFS) Stat(path string) (fs.FileInfo, error)  { return os.Stat(path) }
func (OSFS) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }
func (OSFS) ReadFile(path string) ([]byte, error)   { return os.ReadFile(path) }
func (OSFS) WriteFile(path string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(path, data, perm)
}
func (OSFS) ReadDir(path string) ([]fs.DirEntry, error)   { return os.ReadDir(path) }
func (OSFS) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }
func (OSFS) Rename(oldPath, newPath string) error         { return os.Rename(oldPath, newPath) }
func (OSFS) Remove(path string) error                     { return os.Remove(path) }
func (OSFS) RemoveAll(path string) error                  { return os.RemoveAll(path) }
func (OSFS) Symlink(target, link string) error            { return os.Symlink(target, link) }
func (OSFS) Readlink(link string) (string, error)         { return os.Readlink(link) }

// OSPaths resolves real user locations.
type OSPaths struct {
	home string
}

// NewOSPaths resolves the current user's home directory once.
func NewOSPaths() (OSPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return OSPaths{}, err
	}
	return OSPaths{home: home}, nil
}

func (p OSPaths) Home() string     { return p.home }
func (p OSPaths) StateDir() string { return filepath.Join(p.home, ".nexo") }

// SystemClock is the real clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }
