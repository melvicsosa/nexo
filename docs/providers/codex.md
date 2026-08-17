# Codex — Provider Research Spike

Status: **v1 scope decided — skills only.**
Evidence gathered on a real machine (macOS, Codex CLI installed via
Homebrew), 2026-08-16.

## Verified on-disk layout

```
~/.codex/
├── skills/            <name>/SKILL.md — SAME convention as Claude Code/Cursor (19 observed)
├── plugins/cache/     plugin content cache
├── config.toml        model config, MCP servers, plugin enablement
├── agents.md          global agent instructions
├── auth.json          credentials — nexo must never read or touch this
└── sessions/…         runtime state
```

Plugin enablement in `config.toml` (TOML, not JSON):

```toml
[plugins."vercel-plugin@plugins-cli"]
enabled = true
```

No project-level `.codex/` directory was found in the wild on the
observed machine; `<project>/.codex/skills` follows the same convention
per ecosystem documentation and is supported by the adapter with that
caveat.

## Key findings

1. **The SKILL.md convention has become a de-facto standard** —
   Claude Code, Cursor and Codex all read `skills/<name>/SKILL.md`.
   `ScanSkillsDir` and the skill installer work unchanged; the Codex
   adapter is ~100 lines.

2. **Plugin state is TOML.** Mutating it safely requires a
   round-tripping TOML parser (comments and formatting must survive) —
   that is a new dependency and a new risk surface. Declared
   `plugins: false` until it can be done right; a future
   `PluginConfigurator` for Codex is the natural follow-up.

3. **`auth.json` sits next to everything.** The adapter only ever
   touches `skills/`; inspection never opens auth or session files.

## v1 adapter scope

| Capability     | State | Basis                                        |
|----------------|-------|----------------------------------------------|
| Global skills  | ✅    | verified layout, 19 skills on a real machine |
| Project skills | ✅    | documented convention; not seen locally yet  |
| Plugins        | ❌    | TOML round-trip needs a dependency — deferred |
| MCP            | ❌    | v1 defers MCP for all providers (D12)        |

## Open questions for the next spike

- TOML round-trip library choice for `config.toml` plugin toggling.
- Do Codex marketplaces mirror Claude's model closely enough for the
  library-as-marketplace approach to extend to it?
