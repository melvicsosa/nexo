// Command nexo is a package manager for AI development assets: skills,
// plugins and (eventually) MCP servers, across Claude Code, Cursor and
// other agentic coding tools.
//
// main is pure wiring: real ports in, exit code out. Everything else
// lives behind injected interfaces (plan D7).
package main

import (
	"fmt"
	"os"

	"github.com/melvicsosa/nexo/internal/cli"
	"github.com/melvicsosa/nexo/internal/ports"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	paths, err := ports.NewOSPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "nexo: %v\n", err)
		os.Exit(1)
	}
	app := &cli.App{
		FS:      ports.OSFS{},
		Paths:   paths,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Version: version,
		Getwd:   os.Getwd,
	}
	os.Exit(app.Run(os.Args[1:]))
}
