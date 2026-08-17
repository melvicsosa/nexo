package portstest

import "path"

// FakePaths is a ports.Paths for tests, rooted at an arbitrary
// in-memory home directory.
type FakePaths struct {
	HomeDir string
}

func (p FakePaths) Home() string     { return p.HomeDir }
func (p FakePaths) StateDir() string { return path.Join(p.HomeDir, ".nexo") }
