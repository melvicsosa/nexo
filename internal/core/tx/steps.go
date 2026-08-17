package tx

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/melvicsosa/nexo/internal/ports"
)

// tmpSuffix marks staged files. Staging + rename gives per-file
// atomicity: a crash mid-write leaves a *.nexo-tmp orphan, never a
// half-written destination.
const tmpSuffix = ".nexo-tmp"

// WriteFile plans an atomic file write: stage to a temp sibling, read
// back to verify, rename into place. If the destination already exists
// its content and mode are captured for rollback. Refuses to write
// over a directory or a symlink — plans must be explicit about those.
func WriteFile(p string, data []byte, perm fs.FileMode) Step {
	return writeFile{p: p, data: data, perm: perm}
}

type writeFile struct {
	p    string
	data []byte
	perm fs.FileMode
}

func (s writeFile) Describe() string { return fmt.Sprintf("write %s (%d bytes)", s.p, len(s.data)) }

func (s writeFile) apply(fsys ports.FS) (undo, error) {
	var prior []byte
	var priorMode fs.FileMode
	priorExists := false
	if info, err := fsys.Lstat(s.p); err == nil {
		if info.IsDir() {
			return nil, fmt.Errorf("destination is a directory")
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("destination is a symlink; plan its removal explicitly")
		}
		data, err := fsys.ReadFile(s.p)
		if err != nil {
			return nil, fmt.Errorf("reading prior content: %w", err)
		}
		prior, priorMode, priorExists = data, info.Mode().Perm(), true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	tmp := s.p + tmpSuffix
	if err := fsys.WriteFile(tmp, s.data, s.perm); err != nil {
		return nil, fmt.Errorf("staging: %w", err)
	}
	// Verify the staged bytes before they can shadow the destination.
	staged, err := fsys.ReadFile(tmp)
	if err != nil || !bytes.Equal(staged, s.data) {
		_ = fsys.Remove(tmp)
		if err == nil {
			err = fmt.Errorf("staged content mismatch")
		}
		return nil, fmt.Errorf("verifying stage: %w", err)
	}
	if err := fsys.Rename(tmp, s.p); err != nil {
		_ = fsys.Remove(tmp)
		return nil, fmt.Errorf("committing: %w", err)
	}

	return func(fsys ports.FS) error {
		if priorExists {
			return fsys.WriteFile(s.p, prior, priorMode)
		}
		return fsys.Remove(s.p)
	}, nil
}

// MkdirAll plans directory creation. Rollback removes only the topmost
// directory that did not exist before — never pre-existing parents.
func MkdirAll(p string, perm fs.FileMode) Step {
	return mkdirAll{p: p, perm: perm}
}

type mkdirAll struct {
	p    string
	perm fs.FileMode
}

func (s mkdirAll) Describe() string { return fmt.Sprintf("mkdir -p %s", s.p) }

func (s mkdirAll) apply(fsys ports.FS) (undo, error) {
	// Find the topmost missing ancestor before creating anything.
	created := ""
	for probe := path.Clean(s.p); probe != "/" && probe != "."; probe = path.Dir(probe) {
		if _, err := fsys.Lstat(probe); err == nil {
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		created = probe
	}
	if err := fsys.MkdirAll(s.p, s.perm); err != nil {
		return nil, err
	}
	if created == "" {
		return nil, nil // everything already existed; nothing to undo
	}
	top := created
	return func(fsys ports.FS) error {
		return fsys.RemoveAll(top)
	}, nil
}

// Symlink plans creating a symlink. Fails if anything already exists at
// the link path — replacing is a Remove step plus a Symlink step, so
// the plan (and its dry-run) says exactly what will happen.
func Symlink(target, link string) Step {
	return symlink{target: target, link: link}
}

type symlink struct {
	target, link string
}

func (s symlink) Describe() string { return fmt.Sprintf("symlink %s -> %s", s.link, s.target) }

func (s symlink) apply(fsys ports.FS) (undo, error) {
	if _, err := fsys.Lstat(s.link); err == nil {
		return nil, fmt.Errorf("link path already exists")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := fsys.Symlink(s.target, s.link); err != nil {
		return nil, err
	}
	return func(fsys ports.FS) error {
		return fsys.Remove(s.link)
	}, nil
}

// Remove plans deleting a file or symlink, capturing enough to restore
// it on rollback. Directories are refused: v1 plans enumerate files so
// every deletion is explicit and reversible (plan D6).
func Remove(p string) Step {
	return removeStep{p: p}
}

type removeStep struct {
	p string
}

func (s removeStep) Describe() string { return fmt.Sprintf("remove %s", s.p) }

func (s removeStep) apply(fsys ports.FS) (undo, error) {
	info, err := fsys.Lstat(s.p)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("is a directory; plans remove files explicitly")
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := fsys.Readlink(s.p)
		if err != nil {
			return nil, err
		}
		if err := fsys.Remove(s.p); err != nil {
			return nil, err
		}
		return func(fsys ports.FS) error {
			return fsys.Symlink(target, s.p)
		}, nil
	}
	data, err := fsys.ReadFile(s.p)
	if err != nil {
		return nil, err
	}
	mode := info.Mode().Perm()
	if err := fsys.Remove(s.p); err != nil {
		return nil, err
	}
	return func(fsys ports.FS) error {
		return fsys.WriteFile(s.p, data, mode)
	}, nil
}
