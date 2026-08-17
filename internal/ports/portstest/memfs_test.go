package portstest

import (
	"errors"
	"io/fs"
	"testing"
)

func TestMemFSWriteRead(t *testing.T) {
	m := NewMemFS()
	if err := m.MkdirAll("/a/b", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := m.WriteFile("/a/b/f.txt", []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := m.ReadFile("/a/b/f.txt")
	if err != nil || string(data) != "hello" {
		t.Fatalf("ReadFile = %q, %v", data, err)
	}
	// Writing into a missing parent must fail like the OS does.
	if err := m.WriteFile("/missing/f.txt", nil, 0o644); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("write to missing parent: got %v, want ErrNotExist", err)
	}
	// Reading a missing file must be ErrNotExist.
	if _, err := m.ReadFile("/a/b/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("read missing: got %v, want ErrNotExist", err)
	}
}

func TestMemFSSymlinks(t *testing.T) {
	m := NewMemFS()
	if err := m.MkdirAll("/lib/asset", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/lib/asset/SKILL.md", []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.MkdirAll("/home/skills", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("/lib/asset", "/home/skills/asset"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Stat follows the link, Lstat does not.
	if info, err := m.Stat("/home/skills/asset"); err != nil || !info.IsDir() {
		t.Errorf("Stat through link: info=%v err=%v, want dir", info, err)
	}
	info, err := m.Lstat("/home/skills/asset")
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("Lstat: mode=%v err=%v, want symlink", info.Mode(), err)
	}

	// Reading THROUGH the link resolves to the target's child.
	data, err := m.ReadFile("/home/skills/asset/SKILL.md")
	if err != nil || string(data) != "content" {
		t.Errorf("read through link = %q, %v", data, err)
	}

	// Readlink returns the raw target.
	if target, err := m.Readlink("/home/skills/asset"); err != nil || target != "/lib/asset" {
		t.Errorf("Readlink = %q, %v", target, err)
	}

	// Symlink over an existing path must fail.
	if err := m.Symlink("/lib/asset", "/home/skills/asset"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("symlink over existing: got %v, want ErrExist", err)
	}

	// Cycles terminate with an error instead of hanging.
	if err := m.Symlink("/loop/b", "/loop"); err == nil {
		t.Skip("parent missing — expected")
	}
	if err := m.MkdirAll("/loop", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("/loop/a", "/loop/b"); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("/loop/b", "/loop/a"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stat("/loop/a"); err == nil {
		t.Error("symlink cycle resolved without error")
	}
}

func TestMemFSRenameSubtree(t *testing.T) {
	m := NewMemFS()
	for _, d := range []string{"/src/sub", "/dst"} {
		if err := m.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.WriteFile("/src/sub/f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename("/src", "/dst/moved"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if data, err := m.ReadFile("/dst/moved/sub/f"); err != nil || string(data) != "x" {
		t.Errorf("after rename: %q, %v", data, err)
	}
	if _, err := m.Lstat("/src"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("old path still exists: %v", err)
	}
}

func TestMemFSRemove(t *testing.T) {
	m := NewMemFS()
	if err := m.MkdirAll("/d/inner", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/d/inner/f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Remove refuses non-empty dirs; RemoveAll does not.
	if err := m.Remove("/d"); err == nil {
		t.Error("Remove of non-empty dir succeeded")
	}
	if err := m.RemoveAll("/d"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := m.Lstat("/d/inner/f"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("subtree survived RemoveAll: %v", err)
	}
	// RemoveAll of a missing path is a no-op, like os.RemoveAll.
	if err := m.RemoveAll("/never-existed"); err != nil {
		t.Errorf("RemoveAll missing: %v", err)
	}
}

func TestMemFSReadDirSorted(t *testing.T) {
	m := NewMemFS()
	if err := m.MkdirAll("/d", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zeta", "alpha", "mid"} {
		if err := m.WriteFile("/d/"+name, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := m.ReadDir("/d")
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, e := range entries {
		got = append(got, e.Name())
	}
	want := []string{"alpha", "mid", "zeta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ReadDir order = %v, want %v", got, want)
		}
	}
}
