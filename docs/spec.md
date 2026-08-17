# AI Skills & Plugins Manager --- Product & Technical Specification

## 1. Vision

Build a lightweight, cross-agent package manager for AI development
assets.

The tool should let a developer:

-   Maintain a personal library of Skills, Plugins, and eventually MCP
    servers.
-   Install assets globally or into specific projects/repositories.
-   See what is available in the local library even when it is not
    active anywhere.
-   Inspect where an asset is currently installed.
-   Run the tool from inside a repository and immediately see what that
    repository uses.
-   Install an asset into another repository.
-   Support multiple AI clients through provider adapters.
-   Start with **Claude Code** and **Cursor**.
-   Provide both:
    -   a traditional CLI for scripts, automation, CI, and power users;
    -   an interactive terminal UI for guided management.

The important conceptual separation is:

> **Library != Installation != Activation**

An asset can exist in the local library without being installed in any
project. It can be installed in one or more projects. It can also be
marked as globally active.

------------------------------------------------------------------------

# 2. Core Concepts

## 2.1 Asset

An Asset is the generic object managed by the application.

Initial asset types:

-   Skill
-   Plugin

Future types:

-   MCP Server
-   Agent configuration
-   Rules / instructions
-   Hooks
-   Templates

Every asset should have a stable identifier, version, metadata, source,
and installation information.

Example:

``` yaml
name: wordpress-review
type: skill
version: 1.2.0
description: Reviews WordPress PHP code for security and WordPress coding standards.
```

------------------------------------------------------------------------

## 2.2 Library

The Library is the user's local collection of downloaded/known assets.

An asset can be in the Library without being active anywhere.

Example:

``` text
Library
├── wordpress-review
├── php-security
├── react-performance
└── tailwind-ui
```

The Library is the source from which installations are made.

This distinction is important because deleting an installation should
not necessarily delete the asset from the Library.

------------------------------------------------------------------------

## 2.3 Installation

An Installation represents an asset being applied to a target.

Targets can initially be:

-   Global
-   Project/repository

Later:

-   Specific provider
-   Specific workspace
-   Specific environment

Example:

``` text
wordpress-review
├── Global
├── ~/Projects/client-a
└── ~/Projects/client-b
```

------------------------------------------------------------------------

## 2.4 Global

"Global" means the asset is active through the configured global
location/mechanism for a provider.

It does **not** necessarily mean that the asset is duplicated into every
project.

For example:

``` text
Library:
  wordpress-review

Global:
  wordpress-review

Projects:
  client-a
  client-b
```

The user should see this as one global installation rather than two
project installations.

------------------------------------------------------------------------

## 2.5 Project

A Project is a repository or directory where assets can be installed.

When the CLI is executed inside a recognized project:

``` bash
ai-manager
```

the application should detect the current working directory and offer
project-specific management.

The project can also be explicitly specified:

``` bash
ai-manager project ~/Projects/client-a
```

------------------------------------------------------------------------

# 3. Providers / Adapters

The core application should not contain provider-specific installation
logic.

Instead, use an adapter architecture.

Initial providers:

1.  Claude Code
2.  Cursor

Future providers:

-   Claude Desktop
-   Codex
-   Gemini CLI
-   Windsurf
-   Other agentic coding tools

Conceptually:

``` text
                    ┌──────────────────────┐
                    │      CLI / TUI       │
                    └──────────┬───────────┘
                               │
                    ┌──────────▼───────────┐
                    │     Core Manager     │
                    │                      │
                    │ Library              │
                    │ Registry              │
                    │ Projects             │
                    │ Installations        │
                    │ Versioning            │
                    └──────────┬───────────┘
                               │
                 ┌─────────────┴─────────────┐
                 │                           │
       ┌─────────▼─────────┐       ┌─────────▼─────────┐
       │ Claude Code       │       │ Cursor             │
       │ Adapter           │       │ Adapter            │
       └───────────────────┘       └────────────────────┘
```

The core should ask an adapter things such as:

