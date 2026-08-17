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

	"github.com/mattn/go-isatty"

	"github.com/melvicsosa/nexo/internal/cli"
	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/tui"
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
	// The interactive UI only makes sense on a real terminal; piped or
	// scripted invocations get the CLI surface.
	if isatty.IsTerminal(os.Stdout.Fd()) && isatty.IsTerminal(os.Stdin.Fd()) {
		app.LaunchTUI = func() error {
			return tui.Run(tui.Deps{
				FS:      ports.OSFS{},
				Paths:   paths,
				Version: version,
				Getwd:   os.Getwd,
				Clock:   ports.SystemClock{},
			})
		}
	}
	os.Exit(app.Run(os.Args[1:]))
}
