// Package portstest provides test doubles for the ports package: an
// in-memory filesystem with symlink support (MemFS), a fault-injecting
// wrapper (FaultFS) and a deterministic clock. Every core service and
// provider adapter in nexo is tested against these — never against the
// real home directory.
package portstest

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/melvicsosa/nexo/internal/ports"
)

var (
	errNotDir   = errors.New("not a directory")
	errIsDir    = errors.New("is a directory")
	errNotEmpty = errors.New("directory not empty")
	errNotLink  = errors.New("not a symlink")
	errLoop     = errors.New("too many levels of symbolic links")
)

type node struct {
	dir  bool
	link string // symlink target when non-empty
	data []byte
	mode fs.FileMode // permission bits only
}

// MemFS is an in-memory implementation of ports.FS. Paths are
// unix-style; relative paths are treated as rooted at "/".
type MemFS struct {
	nodes map[string]*node
}

var _ ports.FS = (*MemFS)(nil)

// NewMemFS returns an empty filesystem containing only the root dir.
func NewMemFS() *MemFS {
	return &MemFS{nodes: map[string]*node{
		"/": {dir: true, mode: 0o755},
	}}
}

func clean(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

// eval resolves symlinks in every intermediate component, and in the
// final component only when followLast is true. The returned path may
// not exist (callers that require existence check the node map).
func (m *MemFS) eval(p string, followLast bool) (string, error) {
	hops := 0
	var walk func(p string, followLast bool) (string, error)
	walk = func(p string, followLast bool) (string, error) {
		p = clean(p)
		if p == "/" {
			return "/", nil
		}
		comps := strings.Split(strings.TrimPrefix(p, "/"), "/")
		cur := "/"
		for i, c := range comps {
			cur = path.Join(cur, c)
			n, ok := m.nodes[cur]
			if !ok {
				if i < len(comps)-1 {
					return "", &fs.PathError{Op: "open", Path: cur, Err: fs.ErrNotExist}
				}
				return cur, nil // final component may not exist yet
			}
			if n.link != "" {
				last := i == len(comps)-1
				if last && !followLast {
					return cur, nil
				}
				hops++
				if hops > 40 {
					return "", &fs.PathError{Op: "open", Path: cur, Err: errLoop}
				}
				target := n.link
				if !strings.HasPrefix(target, "/") {
					target = path.Join(path.Dir(cur), target)
				}
				rest := append([]string{target}, comps[i+1:]...)
				return walk(path.Join(rest...), followLast)
			}
			if !n.dir && i < len(comps)-1 {
				return "", &fs.PathError{Op: "open", Path: cur, Err: errNotDir}
			}
		}
		return cur, nil
	}
	return walk(p, followLast)
}

func (m *MemFS) get(p string, followLast bool, op string) (string, *node, error) {
	res, err := m.eval(p, followLast)
	if err != nil {
		return "", nil, err
	}
	n, ok := m.nodes[res]
	if !ok {
		return "", nil, &fs.PathError{Op: op, Path: p, Err: fs.ErrNotExist}
	}
	return res, n, nil
}

func (m *MemFS) Stat(p string) (fs.FileInfo, error) {
	res, n, err := m.get(p, true, "stat")
	if err != nil {
		return nil, err
	}
	return memInfo{name: path.Base(res), n: n}, nil
}

func (m *MemFS) Lstat(p string) (fs.FileInfo, error) {
	res, n, err := m.get(p, false, "lstat")
	if err != nil {
		return nil, err
	}
	return memInfo{name: path.Base(res), n: n}, nil
}

func (m *MemFS) ReadFile(p string) ([]byte, error) {
	_, n, err := m.get(p, true, "open")
	if err != nil {
		return nil, err
	}
	if n.dir {
		return nil, &fs.PathError{Op: "read", Path: p, Err: errIsDir}
	}
	out := make([]byte, len(n.data))
	copy(out, n.data)
	return out, nil
}

func (m *MemFS) WriteFile(p string, data []byte, perm fs.FileMode) error {
	res, err := m.eval(p, true)
	if err != nil {
		return err
	}
	parent, ok := m.nodes[path.Dir(res)]
	if !ok {
		return &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	if !parent.dir {
		return &fs.PathError{Op: "open", Path: p, Err: errNotDir}
	}
	if existing, ok := m.nodes[res]; ok && existing.dir {
		return &fs.PathError{Op: "open", Path: p, Err: errIsDir}
	}
	buf := make([]byte, len(data))
	copy(buf, data)
	m.nodes[res] = &node{data: buf, mode: perm.Perm()}
	return nil
}

func (m *MemFS) ReadDir(p string) ([]fs.DirEntry, error) {
	res, n, err := m.get(p, true, "open")
	if err != nil {
		return nil, err
	}
	if !n.dir {
		return nil, &fs.PathError{Op: "readdir", Path: p, Err: errNotDir}
	}
	var names []string
	prefix := res
	if prefix != "/" {
		prefix += "/"
	} else {
		prefix = "/"
	}
	for k := range m.nodes {
		if k == res || !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		if rest == "" || strings.Contains(rest, "/") {
			continue
		}
		names = append(names, rest)
	}
	sort.Strings(names)
	entries := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, memEntry{memInfo{name: name, n: m.nodes[path.Join(res, name)]}})
	}
	return entries, nil
}

