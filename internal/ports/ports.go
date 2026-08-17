// Package ports defines the interfaces the core needs from the outside
// world. No adapter or core service touches the OS directly (plan D7):
// they receive these ports injected, which is what makes every piece of
// nexo testable against an in-memory filesystem.
package ports

import (
	"io/fs"
	"time"
)

// FS is the filesystem port. It is intentionally narrow: only the
// operations the core and the adapters actually need.
//
// Path semantics follow the os package: Stat/ReadFile follow symlinks,
// Lstat/Remove/Rename operate on the link itself.
type FS interface {
	Stat(path string) (fs.FileInfo, error)
	Lstat(path string) (fs.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm fs.FileMode) error
	ReadDir(path string) ([]fs.DirEntry, error)
	MkdirAll(path string, perm fs.FileMode) error
	Rename(oldPath, newPath string) error
	Remove(path string) error
	RemoveAll(path string) error
	Symlink(target, link string) error
	Readlink(link string) (string, error)
}

// Paths resolves the well-known locations nexo works with.
type Paths interface {
	// Home is the user's home directory.
	Home() string
	// StateDir is nexo's own metadata directory (~/.nexo).
	StateDir() string
}

// Clock abstracts time so transactions and journals are deterministic
// under test.
type Clock interface {
	Now() time.Time
}
