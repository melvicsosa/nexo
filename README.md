# nexo

**A package manager for AI development assets.**

Skills, plugins and (eventually) MCP servers live in different places for every AI coding tool — Claude Code, Cursor, Codex. nexo gives you one library and one command to decide exactly where each asset is active: globally, per project, per provider.

> **Status: pre-alpha (Phase 0).** The distribution pipeline is live; the tool itself is being built. See [PLAN.md](PLAN.md) for the roadmap and [docs/spec.md](docs/spec.md) for the full product spec.

## Install

```bash
brew install melvicsosa/tap/nexo
```

Or without Homebrew:

```bash
curl -fsSL https://raw.githubusercontent.com/melvicsosa/nexo/main/scripts/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/melvicsosa/nexo/main/scripts/install.ps1 | iex
```

## The mental model

```
REGISTRY → LIBRARY → { GLOBAL | PROJECT } → { Claude Code | Cursor | … }
```

- **Library** — what you have available.
- **Installation** — where you activated it.
- **Provider** — which AI tool understands it.

## Architecture in one paragraph

A Go single binary. A pure domain core with injected filesystem ports, a transactional install engine, and pluggable provider adapters. Adapters *plan* installations; the core *executes* them — so adding a provider is one new package plus one registration line, with zero transactional code to write. See [PLAN.md](PLAN.md).

## Development

```bash
make check   # vet + test + gofmt
make build   # bin/nexo
```

Releases are cut by tagging (`git tag v0.x.y && git push --tags`); GoReleaser builds the binaries and pushes the Homebrew cask to [melvicsosa/homebrew-tap](https://github.com/melvicsosa/homebrew-tap).

## License

[MIT](LICENSE)