``` text
Can this provider install this asset type?
Where does global installation live?
Where does project installation live?
How should the asset be enabled?
How should it be removed?
How can installed assets be detected?
```

This keeps provider-specific details out of the main application.

------------------------------------------------------------------------

# 4. Installation Model

An installation should have enough information to answer:

-   What asset?
-   What version?
-   Which provider?
-   Global or project?
-   Which project?
-   Where is it physically installed?
-   When was it installed?
-   Was it installed from the Library or Registry?
-   Is the installation currently valid?

Example internal model:

``` yaml
asset:
  id: wordpress-review
  version: 1.2.0

provider:
  id: claude-code

target:
  type: project
  path: /Users/me/Projects/client-a

installation:
  installed_at: 2026-08-16T19:00:00
  source: library
```

------------------------------------------------------------------------

# 5. Main Interactive UI

Running the CLI without arguments can launch the interactive interface.

Example:

``` bash
ai-manager
```

The first screen should be intentionally simple.

## Main Menu

``` text
AI Manager

> Manage Skills
  Manage Plugins
  Projects
  Library
  Providers
  Settings
  Exit
```

Do not overload the first screen with every technical concept.

The primary workflow is managing Skills and Plugins.

------------------------------------------------------------------------

# 6. Manage Skills

Selecting:

``` text
Manage Skills
```

should show a concise list.

Example:

``` text
Skills

NAME                    STATUS
────────────────────────────────────────
wordpress-review        Global
php-security            Project
react-performance       Library
tailwind-ui             Project
```

The status should communicate the most useful fact without creating
visual noise.

Possible statuses:

-   Global
-   Project
-   Library
-   Not installed
-   Multiple

For an asset that is global, do not list every project where it is
implicitly available.

------------------------------------------------------------------------

# 7. Skill Details

Selecting a skill opens a detail view.

Example:

``` text
wordpress-review

Version:       1.2.0
Type:          Skill
Description:   WordPress PHP security and standards review

Status:        Global

Actions:
  Install in project
  Make global
  View installations
  Update
  Remove installation
  Remove from library
  Back
```

The detail screen is where more information belongs.

This keeps the main list uncluttered.

------------------------------------------------------------------------

# 8. Installation Details

Selecting "View installations":

``` text
wordpress-review

Global
✓ Active globally

Projects
────────────────────────────────
~/Projects/client-a
~/Projects/client-b
~/Projects/client-c
```

If there are no project installations:

``` text
Global
✓ Active globally

Projects
None
```

------------------------------------------------------------------------

# 9. Install Into Another Repository

From an asset's detail screen:

``` text
Install in project
```

should open project selection.

Example:

``` text
Select project

> ~/Projects/client-a
  ~/Projects/client-b
  ~/Projects/client-c
  Enter another path...
```

The user can navigate to a project or manually enter a path.

The application should validate that the path is appropriate before
installing.

------------------------------------------------------------------------

# 10. Running the CLI Inside a Repository

This is an important workflow.

If the user is already inside:

``` bash
cd ~/Projects/client-a
```

and runs:

``` bash
ai-manager
```

the application should recognize the current repository.

The UI can then start with:

``` text
Project: client-a

Skills
Plugins

> Manage project
  Add asset
  View project details
  Back
```

Or a simple project summary:

``` text
client-a

Claude Code
  Skills: 4
  Plugins: 2

Cursor
  Skills: 2
  Plugins: 1

Actions:
> Manage Skills
  Manage Plugins
  Add Asset
  Providers
```

------------------------------------------------------------------------

# 11. Project Asset Detection

The manager should not rely only on its own database.

Provider adapters should be able to inspect a project and discover
existing assets.

This allows the tool to manage repositories that were configured
manually before the manager existed.

For example:

``` bash
cd ~/Projects/existing-project
ai-manager
```

The application should inspect provider-specific configuration and
report what it finds.

Potential states:

``` text
Managed
Detected
Unknown
Broken
```

This distinction is useful.

