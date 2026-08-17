# nexo — Implementation Plan

> Name: `nexo`. Repo: `melvicsosa/nexo`. Companion doc: `docs/spec.md` (product spec).
> **Status (2026-08-16): Phases 0–6 SHIPPED** — v0.5.0 on the tap. Adapters: Claude Code,
> Cursor, Codex. Deferred within v1 scope: Cursor/Codex plugin management (spike docs name
> the blockers), MCP assets (modeled, D12), dirty-git-tree warning (needs an exec port).
> This plan resolves every blocking gap found in the spec review and defines the build sequence.

---

## 1. Resolved Decisions

These were the blockers. Each one is now a decision, not an open question.

| # | Gap | Decision |
|---|-----|----------|
| D1 | Plugins ≠ file copies | Two install strategies in the core: **Materialize** (write files — skills) and **Reference** (mutate provider config — Claude plugins). The Library doubles as a **local marketplace**: nexo generates `.claude-plugin/marketplace.json` from it, so Claude Code installs plugins natively from nexo's Library. We never fight the provider's own mechanism. |
| D2 | Skills have no version field | Identity = **content hash** (sha256 of the asset file tree), always. Version = optional metadata in a nexo **sidecar manifest** (`library/<asset>/.nexo.yaml`). `unversioned` is a first-class state, never an error. Drift detection and `Broken` state are decided by hash, not version. |
| D3 | Copy vs symlink | **Global = symlink** (one source of truth on your machine, updates are automatic). **Project = copy** (repo must be self-contained: committable, travels to teammates, works in CI). This is why the project manifest (spec §19) exists — it records what was copied. |
| D4 | Global scope | **Kept.** All three target providers have a native global location (`~/.claude/skills`, `~/.cursor/skills`, `~/.codex/`). Dropping it would misrepresent how these tools actually work. |
| D5 | Git safety | Project installs touch tracked files in someone's repo. Rules: (1) never modify `.gitignore`; (2) if the target file exists with **different content** → abort with a diff summary, require `--force`; (3) identical content → no-op, mark Managed; (4) warn (not fail) on dirty working tree. |
| D6 | Uninstall safety | nexo only deletes what it installed, **verified by hash**. `Detected` assets (pre-existing, not installed by nexo) are never deleted — the offered paths are `nexo adopt` (bring into Library) or explicit `--force`. |
| D7 | Filesystem port | No adapter touches the OS directly. `FS` + `Paths` (home dir, config dir) are injected interfaces. Every adapter is testable against an in-memory FS (`afero` / `fstest.MapFS`). Non-negotiable for a tool whose job is mutating the user's home. |
| D8 | Atomicity | Every install/remove is a transaction: stage to temp → verify → rename into place → journal. On any failure, roll back from the journal. Config mutations (JSON/TOML) are parse → edit → atomic write, never string templating. |
| D9 | Dependencies (spec §25) | **Deferred.** Claude plugins already bundle their skills. No dependency resolver in v1. |
| D10 | Asset identity / namespacing | Full ID is `source/name` (`local/wordpress-review`, `company/wordpress-review`) from day one. Display short name when unambiguous. Avoids the rename migration later. |
| D11 | Distribution first, not last | Release pipeline (GoReleaser → GitHub Release → homebrew-tap) ships in Phase 0 with a stub binary. Every phase after that is installable. Pipeline is already proven in `video-optimizer` — copy and adapt. |
| D12 | MCP servers | Not in MVP, but the `Asset.Type` enum and capability model include `mcp` from day one so adding it is additive, not structural. |

---

## 2. Architecture (built to be forked and extended)

The explicit design goal: **adding a provider must touch exactly one new package plus one registration line.** That is the fork/extension surface.

```
cmd/nexo/                     main; wires everything (the ONLY place with os.* access)

internal/
  domain/                     pure types, zero deps: Asset, Installation,
                              InstallStrategy, Capability, HealthState
  ports/                      interfaces the core needs: FS, Paths, Clock, Journal
  core/
    library/                  Library service (sidecar manifests, hashing)
    install/                  transactional install/remove engine
    inspect/                  detection + health (Managed/Detected/Unknown/Broken)
    registry/                 registry client (git/dir based, index.json)
  providers/
    provider.go               the Provider interface + capability declaration
    registry.go               provider registration (the one line adapters add)
    claudecode/
    cursor/
    codex/                    ← phase 6, but the seat exists from day one
  config/                     ~/.nexo/ store, schema_version, migrations
  cli/                        cobra commands, every one supports --json
  tui/                        bubbletea app — calls the SAME core services
```

### Provider contract

```go
type Provider interface {
    ID() string
    Detect(fs ports.FS, paths ports.Paths) DetectionResult
    Capabilities() Capabilities              // never assume; UI queries this
    InspectGlobal() ([]FoundAsset, error)
    InspectProject(path string) ([]FoundAsset, error)
    Plan(asset Asset, target Target) (InstallPlan, error)  // returns steps, does not execute
    // NOTE: adapters PLAN, the core EXECUTES. This is what makes
    // transactions, rollback, dry-run and --json uniform across providers.
}
```

