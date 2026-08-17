# nexo

**A package manager for AI development assets.**

Skills, plugins and (eventually) MCP servers live in different places for every AI coding tool — Claude Code, Cursor, Codex. nexo gives you one library and one command to decide exactly where each asset is active: globally, per project, per provider.

```
REGISTRY → fetch → LIBRARY → install → { GLOBAL | PROJECT } → { Claude Code | Cursor | Codex }
```

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

## Quick tour

```bash
nexo                       # interactive UI (on a terminal)
nexo providers             # which AI tools are on this machine, and what they support
nexo list                  # everything installed globally, per provider
nexo project inspect       # what THIS repo has configured — works on repos nexo never touched

nexo adopt go-testing      # bring an existing skill into your library
nexo install go-testing --global            # symlink: one source of truth
nexo install go-testing --project ~/x       # copy: the repo stays self-contained
nexo doctor                # verify every record against reality (hash-based)

nexo registry add company ~/registries/company
nexo search wordpress
nexo fetch company/wordpress-review
nexo install company/wordpress-review --project ~/client-a

nexo plugin enable engram@engram            # flip one key in the provider's config
nexo marketplace sync                       # expose library plugins to Claude Code natively
```

Every listing command takes `--json`. Every mutating command takes `--dry-run`, and the plan it prints is byte-exact — what you see is what runs.

## The safety model

- **Identity is content.** Every asset is identified by a sha256 tree hash; versions are optional metadata (skills in the wild have none). Drift is detected by hash, never guessed.
- **Everything is transactional.** Installs stage, verify, then commit; any failure rolls the filesystem back byte-identical — including the project manifest written in the same transaction.
- **nexo only deletes what it can prove it installed.** Assets it merely *detected* are never touched: adopt them or remove them yourself. Modified installs require an explicit `--force`.
- **Providers are never fought.** Claude Code plugins install through Claude's own marketplace mechanism — nexo generates the marketplace and manages the enable flag, nothing more. Cursor's self-synced skill set is off-limits. Codex's `auth.json` is never opened.
- **Capabilities are declared, not assumed.** If an adapter can't do something safely yet, it says so and the UI hides the action.

## Architecture

A Go binary with a pure domain core, injected filesystem ports (every test runs against an in-memory FS — none touches your home), a sealed-vocabulary transaction engine, and pluggable provider adapters. **Adapters plan; the core executes.** Adding a provider is one package plus one registration line, with zero transactional code to write — see [docs/ADDING_A_PROVIDER.md](docs/ADDING_A_PROVIDER.md).

Research spikes ground every adapter in a real machine before code exists: [Cursor](docs/providers/cursor.md) · [Codex](docs/providers/codex.md). Product spec: [docs/spec.md](docs/spec.md) · roadmap: [PLAN.md](PLAN.md).

## Development

```bash
make check   # vet + test + gofmt
make build   # bin/nexo
```

Releases are cut by tagging (`git tag v0.x.y && git push --tags`); GoReleaser builds darwin/linux/windows binaries and pushes the Homebrew cask to [melvicsosa/homebrew-tap](https://github.com/melvicsosa/homebrew-tap).

## License

[MIT](LICENSE)