### Managed

Installed by the manager.

### Detected

Exists in the project but was not installed by the manager.

### Unknown

The provider reports something the manager does not understand.

### Broken

The expected installation/configuration is missing or invalid.

------------------------------------------------------------------------

# 12. Adding an Asset From a Project

Inside a project:

``` text
Add Asset
```

should provide:

``` text
Add Asset

> Skills
  Plugins
```

Then:

``` text
Available Skills

> wordpress-review
  php-security
  react-performance
  tailwind-ui
```

Selecting an asset should ask where to install it if the target is
ambiguous:

``` text
Install wordpress-review

Provider:
> Claude Code
  Cursor

Scope:
> This project
  Global
```

If the project only supports one relevant provider, the provider
selection can be skipped.

------------------------------------------------------------------------

# 13. Library View

The Library is separate from installed assets.

Example:

``` text
Library

NAME                    VERSION     STATUS
──────────────────────────────────────────────
wordpress-review        1.2.0       Global
php-security            2.0.1       Project
react-performance       1.4.0       Not installed
tailwind-ui             3.1.0       Project
```

This allows the user to keep assets available without activating them
everywhere.

For example:

``` text
react-performance
```

can remain downloaded and ready but not active in any project.

------------------------------------------------------------------------

# 14. Registry

The Registry is conceptually different from the Library.

``` text
Registry
    ↓
Download
    ↓
Library
    ↓
Install
    ↓
Project / Global
```

The Registry can eventually be:

-   Public
-   Private
-   Company-hosted
-   Git-based
-   Filesystem-based

For the first version, the Registry does not need to be implemented as a
marketplace.

A local directory or Git repository can be enough.

------------------------------------------------------------------------

# 15. Future Private Company Registry

The architecture should allow a company to have:

``` text
Company Registry
├── Engineering
│   ├── Skills
│   └── Plugins
├── Security
│   └── Skills
└── Internal Tools
    └── Plugins
```

A user could eventually run:

``` bash
ai-manager registry add company https://...
ai-manager search wordpress
ai-manager install company/wordpress-review
```

The registry should support versioning.

------------------------------------------------------------------------

# 16. CLI Mode

The same functionality should be available without the interactive
interface.

Examples:

``` bash
ai-manager list
```

``` bash
ai-manager skill list
```

``` bash
ai-manager skill install wordpress-review
```

``` bash
ai-manager skill install wordpress-review --global
```

``` bash
ai-manager skill install wordpress-review --project ~/Projects/client-a
```

``` bash
ai-manager skill info wordpress-review
```

``` bash
ai-manager skill locations wordpress-review
```

``` bash
ai-manager skill remove wordpress-review --project ~/Projects/client-a
```

``` bash
ai-manager project inspect
```

``` bash
ai-manager project list
```

``` bash
ai-manager plugin list
```

The exact command syntax can be finalized during implementation.

------------------------------------------------------------------------

# 17. Interactive vs CLI

Both interfaces must use the same core services.

Do NOT implement separate business logic for the TUI.

``` text
                 ┌─────────────┐
                 │ Core Engine │
                 └──────┬──────┘
                        │
              ┌─────────┴─────────┐
              │                   │
        ┌─────▼─────┐       ┌─────▼─────┐
        │ CLI       │       │ Interactive│
        │ Commands  │       │ UI        │
        └───────────┘       └───────────┘
```

This prevents behavior from drifting between modes.

------------------------------------------------------------------------

# 18. Global Storage

The application needs its own metadata directory.

Conceptually:

``` text
~/.ai-manager/
├── config/
├── library/
├── registry/
├── installations/
└── cache/
```

The exact filesystem structure should be provider-independent.

The manager should own its metadata without taking ownership of provider
configuration it did not create.

------------------------------------------------------------------------

# 19. Project Metadata

Project-specific manager metadata should preferably be stored in the
repository only when necessary.

Possible structure:

``` text
project/
└── .ai-manager/
    └── manifest.yaml
```