Key idea: adapters **describe** installations (which files, which config edits); the core **executes** them through the transaction engine. An adapter author writes zero transactional code — that's what keeps forks/contributions cheap and safe.

### Capability declarations (initial, to be verified per adapter phase)

```yaml
claude-code: { global_skills: yes, project_skills: yes, plugins: yes(reference), mcp: later }
cursor:      { global_skills: yes, project_skills: yes, plugins: research-spike, mcp: later }
codex:       { research-spike in phase 6 — known surface: ~/.codex/, AGENTS.md, prompts/ }
```

### Update / release strategy (fork-friendly)

- GoReleaser + tag push → binaries + Homebrew cask, same as `vopt`. Fine-grained PAT `HOMEBREW_TAP_TOKEN` per-repo secret.
- `nexo version --check` compares against GitHub latest release (no auto-update in v1; brew handles it).
- `~/.nexo/` carries `schema_version: 1` from the first write; `config/migrations.go` exists from day one even if empty.
- `docs/ADDING_A_PROVIDER.md` written in Phase 2 while writing the second adapter — the guide is extracted from real experience, not invented.

---

## 3. Build Phases

### Phase 0 — Repo + living pipeline
- [ ] Create `melvicsosa/nexo`, `go.mod github.com/melvicsosa/nexo`
- [ ] Copy from `video-optimizer`: `.goreleaser.yaml` (rename, drop ffmpeg dep), `release.yml`, `ci.yml`, `Makefile`, `LICENSE`, install scripts
- [ ] Add `HOMEBREW_TAP_TOKEN` secret to the new repo (per-repo, not inherited)
- [ ] Stub binary: `nexo version` → tag `v0.0.1` → verify `brew install melvicsosa/tap/nexo` end-to-end

### Phase 1 — Domain + ports (pure Go, fully tested)
- [ ] `domain`: Asset (namespaced ID, type, optional version, content hash), Installation, Target, HealthState
- [ ] `ports`: FS, Paths, Clock, Journal + afero-backed and in-memory implementations
- [ ] Transaction engine: stage → verify → commit → rollback (table-driven tests, failure injection)
- [ ] `~/.nexo/` store with `schema_version`

### Phase 2 — Read-only detection (first real value)
- [ ] Claude Code adapter: Detect, InspectGlobal (`~/.claude/skills`, `installed_plugins.json`, `settings.json`), InspectProject (`.claude/`)
- [ ] Cursor **research spike** (timeboxed): document `~/.cursor/skills` vs `skills-cursor` + `.sync-manifest.json`, `plugins/local`, `rules/*.mdc` → `docs/providers/cursor.md`; then read-only adapter
- [ ] `nexo list`, `nexo project inspect`, `--json` everywhere
- [ ] Write `docs/ADDING_A_PROVIDER.md` from the experience of adapter #2
- [ ] **Ship it**: `cd repo && nexo` showing real state is already a useful tool

### Phase 3 — Library + skill install/remove
- [ ] Library service: add/remove/list, sidecar manifests, tree hashing
- [ ] `nexo adopt` — promote Detected assets into the Library (the natural on-ramp)
- [ ] Install skills: global (symlink) + project (copy), through the transaction engine, honoring D5/D6
- [ ] `nexo doctor` — health report + guided repair for Broken states
- [ ] Project manifest `.nexo/manifest.yaml` (reproducibility; provider config stays the runtime source of truth)

### Phase 4 — Claude plugins (Reference strategy)
- [ ] Library-as-marketplace: generate `.claude-plugin/marketplace.json`
- [ ] Safe mutation of `settings.json` `enabledPlugins` (parse/edit/atomic-write)
- [ ] Detection of plugin enable/disable state in inspect + doctor

### Phase 5 — TUI
- [ ] Bubbletea (already proven in `vopt`; `go-testing` skill covers teatest patterns)
- [ ] Screens from spec §32; progressive disclosure per §33; zero business logic in the TUI — core services only

### Phase 6 — Registry + Codex
- [ ] Registry: git-repo/directory with `index.json`; `nexo registry add`, `nexo search`, `nexo install <source>/<name>`
- [ ] Codex research spike → `docs/providers/codex.md` → adapter (surface to verify at build time: `~/.codex/config.toml`, `AGENTS.md`, `prompts/`, skills support)
- [ ] Windows release artifacts (GoReleaser already builds them; validate symlink fallback → copy on Windows)

---

## 4. Out of scope for v1 (explicitly)

Marketplace/accounts/cloud/billing/analytics/web UI (spec §31), dependency resolution (D9), MCP management (D12 — modeled, not implemented), auto-update daemon.
