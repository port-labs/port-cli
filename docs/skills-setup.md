# Setting up AI skill hooks with Port CLI

The `port skills` commands let you automatically load skills from your Port
organization into your local AI coding tools at the start of every session.

Supported tools: **Agents (cross-platform)**, **Cursor**, **Claude Code**, **Gemini CLI**,
**OpenAI Codex**, **Windsurf**, and **GitHub Copilot**.

**Agents** writes skills under `~/.agents/skills/` and `<project>/.agents/skills/`
(the [agentskills.io](https://agentskills.io/client-implementation/adding-skills-support)
`.agents/skills/` convention). No session hooks are installed for Agents — skills only.

## Prerequisites

- `port` CLI installed (`npm install -g @port-experimental/port-cli` or download from [GitHub Releases](https://github.com/port-experimental/port-cli/releases))
- A Port account with skills that have an **active version** set in Port
- At least one supported AI tool installed

---

## Step 1 — Authenticate

Choose one of the following.

### Interactive (browser login)

```sh
port auth login
# optional: port auth login --region eu|us
```

This opens a browser window for SSO and stores a token in `~/.port/creds.json`.

### Non-interactive (machine credentials)

For scripts and CI, use application `client_id` / `client_secret` (same as `port export`,
`port api`, and other commands). See [Non-interactive and CI usage](../README.md#non-interactive-and-ci-usage).

```sh
export PORT_CLIENT_ID="your-client-id"
export PORT_CLIENT_SECRET="your-client-secret"
export PORT_API_URL="https://api.getport.io/v1"

# Local stack example:
# export PORT_API_URL="http://localhost:3000/v1"
```

Equivalent options: `~/.port/config.yaml` (`client_id` / `client_secret` /
`api_url` per org), `~/.port/.env` (same `PORT_*` variable names), or global
flags `--client-id`, `--client-secret`, `--api-url` on each command.

Use your Port **application** Client ID and Secret, not the organization ID.

---

## Step 2 — Configure tools and sync skills

Run the one-time setup command:

```sh
port skills init
```

You will be asked two questions:

1. **Which AI tools should receive synced skills** — an interactive multi-select lists
   all supported tools. Skills are written under each tool’s `skills/` directory
   (e.g. `~/.cursor/skills/`, `~/.agents/skills/`). **GitHub Copilot is
   repo-scoped:** skills go under `<repo>/.github/skills/`. Run init from the
   repository root when you select Copilot.
2. **Which skills to sync** — the CLI fetches all skill groups from Port
   (`GET /skills?teams_default=false&exclude=files&exclude=internal`). Groups owned
   by your Port teams are **pre-selected**; adjust the list to opt in or out.
   Ungrouped skills are chosen in a separate step. Your choices are saved as
   `include_groups` / `exclude_groups` adjustments on top of the team default used
   during `port skills sync`.

After confirming your selection, the CLI saves your tool targets and skill selection
to `~/.port/config.yaml`. Run `port skills sync` to download skills to disk.

### Optional: session-start hooks

By default, init does **not** change `hooks.json` or `settings.json`. To install
hooks that run `port skills sync --quiet --org <org>` at the start of each AI session
(the org resolved for that init — `--org` or your `default_org`):

```sh
port skills init --install-hooks
```

Interactive init accepts the same flag. For Cursor, Claude Code, Gemini CLI,
OpenAI Codex, and Windsurf, hooks are global (e.g. `~/.cursor/hooks.json`).
**GitHub Copilot** hooks are repo-scoped: `<repo>/.github/hooks/hooks.json`.

### Non-interactive init (CI / scripts)

Use the same `--tool` names as sync. Repeat `--tool` for multiple tools:

```sh
# Single tool
port skills init --tool Cursor --select-all-groups --select-all-ungrouped

# Multiple tools
port skills init --tool Cursor --tool "Claude Code" --select-all-groups --select-all-ungrouped
port skills init --tool "Agents (cross-platform)" --tool Cursor --tool Windsurf --select-all-groups

# With hooks (then run port skills sync to write files)
port skills init --tool Cursor --tool "Claude Code" --install-hooks --select-all-groups --select-all-ungrouped
```

All of `init`, `add`, and `remove` work without a TTY for scripts and CI. Pass explicit
flags or `-y` / `--yes` (globally as `port -y …` also works). `-y` selects every option
in each step — the same as checking every box in the interactive prompts:

- **init** — all AI tools, all skill groups, and all ungrouped skills
- **add** — every group, skill, and tool not already in your selection
- **remove** — every group, skill, and configured tool in your selection (no confirmation)

---

## Step 3 — Sync skills to disk

After init, download skills to your configured tool directories:

```sh
port skills sync
```

If you used `--install-hooks`, starting a new AI session runs `port skills sync --quiet --org <org>`
(pinned to the org from init) automatically in the background before the assistant starts.

When project-scoped skills are written inside a git repository, sync checks whether
the generated `<tool>/skills/` path is already ignored. If it is not, Port CLI
adds that path to the repository `.gitignore` so synced skills are not committed
accidentally. To skip this best-effort update for a run, pass `--no-gitignore`.
If the `.gitignore` check or update fails, sync still completes and prints a warning
unless `--quiet` is set.

### Sync without init

You can sync without running init first by passing `--tool`. Runtime flags apply to
this sync only and are **not** saved to `~/.port/config.yaml`.

Supported `--tool` values (must match exactly):

| Tool | `--tool` value | Skills directory |
| ---- | -------------- | ---------------- |
| Agents (cross-platform) | `"Agents (cross-platform)"` | `~/.agents/skills/` and `<project>/.agents/skills/` |
| Cursor | `Cursor` | `~/.cursor/skills/` |
| Claude Code | `"Claude Code"` | `~/.claude/skills/` |
| Gemini CLI | `"Gemini CLI"` | `~/.gemini/skills/` |
| OpenAI Codex | `"OpenAI Codex"` | `~/.codex/skills/` |
| Windsurf | `Windsurf` | `~/.codeium/windsurf/skills/` |
| GitHub Copilot | `"GitHub Copilot"` | `<repo>/.github/skills/` (run from repository root) |

**One tool:**

```sh
port skills sync --tool "Agents (cross-platform)"
port skills sync --tool Cursor
port skills sync --tool "Claude Code"
port skills sync --tool "Gemini CLI"
port skills sync --tool "OpenAI Codex"
port skills sync --tool Windsurf
port skills sync --tool "GitHub Copilot"   # from your repo root
```

**Multiple tools** (repeat `--tool` for each):

```sh
# Two tools
port skills sync --tool Cursor --tool "Claude Code"

# Three tools
port skills sync --tool Cursor --tool "Gemini CLI" --tool "OpenAI Codex"

# Mix Agents with IDE tools
port skills sync --tool "Agents (cross-platform)" --tool Cursor --tool Windsurf

# Include GitHub Copilot (run from the repository root)
port skills sync --tool Cursor --tool "Claude Code" --tool "GitHub Copilot"

# All global tools at once
port skills sync \
  --tool "Agents (cross-platform)" \
  --tool Cursor \
  --tool "Claude Code" \
  --tool "Gemini CLI" \
  --tool "OpenAI Codex" \
  --tool Windsurf
```

### One-off skill selection

Pass groups or skills for this sync without changing saved config:

```sh
port skills sync --tool Cursor --group operations --group security
port skills sync --tool Cursor --skill integrations-overview
port skills sync --tool Cursor --select-all-groups --select-all-ungrouped
```

### Catalog filters

```sh
# Include Port built-in registry skills (excluded by default)
port skills sync --include-internal

# Omit legacy blueprint skills
port skills sync --exclude-legacy

# Combine with a runtime tool target
port skills sync --tool Cursor --include-internal --exclude-legacy
```

### Adjust group defaults for one run

When using team group defaults, add or exclude groups for this sync only:

```sh
port skills sync --include-group operations --exclude-group legacy
```

### Hooks + sync in one command

Install session-start hooks and sync in a single step (`--install-hooks` requires `--tool`):

```sh
port skills sync --tool Cursor --install-hooks
```

---

## Previewing skills without syncing

`port skills list` shows exactly what `port skills sync` would download — it uses the same filters as sync but writes nothing to disk.

```sh
# Preview using saved config (same result as sync, minus the file writes)
port skills list

# Show all skills in Port, ignoring saved filters and team ownership
port skills list --all

# Include skills that have no active version set
port skills list --include-unpublished

# Machine-readable JSON (grouped response)
port skills list --json

# Combine flags
port skills list --all --include-unpublished --json
```

`--all` passes `teams_default=false` to the API so groups are returned regardless of which team owns them, and clears any saved `include_group` / `exclude_group` filters for that one call. It is equivalent to what the init catalog shows before you make any selection.

---

## Updating your skill selection

### Incremental add and remove

Use `add` and `remove` to change your saved selection without a full re-prompt. Both commands update `~/.port/config.yaml` and re-sync skills to disk.

**Non-interactive** — pass flags and/or positional skill identifiers (positional args are equivalent to `--skill`):

```sh
# Add a group, skill, or AI tool
port skills add --group security
port skills add --skill integrations-overview
port skills add integrations-overview
port skills add --skill my-skill --tool Cursor
port skills add -y

# Remove a group, skill, or AI tool (skips confirmation prompts)
port skills remove --group legacy
port skills remove --skill integrations-overview
port skills remove integrations-overview
port skills remove --tool Windsurf
port skills remove -y
```

**Interactive** — run without flags to pick from items not already in your selection:

```sh
port skills add
port skills remove
```

For a one-off sync without changing saved config, use `port skills sync --tool Cursor --skill <id>` instead.

### Replace entire selection

To change which skills and groups are synced (without reinstalling hooks):

```sh
port skills select
```

This re-presents the same group/skill prompts as `port skills init`. Your new selection replaces the previous one and skills are re-synced.

Non-interactive example:

```sh
port skills select --select-all-groups --select-all-ungrouped
```

Or pick explicit groups:

```sh
port skills select --group operations --group security
```

Built-in registry skills from the ai-skills package are **excluded by default**. Opt in with:

```sh
port skills sync --include-internal
```

To omit legacy blueprint `skill` entities as well:

```sh
port skills sync --exclude-legacy
```

You can also re-run full init (including hook install):

```sh
port skills init
```

---

## Manual sync (saved config)

To refresh skills using the targets and selection from `port skills init`:

```sh
port skills sync
```

Without init, you must pass `--tool` (see [Sync without init](#sync-without-init) above).
Run `port skills init` when you want to persist tool directories, selection, and optional hooks.

---

## Command reference


| Command                     | Description                                                                            |
| --------------------------- | -------------------------------------------------------------------------------------- |
| `port skills init`          | Choose tools and skill selection; save to config (hooks optional) |
| `port skills sync`          | Download skills to disk (saved config, or `--tool` for one-off sync) |
| `port skills select`        | Change skill/group selection and re-sync (no hook changes); same selection flags as init |
| `port skills add`           | Add groups, skills, or tools to saved selection and re-sync; non-interactive with flags or positional skill IDs |
| `port skills remove`        | Remove groups, skills, or tools from saved selection and re-sync; non-interactive with flags or positional skill IDs |
| `port skills init --install-hooks` | Also write session-start hooks for selected tools |
| `port skills list`          | Preview what `port skills sync` would download — same query and filters as sync, but nothing is written to disk. `--all` bypasses saved filters and shows every skill regardless of team ownership. `--include-unpublished` includes skills without an active version. `--json` for machine output. |
| `port skills search <query>` | Search skills by identifier or title substring (`GET /skills/search`); `--json`, `--limit`, `--published-only` |
| `port skills upload <dir>`  | Upload skill(s) from a folder or bundle; requires the experimental versioned skills data model |
| `port skills publish <id>`  | Make the latest uploaded version active; requires the experimental versioned skills data model |
| `port skills unpublish <id>` | Clear the active version; requires the experimental versioned skills data model |
| `port skills --org NAME`    | Use a specific organization from config (default org is not hard-coded to `production`) |
| `port skills clear`         | Delete locally synced skill files from AI tool dirs (hooks remain; with confirmation)  |
| `port skills clear --force` | Delete skill files without confirmation prompt                                         |
| `port skills status`        | Show current configuration and last sync time                                          |
| `port cache clear`          | Full cleanup: remove hooks, skill files, and config — everything Port CLI installed    |
| `port cache clear --force`  | Full cleanup without confirmation prompt                                               |


---

## Checking your configuration

```sh
port skills status
```

Output example — team defaults mode (the default after `port skills init`):

```
Port Skills Status
────────────────────────────────────────
Last synced:     2026-03-25T09:00:00Z

Hook targets (6):
  - /Users/you/.cursor/skills/
  - /Users/you/.claude/skills/
  - /Users/you/.gemini/skills/
  - /Users/you/.codex/skills/
  - /Users/you/.codeium/windsurf/skills/
  - /Users/you/myproject/.github/skills/

Project directories (1):
  - /Users/you/myproject

Skill selection:
  Groups:           team defaults (groups owned by your teams)
    extra includes (1):
      + operations
    excluded (1):
      - legacy-engineering
  Ungrouped skills (2):
    - incident-triage
    - integrations-overview
```

Output example — explicit selection (after `port skills select --select-all-groups --select-all-ungrouped`):

```
Port Skills Status
────────────────────────────────────────
Last synced:     2026-03-25T09:00:00Z

Hook targets (1):
  - /Users/you/.cursor/skills/

Project directories (0):
  (none)

Skill selection:
  Groups:           all
  Ungrouped skills: all
```

The **GitHub Copilot** line is a path inside your **repository** (`…/myproject/.github`), not under your home directory like the other tools. The same `myproject` folder also appears under **Project directories** because `port skills init` registers that repo for Port `location=project` skills (all tools) and ties Copilot hooks to the repo root. That duplication in the example is intentional: one line is the Copilot skill/hook root, the other is the registered project root used when syncing.

---

## Clearing locally synced skills

To remove all Port skill files from your local AI tool directories without touching hooks or config:

```sh
port skills clear
```

This deletes Port-managed skill directories from every configured target and project dir, and prompts for confirmation first. To skip the prompt:

```sh
port skills clear --force
```

> **Note:** This only removes the skill files — hooks and config remain intact. Skills will be re-synced automatically the next time you start a new AI session, or run `port skills sync` to sync immediately.

---

## Full cleanup

To remove everything Port CLI installed — hooks, skill files, and saved config:

```sh
port cache clear
```

This surgically removes only the Port entries from your `hooks.json` / `settings.json` files (other hooks are left untouched), deletes all skill files, and clears the skills section from `~/.port/config.yaml`. GitHub Copilot hooks under `<repo>/.github/hooks/` are found using the saved paths in your skills config, so cleanup works even if you run the command outside the repository.

To skip the confirmation:

```sh
port cache clear --force
```

---

## How it works

```
~/.cursor/hooks.json                  ← sessionStart → port skills sync
~/.claude/settings.json               ← UserPromptSubmit → port skills sync
~/.gemini/settings.json               ← SessionStart → port skills sync
~/.codex/hooks.json                   ← sessionStart → port skills sync
~/.codeium/windsurf/hooks.json        ← pre_user_prompt → port skills sync
<repo>/.github/hooks/hooks.json       ← sessionStart → port skills sync (Copilot)

port skills sync
  └─ GET /skills (grouped; full file content; team filter and group include/exclude from saved config)
  └─ `port skills init` fetches metadata-only (`exclude=files`) for the selection UI, then sync loads full content
  └─ for each skill, checks location from the catalog:
       "global"  → writes to every AI tool dir configured during init
                   e.g. ~/.cursor/skills/{skill}/SKILL.md
                   e.g. <repo>/.github/skills/{skill}/SKILL.md (Copilot)
       "project" → writes to the matching tool sub-directory inside each
                   project dir registered in ~/.port/config.yaml
                   e.g. ~/projects/my-app/.cursor/skills/{skill}/SKILL.md
                   e.g. ~/projects/my-app/.github/skills/{skill}/SKILL.md
                   if the project dir is inside a git repo, adds the generated
                   skills path to .gitignore unless already ignored
  └─ removes any local skill dirs no longer in Port

port skills clear
  └─ removes Port-managed skills from every configured AI tool dir
  └─ removes Port-managed skills from every registered project dir

port cache clear
  └─ removes Port hook entries from all AI tool hook/settings files (missing or
      invalid hook files are skipped — no error if hooks were never installed)
  └─ removes Port-managed skills from all dirs (same as port skills clear)
  └─ clears skills config from ~/.port/config.yaml
```

### Skill location

Each skill in Port has a `location` property on the `skill` blueprint:


| Value                | Where the skill is written                                                                                                                                             |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `global` *(default)* | Your AI tool directories (`~/.cursor/skills/`, etc.). If GitHub Copilot is enabled, that includes `<repo>/.github/skills/` for each repo where you ran init. |
| `project`            | Every directory where you have run `port skills init`                                                                                                                  |


If the `location` property is missing or set to any other value, `global` is used. You do not choose this when running `port skills init` — it is fully controlled from Port.

Running `port skills init` in a project registers that directory. You can run it in multiple projects; all of them will receive project-scoped skills on every `port skills sync`.

**GitHub Copilot:** Copilot does not load agent skills or hooks from a global home directory in this flow. Hooks and synced skills live only under `<repo>/.github/`. Older CLI versions may have used `~/.copilot`; `port cache clear` removes Port hook entries from that legacy path too.

Skills are written as `SKILL.md` files under `skills/{skill-name}/`, following the [Agent Skills specification](https://agentskills.io/specification) used by supported AI tools. The local folder name always matches the `name` in `SKILL.md`; Port skill groups are used for selection only. Reference, asset, script (`scripts`), and other bundled files (`additional_files`) defined on the skill entity—each an array of `{ path, content }` like references and assets—are written alongside `SKILL.md`.

---

## Hook formats by tool


| Tool           | Default hook file                 | Event key          |
| -------------- | --------------------------------- | ------------------ |
| Cursor         | `~/.cursor/hooks.json`            | `sessionStart`     |
| Claude Code    | `~/.claude/settings.json`         | `UserPromptSubmit` |
| Gemini CLI     | `~/.gemini/settings.json`         | `SessionStart`     |
| OpenAI Codex   | `~/.codex/hooks.json`             | `sessionStart`     |
| Windsurf       | `~/.codeium/windsurf/hooks.json`  | `pre_user_prompt`  |
| GitHub Copilot | `<repo>/.github/hooks/hooks.json` | `sessionStart`     |


GitHub Copilot agent hooks and synced skills are **repository-local** under
`<repo>/.github/`, following the
[agent skills specification](https://docs.github.com/en/copilot/concepts/agents/about-agent-skills).
Hook entries use GitHub’s agent format (`type: command`, `bash`, `powershell`, etc.) as described in
[About hooks](https://docs.github.com/en/copilot/concepts/agents/cloud-agent/about-hooks), not the Cursor-style `{ "command": "..." }` object.

### XDG and custom config directories

For tools that support non-default config locations, the CLI checks environment
variables before falling back to the default `~/.<tool>` path:


| Tool   | Env override        | XDG support               |
| ------ | ------------------- | ------------------------- |
| Cursor | `CURSOR_CONFIG_DIR` | `$XDG_CONFIG_HOME/cursor` |


Resolution order for each tool:

1. **Tool-specific env var** — if `CURSOR_CONFIG_DIR` is set, that path is used directly.
2. `**XDG_CONFIG_HOME*`* — if the tool has XDG support and `XDG_CONFIG_HOME` is set, the tool's XDG directory name is used under it (e.g. `$XDG_CONFIG_HOME/cursor`).
3. **Default** — `~/.<tool>` (e.g. `~/.cursor`).

Other tools (Claude Code, Gemini CLI, etc.) do not currently support custom
config directories and always use their default paths.

---

## Configuration file

The CLI stores its state in `~/.port/config.yaml` under a `skills` section:

```yaml
skills:
  targets:
    - /Users/you/.cursor
    - /Users/you/.claude
    - /Users/you/.gemini
    - /Users/you/.codex
    - /Users/you/.codeium/windsurf
    - /Users/you/myproject/.github
  project_dirs:
    - /Users/you/myproject
  team_group_defaults: true
  include_groups: []   # extra groups beyond your teams
  exclude_groups: []   # team-owned groups you opted out of during init
  select_all_ungrouped: true
  selected_skills: []
  last_synced_at: "2026-03-25T09:00:00Z"
```

Older configs may still use a top-level `plugin:` key; the CLI reads that for backward compatibility and writes `skills:` on the next save.

You can edit this file directly if you prefer.

---

## FAQ

**How are skills managed in Port?**

Port supports two skills data models:

- The [main skills data model](skills-main-data-model.md), based on `skill_group => skill`, is the default customer model.
- The [experimental versioned skills data model](skills-versioned-data-model.md), based on `_skill*` system blueprints, is available only after Port enables it for your organization.

Because skills live in the Port catalog, you can populate and keep them up to date using any of the normal ingestion methods:

- **Port UI** — create and edit skill entities directly in the catalog.
- **Port API** — `POST /v1/blueprints/skill/entities` to create or upsert skills programmatically from any script or CI pipeline.
- **Ocean integrations** — Port's 60+ plug-and-play integrations (GitHub, GitLab, Kubernetes, Jira, etc.) can map tool data to skill entities via the standard mapping configuration. See [sync data to catalog](https://docs.port.io/build-your-software-catalog/sync-data-to-catalog) for the full list.
- **Webhooks** — push skill updates from external systems by sending a payload to a Port webhook endpoint.
- **IaC (Terraform / Pulumi)** — define skill entities as infrastructure-as-code resources and apply them as part of your normal delivery pipeline.
- **Custom Ocean integrations** — build a dedicated integration for any internal tool using the Ocean framework.

Whichever method you use, `port skills sync` will pick up the latest state of all skill entities the next time a hook fires or you run the command manually.

---

**Does the CLI support skill versioning?**

Versioning is supported only when Port has enabled the experimental versioned skills data model for your organization. The main `skill_group => skill` model reflects the current state of each skill and does not support CLI-managed upload/publish versioning.

---

**Can I install skills from a public skills marketplace?**

Not at this time. Skills are private to your Port organization. There is no public marketplace to browse or install community-contributed skills from. All skills must be created and managed within your own Port account.

---

## Creating versioned skills from local directories

`port skills upload`, `port skills publish`, and `port skills unpublish` require the [experimental versioned skills data model](skills-versioned-data-model.md). They will not work against the main `skill_group => skill` model. Contact Port to enable the new skills data model with versioning.

Uploaded skills follow the [Agent Skills specification](https://agentskills.io/specification): each skill is a directory with `SKILL.md` at the root (YAML frontmatter with `name` and `description`), plus optional `scripts/`, `references/`, and `assets/`.

```sh
# Single skill directory (folder basename must match SKILL.md name:)
port skills upload ./my-skill --publish

# Bundle: e.g. ./claude/skills with skill-a/SKILL.md, skill-b/SKILL.md
port skills upload ./claude/skills --publish
```

`upload` **upserts** through the Skills API: a missing skill is created, and an existing skill receives a new patch version instead of returning a conflict.

The skill folder name, optional `--identifier`, and SKILL.md frontmatter `name:` must all match after normalization.

`--publish` makes the uploaded version active. Without it, the active version is unchanged.

Init/sync catalog includes built-in `@port-labs/ai-skills` under **ungrouped** when enabled by org feature flags (Port/customer skills win on name collision).

Skills API routes (under your org `api_url`): `GET /skills`, `GET /skills/search`, `POST /skills/upload`, `POST /skills/upload/batch`, `GET /skills/:identifier`, `POST /skills/:identifier/publish`, `POST /skills/:identifier/unpublish`.

---

## Troubleshooting

**Skills are not appearing in my AI tool**

- Verify the hook is installed: check that the appropriate hooks file exists (see table above).
- Start a brand new session (existing sessions do not re-run the hook).
- Run `port skills sync` manually to see any error output.

**Authentication errors**

- Re-run `port auth login` to refresh your token.

**Port API errors**

- Confirm skills have an active version set in your Port organization.
- Check your API URL with `port config --show`.
- Use `port skills --org <name>` if you have multiple organizations in config.

**GitHub Copilot hooks not working**

- GitHub Copilot only supports repo-scoped hooks. Make sure you ran `port skills init` from the root of the repository where you want hooks installed.