Example:

``` yaml
version: 1

skills:
  - wordpress-review@1.2.0
  - php-security@2.0.1

plugins:
  - company-tools@3.0.0
```

However, provider-native configuration remains the source of truth for
whether an asset is actually active.

The manifest is primarily useful for:

-   Reproducibility
-   Team sharing
-   Version pinning
-   Syncing
-   CI
-   Detecting drift

------------------------------------------------------------------------

# 20. Desired Project Workflow

A developer clones a repository:

``` bash
git clone ...
cd project
```

Then:

``` bash
ai-manager
```

The manager detects the repository.

It can show:

``` text
Project: project

Detected providers:
✓ Claude Code
✓ Cursor

Installed:
Skills: 5
Plugins: 2

> Manage Skills
  Manage Plugins
  Add Asset
  Inspect
```

The developer can add a skill:

``` text
Add Asset
→ Skill
→ wordpress-review
→ Claude Code
→ This project
```

The manager installs it and optionally updates the project manifest.

------------------------------------------------------------------------

# 21. Provider Detection

Providers should be detectable independently.

For example:

``` text
Claude Code
  Installed: Yes
  Project configuration: Detected

Cursor
  Installed: Yes
  Project configuration: Detected
```

If a provider is not installed locally, the manager should not
necessarily fail.

It can simply report:

``` text
Claude Code
  Not detected
```

Provider support should remain modular.

------------------------------------------------------------------------

# 22. Initial Providers

## Claude Code Adapter

Responsibilities:

-   Detect Claude Code.
-   Detect project configuration.
-   Detect available skills/plugins.
-   Install assets.
-   Remove assets.
-   Handle global installations.
-   Handle project installations.
-   Report installation status.

## Cursor Adapter

Responsibilities:

-   Detect Cursor.
-   Detect project configuration.
-   Detect supported assets.
-   Install assets.
-   Remove assets.
-   Handle global/project scope where supported.
-   Report installation status.

The adapter should explicitly declare capabilities because not every
provider supports the same concepts.

Example:

``` yaml
provider: cursor

capabilities:
  global_skills: true
  project_skills: true
  plugins: true
  mcp: true
```

------------------------------------------------------------------------

# 23. Capability Model

Never assume that every provider supports every feature.

The UI should query capabilities.

For example:

``` text
Claude Code
  Global Skills       ✓
  Project Skills      ✓
  Plugins             ✓
  MCP                 ✓

Cursor
  Global Skills       ?
  Project Skills      ✓
  Plugins             ?
  MCP                 ✓
```

If a capability is unsupported, the UI should hide or disable the
relevant action.

------------------------------------------------------------------------

# 24. Package Manifest

Each managed asset should have a manifest.

Example:

``` yaml
id: wordpress-review
name: WordPress Review
type: skill
version: 1.2.0

description: >
  Reviews WordPress PHP code for security,
  coding standards, performance and common mistakes.

author:
  name: My Name

compatibility:
  providers:
    - claude-code
    - cursor

files:
  - source: SKILL.md
```

Plugins may require additional metadata.

Example:

``` yaml
id: company-tools
name: Company Tools
type: plugin
version: 3.0.0

compatibility:
  providers:
    - claude-code

dependencies:
  - php-security@>=2.0.0
```

------------------------------------------------------------------------

# 25. Dependencies

Keep dependencies simple in version one.

A Plugin may depend on Skills.

Example:

``` text
company-wordpress-plugin
    ├── wordpress-review
    └── php-security
```

The manager should resolve dependencies before installation.

Do not attempt to become npm.

Only implement the minimum dependency functionality needed for AI
assets.

------------------------------------------------------------------------

# 26. Versioning

Use semantic versioning where possible:

``` text
1.0.0
1.1.0
2.0.0
```

Allow version pinning:

``` yaml
wordpress-review: 1.2.0
```

and ranges where useful:

``` yaml
wordpress-review: ^1.2.0
```

Version handling should be centralized in the core.

