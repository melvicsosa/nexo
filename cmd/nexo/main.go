// Command nexo is a package manager for AI development assets: skills,
// plugins and (eventually) MCP servers, across Claude Code, Cursor and
// other agentic coding tools.
package main

import (
	"fmt"
	"os"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "-version", "--version", "-v":
			fmt.Printf("nexo %s\n", version)
			return
		}
	}
	fmt.Printf("nexo %s — package manager for AI development assets\n", version)
	fmt.Println("Phase 0 stub: distribution pipeline only. See PLAN.md for the roadmap.")
}
