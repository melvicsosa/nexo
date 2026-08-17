package treehash

import (
	"testing"

	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

func buildAsset(t *testing.T, m *portstest.MemFS, root string, files map[string]string) {
	t.Helper()
	if err := m.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		dir := root
		if idx := lastSlash(name); idx >= 0 {
			dir = root + "/" + name[:idx]
			if err := m.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := m.WriteFile(root+"/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func TestTreeDeterministicAndOrderIndependent(t *testing.T) {
	files := map[string]string{
		"SKILL.md":     "# skill",
		"ref/notes.md": "notes",
	}
	m1 := portstest.NewMemFS()
	buildAsset(t, m1, "/a", files)

	// Same content created in a different order on a different FS.
	m2 := portstest.NewMemFS()
	if err := m2.MkdirAll("/other/ref", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m2.WriteFile("/other/ref/notes.md", []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m2.WriteFile("/other/SKILL.md", []byte("# skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	h1, err := Tree(m1, "/a")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := Tree(m2, "/other")
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("identical trees hash differently: %s vs %s", h1, h2)
	}
}

func TestTreeContentChangesHash(t *testing.T) {
	m := portstest.NewMemFS()
	buildAsset(t, m, "/a", map[string]string{"SKILL.md": "v1"})
	h1, err := Tree(m, "/a")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/a/SKILL.md", []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	h2, err := Tree(m, "/a")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("content change did not change the hash")
	}
}

func TestTreePathChangesHash(t *testing.T) {
	m1 := portstest.NewMemFS()
	buildAsset(t, m1, "/a", map[string]string{"one.md": "same"})
	m2 := portstest.NewMemFS()
	buildAsset(t, m2, "/a", map[string]string{"two.md": "same"})
	h1, _ := Tree(m1, "/a")
	h2, _ := Tree(m2, "/a")
	if h1 == h2 {
		t.Error("same content under different names must hash differently")
	}
}

func TestTreeIgnoresNoiseAndSidecar(t *testing.T) {
	m := portstest.NewMemFS()
	buildAsset(t, m, "/a", map[string]string{"SKILL.md": "x"})
	base, err := Tree(m, "/a")
	if err != nil {
		t.Fatal(err)
	}
	for _, noise := range []string{".DS_Store", SidecarName} {
		if err := m.WriteFile("/a/"+noise, []byte("junk"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.MkdirAll("/a/.git", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/a/.git/HEAD", []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Tree(m, "/a")
	if err != nil {
		t.Fatal(err)
	}
	if got != base {
		t.Error("ignored files changed the hash")
	}
}

func TestTreeSymlinkTargetMatters(t *testing.T) {
	m := portstest.NewMemFS()
	buildAsset(t, m, "/a", map[string]string{"f": "x"})
	if err := m.Symlink("/one", "/a/link"); err != nil {
		t.Fatal(err)
	}
	h1, err := Tree(m, "/a")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("/a/link"); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("/two", "/a/link"); err != nil {
		t.Fatal(err)
	}
	h2, err := Tree(m, "/a")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("symlink target change did not change the hash")
	}
}

func TestTreeSingleFileRoot(t *testing.T) {
	m := portstest.NewMemFS()
	if err := m.WriteFile("/f.md", []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Tree(m, "/f.md"); err != nil {
		t.Fatalf("single-file root: %v", err)
	}
	if _, err := Tree(m, "/missing"); err == nil {
		t.Error("missing root must error")
	}
}