------------------------------------------------------------------------

# 27. Go Architecture

Go is a strong choice because the goal is a lightweight, single-binary
CLI.

Suggested architecture:

``` text
cmd/
  ai-manager/

internal/
  core/
  assets/
  library/
  registry/
  installation/
  project/
  providers/
    claude_code/
    cursor/
  config/
  ui/
  cli/
```

Possible interfaces:

``` go
type Provider interface {
    ID() string
    Name() string
    Detect() DetectionResult
    Capabilities() Capabilities
    InspectProject(path string) (ProjectState, error)
    Install(asset Asset, target InstallTarget) error
    Uninstall(asset Asset, target InstallTarget) error
}
```

The exact API should evolve during implementation.

------------------------------------------------------------------------

# 28. Dependency Philosophy

The CLI should intentionally have few dependencies.

Goals:

-   Single binary.
-   No runtime required.
-   No Node.js requirement.
-   No Python requirement.
-   Minimal external services.
-   Works offline for already-installed assets.
-   Fast startup.

Go is particularly suitable for this.

For the interactive UI, use a mature Go terminal UI library only if it
provides clear value. Avoid adding a large framework just for a simple
menu.

------------------------------------------------------------------------

# 29. Distribution

## macOS

Primary distribution:

``` text
Homebrew
```

Potential installation:

``` bash
brew install ai-manager
```

Eventually:

``` bash
brew tap company/ai-tools
brew install ai-manager
```

------------------------------------------------------------------------

## Linux

Support:

-   Homebrew/Linuxbrew
-   Direct binary releases
-   Eventually native packages if justified

Example:

``` bash
brew install ai-manager
```

------------------------------------------------------------------------

## Windows

Windows can be added after macOS/Linux.

Potential distribution options:

-   Scoop
-   winget
-   Chocolatey
-   Direct executable

Because the core is Go, Windows does not require architectural changes.

The same binary-oriented design can support Windows naturally.

------------------------------------------------------------------------

# 30. Naming

The project should have a short CLI name.

Potential conceptual names:

``` text
aipm
aim
agentpm
skillman
skillctl
agentctl
```

Avoid choosing the name before checking package, GitHub, Homebrew and
trademark availability.

------------------------------------------------------------------------

# 31. MVP

The first version should NOT try to solve everything.

### MVP should contain

-   Go CLI
-   Single binary
-   Interactive mode
-   Non-interactive CLI mode
-   Library
-   Skills
-   Plugins
-   Global installations
-   Project installations
-   Project detection
-   Claude Code adapter
-   Cursor adapter
-   Local registry
-   Basic manifests
-   Installation tracking
-   Inspect project
-   Install/remove assets
-   List assets
-   Asset detail view

### Explicitly defer

-   Public marketplace
-   Accounts
-   Cloud synchronization
-   Billing
-   Team administration
-   Analytics
-   Web dashboard
-   Complex dependency resolution
-   Many providers
-   Remote authentication
-   Enterprise features

------------------------------------------------------------------------

# 32. Suggested MVP Screens

## Screen 1 --- Main

``` text
AI Manager

> Manage Skills
  Manage Plugins
  Projects
  Library
  Providers
  Settings
  Exit
```

## Screen 2 --- Skills

``` text
Skills

> wordpress-review       Global
  php-security           Project
  react-performance      Library
```

## Screen 3 --- Skill Detail

``` text
wordpress-review

Version: 1.2.0
Status: Global

> Install in project
  View installations
  Make global
  Update
  Remove
  Back
```

## Screen 4 --- Project

``` text
Project: client-a

Claude Code
  Skills: 4
  Plugins: 2

Cursor
  Skills: 2
  Plugins: 1

> Manage Skills
  Manage Plugins
  Add Asset
  Inspect
```

------------------------------------------------------------------------

# 33. Important UX Principle

Do not expose all technical concepts at once.

The user should not have to understand:

-   Registry
-   Library
-   Installation
-   Provider
-   Adapter
-   Manifest
-   Scope
-   Capability

