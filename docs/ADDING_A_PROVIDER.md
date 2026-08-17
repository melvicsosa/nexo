# Adding a Provider to nexo

This guide was extracted from writing the second adapter (Cursor) right
after the first (Claude Code). Total extension surface: **one new
package + one registration line.**

## 1. Start with a research spike, not code

Before writing anything, document the provider's real on-disk layout in
`docs/providers/<id>.md`:

- Where do global assets live? Project assets?
- Is installation file-based (Materialize) or config-based (Reference)?
- What does the provider own that nexo must never touch? (Cursor's
  `skills-cursor/` sync set is the canonical example.)
- What can't you confirm? Declare that capability `false` — the UI
  hides what a provider doesn't declare (spec §23). An honest ❌ beats
  a guessed ✅ that corrupts someone's setup.

Ground the spike in a real machine when possible. Assumptions are how
adapters destroy user configuration.

## 2. Create the package

```
internal/providers/<id>/<id>.go       the adapter
internal/providers/<id>/<id>_test.go  tests against MemFS fixtures
```

Implement `providers.Provider`:

```go
type Adapter struct {
    fs    ports.FS
    paths ports.Paths
}

func New(fsys ports.FS, paths ports.Paths) *Adapter
func (a *Adapter) ID() string                // stable, kebab-case
func (a *Adapter) Name() string              // display name
func (a *Adapter) Detect() providers.DetectionResult
func (a *Adapter) Capabilities() providers.Capabilities
func (a *Adapter) InspectGlobal() ([]providers.FoundAsset, error)
func (a *Adapter) InspectProject(path string) ([]providers.FoundAsset, error)
```

Rules that keep adapters safe and cheap:

- **Never touch the OS.** Use only the injected `ports.FS`/`ports.Paths`.
  If you typed `os.`, stop.
- **Reuse `providers.ScanSkillsDir`** for SKILL.md-convention
  directories; it already handles symlinked skill dirs (real projects
  symlink them) and treats a missing dir as zero assets.
- **Missing files are not errors; corrupt files are.** A machine
  without the provider configured is normal. A provider file that
  exists but doesn't parse must surface loudly — that inconsistency is
  what inspection is for.
- **Report inconsistencies instead of hiding them** (e.g. a plugin
  enabled in settings but not installed).
- **No transactional code.** Adapters plan, the core executes (plan
  D8). In Phase 3+ adapters return `tx.Step` plans built from the
  sealed step vocabulary; you never write undo/rollback logic.

## 3. Register it

One line in `internal/providers/registry/registry.go`:

```go
func All(fsys ports.FS, paths ports.Paths) []providers.Provider {
    return []providers.Provider{
        claudecode.New(fsys, paths),
        cursor.New(fsys, paths),
        yourprovider.New(fsys, paths),   // ← this
    }
}
```

`nexo providers`, `nexo list` and `nexo project inspect` pick it up
with no further changes.

## 4. Test against MemFS fixtures

Build the provider's layout in `portstest.NewMemFS()` and assert on
inspection results. Minimum bar (see `claudecode_test.go`):

- Detect: present and absent machines
- InspectGlobal: assets found, non-assets excluded, hashes non-empty
- Missing config files → zero assets, no error
- Corrupt config files → error, not silence
- InspectProject: including a symlinked asset dir

No test may touch the real home directory. If your adapter needs an FS
operation `ports.FS` lacks, extend the port and `MemFS` together —
don't reach around the interface.
