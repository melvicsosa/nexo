package tx

import (
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/melvicsosa/nexo/internal/ports"
)

// RemoveEmptyDir plans deleting a directory that must be empty by the
// time the step runs (tree plans emit children removals first). Undo
// recreates it. Refusing non-empty dirs keeps every deletion explicit
// and reversible (plan D6).
func RemoveEmptyDir(p string) Step {
	return removeEmptyDir{p: p}
}

type removeEmptyDir struct {
	p string
}

func (s removeEmptyDir) Describe() string { return fmt.Sprintf("rmdir %s", s.p) }

func (s removeEmptyDir) apply(fsys ports.FS) (undo, error) {
	info, err := fsys.Lstat(s.p)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory")
	}
	entries, err := fsys.ReadDir(s.p)
	if err != nil {
		return nil, err
	}
	if len(entries) != 0 {
		return nil, fmt.Errorf("directory not empty")
	}
	mode := info.Mode().Perm()
	if err := fsys.Remove(s.p); err != nil {
		return nil, err
	}
	return func(fsys ports.FS) error {
		return fsys.MkdirAll(s.p, mode)
	}, nil
}

// PlanCopyTree reads src at plan time and emits the steps that
// materialize it at dst: mkdir for directories, an atomic write per
// file, a symlink per symlink (target copied verbatim). Entries whose
// name is in ignore are skipped. Contents are captured into the plan,
// so the plan is self-contained: what DryRun shows is exactly what Run
// writes.
func PlanCopyTree(fsys ports.FS, src, dst string, ignore map[string]bool) ([]Step, error) {
	steps := []Step{MkdirAll(dst, 0o755)}
	if err := planCopyChildren(fsys, src, dst, ignore, &steps); err != nil {
		return nil, fmt.Errorf("plan copy %s: %w", src, err)
	}
	return steps, nil
}

func planCopyChildren(fsys ports.FS, src, dst string, ignore map[string]bool, steps *[]Step) error {
	entries, err := fsys.ReadDir(src)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if ignore[e.Name()] {
			continue
		}
		srcChild := path.Join(src, e.Name())
		dstChild := path.Join(dst, e.Name())
		info, err := fsys.Lstat(srcChild)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := fsys.Readlink(srcChild)
			if err != nil {
				return err
			}
			*steps = append(*steps, Symlink(target, dstChild))
		case info.IsDir():
			*steps = append(*steps, MkdirAll(dstChild, 0o755))
			if err := planCopyChildren(fsys, srcChild, dstChild, ignore, steps); err != nil {
				return err
			}
		default:
			data, err := fsys.ReadFile(srcChild)
			if err != nil {
				return err
			}
			*steps = append(*steps, WriteFile(dstChild, data, info.Mode().Perm()))
		}
	}
	return nil
}

// PlanRemoveTree emits the steps that delete root entirely: children
// depth-first, directories after their contents, every file removal
// individually reversible. A symlinked root is a single link removal —
// never a traversal into the target.
func PlanRemoveTree(fsys ports.FS, root string) ([]Step, error) {
	info, err := fsys.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("plan remove %s: %w", root, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return []Step{Remove(root)}, nil
	}
	var steps []Step
	if err := planRemoveChildren(fsys, root, &steps); err != nil {
		return nil, fmt.Errorf("plan remove %s: %w", root, err)
	}
	steps = append(steps, RemoveEmptyDir(root))
	return steps, nil
}

func planRemoveChildren(fsys ports.FS, dir string, steps *[]Step) error {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		child := path.Join(dir, e.Name())
		info, err := fsys.Lstat(child)
		if err != nil {
			return err
		}
		if info.IsDir() && info.Mode()&fs.ModeSymlink == 0 {
			if err := planRemoveChildren(fsys, child, steps); err != nil {
				return err
			}
			*steps = append(*steps, RemoveEmptyDir(child))
			continue
		}
		*steps = append(*steps, Remove(child))
	}
	return nil
}