just to install a Skill.

The UI should progressively reveal complexity.

Simple:

``` text
Install Skill
```

Then:

``` text
Where?

> This project
  Global
  Another project
```

Then, only if necessary:

``` text
Which provider?

> Claude Code
  Cursor
```

------------------------------------------------------------------------

# 34. Recommended Mental Model

The simplest way to understand the whole product is:

``` text
                 REGISTRY
                    │
                    ▼
                 LIBRARY
                    │
          ┌─────────┴─────────┐
          │                   │
          ▼                   ▼
       GLOBAL              PROJECT
                              │
                     ┌────────┴────────┐
                     ▼                 ▼
                CLAUDE CODE         CURSOR
```

The manager controls the flow between these layers.

------------------------------------------------------------------------

# 35. Long-Term Product Direction

If the personal version works well, the same architecture can evolve
into a company product.

Possible future capabilities:

``` text
Personal Registry
      ↓
Private Registry
      ↓
Team Registry
      ↓
Enterprise Registry
```

Then:

-   Organization accounts
-   Private packages
-   Permissions
-   Approved Skills
-   Version policies
-   Security scanning
-   Team manifests
-   Shared configurations
-   Audit history
-   Remote synchronization
-   Web UI
-   Package publishing
-   CI integration

The CLI remains the primary developer interface.

------------------------------------------------------------------------

# 36. Product Thesis

The core product is not "a marketplace for Skills."

The stronger concept is:

> **A package manager and environment manager for AI development
> tools.**

It should solve the problem:

> "I have Skills, Plugins and eventually MCPs, but every AI tool stores
> and manages them differently. I want one place to manage my collection
> and decide exactly where each asset is active."

That distinction is important because it makes the product useful even
without a marketplace.

------------------------------------------------------------------------

# 37. First Implementation Sequence

Recommended order:

1.  Create Go CLI.
2.  Define Asset and Installation models.
3.  Implement local Library.
4.  Implement global/project scope.
5.  Implement project detection.
6.  Implement Claude Code adapter.
7.  Implement Cursor adapter.
8.  Implement basic CLI commands.
9.  Add interactive UI.
10. Add local registry.
11. Add manifests.
12. Add versioning.
13. Add dependency handling.
14. Package for Homebrew.
15. Add Linux release binaries.
16. Add Windows distribution later.

This keeps the first implementation focused and avoids prematurely
building the marketplace/cloud layer.

------------------------------------------------------------------------

# 38. Final Architecture

``` text
                         ┌──────────────────────┐
                         │     User / Dev       │
                         └──────────┬───────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
             CLI Commands                    Interactive UI
                    │                               │
                    └───────────────┬───────────────┘
                                    │
                         ┌──────────▼──────────┐
                         │      Core Engine    │
                         │                     │
                         │ Assets              │
                         │ Library             │
                         │ Registry            │
                         │ Projects            │
                         │ Installations       │
                         │ Versioning          │
                         │ Config              │
                         └──────────┬──────────┘
                                    │
                         Provider Adapter API
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
             Claude Code                         Cursor
                    │                               │
                    └───────────────┬───────────────┘
                                    │
                       Provider-specific configs
                                    │
                         ┌──────────▼──────────┐
                         │     AI Tooling      │
                         └─────────────────────┘
```

## Bottom line

Build the first version as a **local, provider-agnostic package
manager** rather than a marketplace.

The three most important objects are:

1.  **Library** --- what I have downloaded/available.
2.  **Installation** --- where I have activated it.
3.  **Provider** --- which AI tool understands the installation.

And the two most important scopes are:

-   **Global**
-   **Project**

Start with **Claude Code + Cursor**, a Go single binary, Homebrew for
macOS/Linux, an interactive terminal UI, and a full CLI API underneath
it.

That gives you a clean foundation that can later support Claude Desktop,
Codex, Gemini CLI, MCPs, private company registries, and eventually a
commercial product without rewriting the core.
