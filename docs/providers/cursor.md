# Cursor — Provider Research Spike

Status: **v1 scope decided — skills only, read-only first.**
Evidence gathered on a real machine (macOS, Cursor installed), 2026-08-16.

## Verified on-disk layout

```
~/.cursor/
├── skills/            user skills — <name>/SKILL.md, same convention as Claude Code
├── skills-cursor/     Cursor's OWN synced skill set + .sync-manifest.json
├── rules/             global rules (*.mdc)
├── plugins/local/     empty on the observed machine
├── mcp.json           MCP server configuration
├── agents/            agent configurations
└── extensions/        VS Code-style extensions
```

Project level: `.cursor/skills/` and `.cursor/rules/` are the documented
conventions. On the observed machine no project carried a `.cursor/`
directory — project-level detection is implemented but flagged as
less field-verified than the Claude Code adapter.

## Key findings

1. **`~/.cursor/skills` mirrors the Claude skill convention** —
   directories containing `SKILL.md`. `ScanSkillsDir` works unchanged
   for both providers.

2. **Cursor has its own sync mechanism.** `~/.cursor/skills-cursor/`
   carries a `.sync-manifest.json` with per-skill `lastSyncedAt`
   timestamps. This set is Cursor's, not the user's: nexo must never
   write there, and does not report it in inspection — managing it
   would fight Cursor's own sync loop.

3. **Plugin surface is not understood well enough to manage.**
   `~/.cursor/plugins/local/` exists but was empty; the install/enable
   mechanism is unconfirmed. Capability declared `plugins: false`
   (spec §23: never assume) until a follow-up spike documents it.

4. **Rules (`*.mdc`) are a real asset class** used in the wild
   (global and per-project). They map to the future `rules` asset type
   (spec §2.1) — out of v1 scope, but the adapter is where they will
   surface.

## v1 adapter scope

| Capability     | State | Basis                                    |
|----------------|-------|------------------------------------------|
| Global skills  | ✅    | verified layout, same SKILL.md convention |
| Project skills | ✅    | documented convention; less field data    |
| Plugins        | ❌    | mechanism unconfirmed — needs spike       |
| MCP            | ❌    | v1 defers MCP for all providers (D12)     |

## Open questions for the next spike

- What does Cursor put in `plugins/local` and how is enablement stored?
- Does Cursor read `.cursor/skills` in projects on current builds, and
  with what precedence vs global skills?
- Are `.mdc` rules importable as a nexo asset type without conversion?
