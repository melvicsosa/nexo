// Package treehash computes the content hash that IS an asset's
// identity (plan D2). Version metadata is optional in the wild — skill
// frontmatter has no version field — so drift detection, the Broken
// state and safe uninstall all hinge on this hash, never on a version.
package treehash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io/fs"
	"path"
	"sort"

	"github.com/melvicsosa/nexo/internal/ports"
)

// SidecarName is nexo's per-asset metadata file. It describes the
// asset, it is not part of it, so it never participates in the hash —
// otherwise editing metadata would look like content drift.
const SidecarName = ".nexo.yaml"

// ignored are entries that never participate in a hash: macOS noise and
// VCS internals would make identical assets hash differently across
// machines.
var ignored = map[string]bool{
	".DS_Store": true,
	".git":      true,
	SidecarName: true,
}

// Tree hashes the file tree rooted at root: sha256 over a deterministic
// stream of relative path + kind + content (symlinks contribute their
// target, not what they point at). Root may also be a single file.
func Tree(fsys ports.FS, root string) (string, error) {
	info, err := fsys.Lstat(root)
	if err != nil {
		return "", fmt.Errorf("treehash: %w", err)
	}
	h := sha256.New()
	if !info.IsDir() {
		data, err := fsys.ReadFile(root)
		if err != nil {
			return "", fmt.Errorf("treehash: %w", err)
		}
		fmt.Fprintf(h, "F . %d\n", len(data))
		h.Write(data)
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	if err := walk(fsys, root, ".", h); err != nil {
		return "", fmt.Errorf("treehash: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func walk(fsys ports.FS, abs, rel string, h hash.Hash) error {
	entries, err := fsys.ReadDir(abs)
	if err != nil {
		return err
	}
	// ReadDir is sorted on the real OS; sort defensively so every FS
	// implementation produces the same stream.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if ignored[e.Name()] {
			continue
		}
		childAbs := path.Join(abs, e.Name())
		childRel := path.Join(rel, e.Name())
		info, err := fsys.Lstat(childAbs)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := fsys.Readlink(childAbs)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "L %s %d\n", childRel, len(target))
			h.Write([]byte(target))
		case info.IsDir():
			fmt.Fprintf(h, "D %s\n", childRel)
			if err := walk(fsys, childAbs, childRel, h); err != nil {
				return err
			}
		default:
			data, err := fsys.ReadFile(childAbs)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "F %s %d\n", childRel, len(data))
			h.Write(data)
		}
	}
	return nil
}