func (m *MemFS) MkdirAll(p string, perm fs.FileMode) error {
	p = clean(p)
	if p == "/" {
		return nil
	}
	comps := strings.Split(strings.TrimPrefix(p, "/"), "/")
	cur := "/"
	for _, c := range comps {
		cur = path.Join(cur, c)
		res, err := m.eval(cur, true)
		if err != nil {
			return err
		}
		n, ok := m.nodes[res]
		if !ok {
			m.nodes[res] = &node{dir: true, mode: perm.Perm()}
			cur = res
			continue
		}
		if !n.dir {
			return &fs.PathError{Op: "mkdir", Path: cur, Err: errNotDir}
		}
		cur = res
	}
	return nil
}

func (m *MemFS) Rename(oldPath, newPath string) error {
	oldRes, n, err := m.get(oldPath, false, "rename")
	if err != nil {
		return err
	}
	newRes, err := m.eval(newPath, false)
	if err != nil {
		return err
	}
	parent, ok := m.nodes[path.Dir(newRes)]
	if !ok || !parent.dir {
		return &fs.PathError{Op: "rename", Path: newPath, Err: fs.ErrNotExist}
	}
	if existing, ok := m.nodes[newRes]; ok && existing.dir {
		return &fs.PathError{Op: "rename", Path: newPath, Err: errIsDir}
	}
	// Move the node and, when it is a directory, its whole subtree.
	delete(m.nodes, oldRes)
	m.nodes[newRes] = n
	if n.dir {
		prefix := oldRes + "/"
		moved := map[string]*node{}
		for k, v := range m.nodes {
			if strings.HasPrefix(k, prefix) {
				moved[path.Join(newRes, strings.TrimPrefix(k, prefix))] = v
				delete(m.nodes, k)
			}
		}
		for k, v := range moved {
			m.nodes[k] = v
		}
	}
	return nil
}

func (m *MemFS) Remove(p string) error {
	res, n, err := m.get(p, false, "remove")
	if err != nil {
		return err
	}
	if n.dir {
		prefix := res + "/"
		for k := range m.nodes {
			if strings.HasPrefix(k, prefix) {
				return &fs.PathError{Op: "remove", Path: p, Err: errNotEmpty}
			}
		}
	}
	delete(m.nodes, res)
	return nil
}

func (m *MemFS) RemoveAll(p string) error {
	res, err := m.eval(p, false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if res == "/" {
		return fmt.Errorf("refusing to remove root")
	}
	delete(m.nodes, res)
	prefix := res + "/"
	for k := range m.nodes {
		if strings.HasPrefix(k, prefix) {
			delete(m.nodes, k)
		}
	}
	return nil
}

func (m *MemFS) Symlink(target, link string) error {
	res, err := m.eval(link, false)
	if err != nil {
		return err
	}
	if _, exists := m.nodes[res]; exists {
		return &fs.PathError{Op: "symlink", Path: link, Err: fs.ErrExist}
	}
	parent, ok := m.nodes[path.Dir(res)]
	if !ok || !parent.dir {
		return &fs.PathError{Op: "symlink", Path: link, Err: fs.ErrNotExist}
	}
	m.nodes[res] = &node{link: target, mode: 0o777}
	return nil
}

func (m *MemFS) Readlink(link string) (string, error) {
	_, n, err := m.get(link, false, "readlink")
	if err != nil {
		return "", err
	}
	if n.link == "" {
		return "", &fs.PathError{Op: "readlink", Path: link, Err: errNotLink}
	}
	return n.link, nil
}

// Snapshot renders the whole filesystem as a map for test assertions:
// "dir", "link:<target>" or "file:<content>" keyed by path. The root
// entry is omitted.
func (m *MemFS) Snapshot() map[string]string {
	out := map[string]string{}
	for k, n := range m.nodes {
		if k == "/" {
			continue
		}
		switch {
		case n.dir:
			out[k] = "dir"
		case n.link != "":
			out[k] = "link:" + n.link
		default:
			out[k] = "file:" + string(n.data)
		}
	}
	return out
}

// memInfo implements fs.FileInfo over a node.
type memInfo struct {
	name string
	n    *node
}

func (i memInfo) Name() string { return i.name }
func (i memInfo) Size() int64  { return int64(len(i.n.data)) }
func (i memInfo) Mode() fs.FileMode {
	switch {
	case i.n.dir:
		return i.n.mode | fs.ModeDir
	case i.n.link != "":
		return i.n.mode | fs.ModeSymlink
	default:
		return i.n.mode
	}
}
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return i.n.dir }
func (i memInfo) Sys() any           { return nil }

// memEntry implements fs.DirEntry over memInfo.
type memEntry struct{ info memInfo }

func (e memEntry) Name() string               { return e.info.name }
func (e memEntry) IsDir() bool                { return e.info.IsDir() }
func (e memEntry) Type() fs.FileMode          { return e.info.Mode().Type() }
func (e memEntry) Info() (fs.FileInfo, error) { return e.info, nil }
