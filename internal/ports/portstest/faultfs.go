package portstest

import (
	"io/fs"
	"time"

	"github.com/melvicsosa/nexo/internal/ports"
)

// FaultFS wraps a ports.FS and fails specific operations on specific
// paths. Keys in FailOn are "op path", e.g. "rename /a/b" or
// "writefile /x/y". This is how transaction rollback is exercised
// deterministically in tests.
type FaultFS struct {
	Inner  ports.FS
	FailOn map[string]error
}

var _ ports.FS = (*FaultFS)(nil)

func (f *FaultFS) check(op, p string) error {
	if err, ok := f.FailOn[op+" "+p]; ok {
		return err
	}
	return nil
}

func (f *FaultFS) Stat(p string) (fs.FileInfo, error) {
	if err := f.check("stat", p); err != nil {
		return nil, err
	}
	return f.Inner.Stat(p)
}

func (f *FaultFS) Lstat(p string) (fs.FileInfo, error) {
	if err := f.check("lstat", p); err != nil {
		return nil, err
	}
	return f.Inner.Lstat(p)
}

func (f *FaultFS) ReadFile(p string) ([]byte, error) {
	if err := f.check("readfile", p); err != nil {
		return nil, err
	}
	return f.Inner.ReadFile(p)
}

func (f *FaultFS) WriteFile(p string, data []byte, perm fs.FileMode) error {
	if err := f.check("writefile", p); err != nil {
		return err
	}
	return f.Inner.WriteFile(p, data, perm)
}

func (f *FaultFS) ReadDir(p string) ([]fs.DirEntry, error) {
	if err := f.check("readdir", p); err != nil {
		return nil, err
	}
	return f.Inner.ReadDir(p)
}

func (f *FaultFS) MkdirAll(p string, perm fs.FileMode) error {
	if err := f.check("mkdirall", p); err != nil {
		return err
	}
	return f.Inner.MkdirAll(p, perm)
}

func (f *FaultFS) Rename(oldPath, newPath string) error {
	if err := f.check("rename", oldPath); err != nil {
		return err
	}
	return f.Inner.Rename(oldPath, newPath)
}

func (f *FaultFS) Remove(p string) error {
	if err := f.check("remove", p); err != nil {
		return err
	}
	return f.Inner.Remove(p)
}

func (f *FaultFS) RemoveAll(p string) error {
	if err := f.check("removeall", p); err != nil {
		return err
	}
	return f.Inner.RemoveAll(p)
}

func (f *FaultFS) Symlink(target, link string) error {
	if err := f.check("symlink", link); err != nil {
		return err
	}
	return f.Inner.Symlink(target, link)
}

func (f *FaultFS) Readlink(link string) (string, error) {
	if err := f.check("readlink", link); err != nil {
		return "", err
	}
	return f.Inner.Readlink(link)
}

// FakeClock returns strictly increasing times so journal entries get
// unique, deterministic identifiers under test.
type FakeClock struct {
	T time.Time
}

func (c *FakeClock) Now() time.Time {
	c.T = c.T.Add(time.Second)
	return c.T
}
