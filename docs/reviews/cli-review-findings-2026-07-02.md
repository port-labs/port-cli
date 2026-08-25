# Port CLI Review Findings

Date: 2026-07-02

Scope: user-facing CLI UX, live command behavior, internal architecture, tests, CI, release automation, and dry-run safety.

Credentials note: live checks were run using temporary local config files with restricted permissions. Secrets are intentionally omitted from this report.

## Executive Summary

The Port CLI is already broad and useful: it has extensive command coverage, strong documentation, a sensible module split, and tests across many feature areas. Local tests pass, and the live org smoke checks confirmed that core read, compare, export, import dry-run, and migrate dry-run flows work.

The highest-impact improvement areas are:

- Make global CLI behavior reliable: `--quiet`, `--no-color`, `--yes`, `PORT_CONFIG_FILE`, and invalid output-format handling are inconsistent.
- Harden safety and automation behavior around destructive commands and non-interactive prompts.
- Resolve a live-data inconsistency where `compare` says two orgs are identical but `migrate --dry-run` says it would create/update resources.
- Improve dry-run and JSON outputs so automation users can see exact resource identifiers, not only counts.
- Reduce repeated command/API implementation code and increase command-layer/API-wrapper test coverage.
- Strengthen CI and release gates.

## Review Commands and Checks Run

- `bd onboard`
- `make build`
- `go test ./...`
- `go test ./... -coverprofile=...`
- `./bin/port --help`
- `./bin/port --tree`
- Help checks for:
  - `auth`
  - `config`
  - `export`
  - `import`
  - `migrate`
  - `compare`
  - `clear`
  - `api`
  - `skills`
  - `cache`
  - `version`
  - `completion`
- Behavior checks for:
  - config display and config path handling
  - invalid resource errors
  - invalid output format behavior
  - `--quiet`
  - `--no-color`
  - non-interactive `clear`
  - API failure paths
  - live org list/compare/export/import dry-run/migrate dry-run flows

## Test and Coverage Snapshot

All tests passed.

Coverage highlights:

- Total: 46.0%
- `internal/commands`: 22.8%
- `internal/api`: 34.6%
- `internal/modules/export`: 75.2%
- `internal/modules/import_module`: 56.8%
- `internal/modules/skills`: 69.2%

The command layer and API wrapper layer have the weakest coverage, even though they define most user-visible behavior.

## Live Org Smoke Check Results

Read-only and dry-run checks were run against a base and target org using a temporary config file.

Observed results:

- `api blueprints list --org base --format json`: 91 blueprints
- `api blueprints list --org target --format json`: 91 blueprints
- `api pages list --org base --format json`: 217 pages
- `api pages list --org target --format json`: 217 pages
- `compare --source base --target target --output json`: identical, with zero added/modified/removed resources
- `compare --source base --target target --include entities --output json`: identical, with zero added/modified/removed resources
- `export --base-org base --skip-entities --output-format json`: succeeded
- `import --target-org target --dry-run --output-format json`: succeeded
- `migrate --source-org base --target-org target --skip-entities --dry-run`: succeeded

No live writes were performed.

## P0 Implementation Status

Implemented on 2026-07-02:

- `compare` and `migrate --dry-run` now agree on the reviewed live orgs for schema-only migration dry-runs.
- Migration dry-run JSON and verbose text output include actionable identifiers for non-zero blueprint and permission changes.
- `--no-color` error suggestions no longer emit ANSI styling.
- `port --quiet version` prints only the version string.
- `PORT_CONFIG_FILE` is honored below explicit `--config` and above the default config path.
- `compare --output` rejects invalid values before doing comparison work.
- `clear` honors global `--yes` / `-y` and fails non-zero in non-interactive mode when confirmation would be required.
- Config writes use `0600` file permissions on Unix-like systems.

Validation performed:

- `go test ./...`
- `make build`
- Local CLI contract smoke checks for no-color, quiet version, `PORT_CONFIG_FILE`, invalid compare output, and non-interactive `clear`.
- Read-only/dry-run live smoke checks for compare and migrate dry-run.

## P0 Findings

### 1. `compare` and `migrate --dry-run` disagree on live orgs

Live `compare` reported the base and target orgs as identical:

```text
identical: true
0 added, 0 modified, 0 removed
```

But live `migrate --dry-run --skip-entities` reported:

```text
Blueprints: 1 new, 0 updated, 90 skipped (identical)
Blueprint permissions updated: 1
```

This creates a trust issue: users cannot tell whether `compare` is missing a resource difference or migration dry-run is over-reporting a change.

Recommended fixes:

- Add regression tests using equivalent export fixtures and assert `compare` and `migrate --dry-run` agree.
- Make migration dry-run include resource identifiers for any create/update/delete counts.
- Add `--verbose` text output that lists the blueprint and permission identifiers behind the counts.
- Check whether compare includes the same blueprint permission resources and comparison normalization as migration.

### 2. `--no-color` does not disable colors on error output

Observed with an invalid resource:

```bash
port --no-color export --include nope --output /tmp/x.tar.gz
```

The output still contained ANSI escape sequences in the suggestion block.

Likely cause: the final top-level error handler reinitializes output with color enabled:

```go
output.Init(false)
```

Recommended fixes:

- Preserve the parsed `noColor` value in the error path.
- Add a regression test that invokes an invalid command with `--no-color` and asserts no ANSI escape sequences are present.

### 3. `--quiet` is not consistently honored

Observed:

```bash
port --quiet version
```

The full version banner still prints.

Additional risk: many direct `fmt.Println` / `fmt.Printf` calls bypass the central output verbosity layer.

Recommended fixes:

- Route user-facing output through `internal/output`.
- Define explicit output contracts:
  - `--quiet`: errors only, except machine-requested output.
  - default: human-friendly progress and summaries.
  - `--verbose`: extra diagnostics and identifiers.
- Add tests for representative commands and output modes.

### 4. Global `--yes` does not appear to work for `clear`

`clear` checks only the local `force` flag:

```go
if !force {
    ...
}
```

It does not use the existing helper:

```go
ShouldSkipConfirm(cmd, force)
```

Recommended fixes:

- Use `ShouldSkipConfirm` for `clear` and all commands that prompt.
- Add tests for `--force`, `--yes`, and `-y`.

### 5. Non-interactive `clear` cancellation exits successfully

Observed:

```bash
port clear --entities < /dev/null
```

Output:

```text
Delete entities from organization "production"? [y/N]: Operation cancelled
```

Exit code: 0.

This is unsafe for CI because a job can appear successful even when nothing happened.

Recommended fixes:

- Detect non-TTY stdin when a prompt would be shown.
- If non-interactive and no `--force` / `--yes`, return a non-zero error.
- Use the existing `RequireInteractive` / `confirmPrompt` helpers instead of raw `fmt.Scanln`.

### 6. `PORT_CONFIG_FILE` is documented but not implemented

README documents `PORT_CONFIG_FILE`, but config path resolution does not appear to read it.

Recommended fixes:

- Implement config path precedence:
  - `--config`
  - `PORT_CONFIG_FILE`
  - `~/.port/config.yaml`
- Add tests for precedence.
- Update `port config --show` to display the resolved config source.

### 7. Config files containing secrets are written with mode `0644`

`ConfigManager.Write` and `WriteBytes` use world-readable file mode:

```go
os.WriteFile(cm.configPath, data, 0o644)
```

The config can contain `client_secret`.

Recommended fixes:

- Write config files as `0600`.
- Warn if an existing config file has broader permissions.
- Audit credential/token files for the same issue.

## P1 Implementation Status

Implemented on 2026-07-02 in branch `fix/cli-p1-review-findings`:

- Extended enum validation to `export --format`, `export --output-format`, `import --output-format`, `migrate --output-format`, and API `--format` output helpers.
- Expanded export JSON summary with `format`, folders count, skipped-entity state, included resources, and blueprint exclusion filters.
- Documented `port api call` raw-envelope behavior and added `--unwrap <field>` for script-friendly extraction.
- Fixed missing-organization suggestions so they recommend `port config --show` instead of config initialization.
- Made release lint blocking by removing `|| true`.
- Re-enabled `ineffassign` and `unused` in golangci-lint configuration.
- Hardened OAuth callback server to use a private mux and return listener/server errors instead of calling `log.Fatal`.
- Added `blueprints_skipped_ids` to migration dry-run JSON and verbose text output.

## P1 Findings

### 8. `compare --output` accepts invalid values silently

Observed:

```bash
port compare --source a --target b --output xml
```

The command succeeded and emitted text output.

Recommended fixes:

- Validate `--output` values at the command boundary.
- Supported values should be explicit: `text`, `json`, `html`.
- Apply the same enum validation pattern to all `--format` and `--output-format` flags.

### 9. Migration dry-run JSON lacks resource identifiers

Live dry-run JSON returned counts such as:

```json
{
  "blueprints_created": 1,
  "blueprint_permissions_updated": 1,
  "blueprints_skipped": 90,
  "success": true
}
```

It did not identify which blueprint or permission would change.

Recommended improvements:

- Include detailed lists in JSON output:
  - `blueprints_to_create`
  - `blueprints_to_update`
  - `blueprints_skipped`
  - `permissions_to_update`
- In text mode, list identifiers when `--verbose` is passed.

### 10. Export JSON summary is too sparse

`export --skip-entities --output-format json` returned success, but common resource count fields were absent or null when inspected.

Recommended improvements:

- Include the same counts text mode prints:
  - blueprints
  - entities
  - actions
  - users
  - teams
  - pages
  - integrations
- Include output path, format, skipped resources, and selected filters.

### 11. Generic API output shape differs from resource-specific API output

`api blueprints list --format json` returns a JSON array.

`api call /blueprints --format json` returns the raw API envelope with keys such as:

```text
blueprints
ok
```

This may be correct, but the difference should be documented.

Recommended improvements:

- Document in `api call --help` that it returns raw API response envelopes.
- Consider adding `--unwrap <field>` or `--jq` for script-friendly extraction.

### 12. Invalid org suggestion is misleading

For a missing org, the CLI correctly reports:

```text
Organization 'missing' not found in configuration. Available organizations: [base target].
```

But the suggestion says:

```text
Run `port config --init` to create a configuration file
```

That is not the right advice when a config exists and only the org name is wrong.

Recommended fixes:

- Refine error suggestion mapping.
- For `organization '<name>' not found`, suggest `port config --show`.
- Avoid generic config-init suggestions when config exists.

### 13. Release lint is non-blocking

Release workflow contains:

```yaml
run: make lint || true
```

Recommended fixes:

- Remove `|| true` once lint is stable.
- If necessary, scope or tune lint before making it blocking.

### 14. Lint config disables useful checks

`.golangci.yml` disables:

```yaml
disable:
  - errcheck
  - ineffassign
  - unused
```

Recommended improvements:

- Re-enable `ineffassign` and `unused` first.
- Re-enable `errcheck` with targeted exclusions.
- Consider adding `gosec`, `unparam`, `unconvert`, `revive`, `goconst`, and `noctx`.

### 15. OAuth login server uses global mux and fatal exits

`auth.TokenFromOAuth` uses a fixed port, global HTTP mux, and `log.Fatalln` from a goroutine.

Recommended fixes:

- Use a private `http.NewServeMux`.
- Avoid `log.Fatal` in library code.
- Return server errors through a channel.
- Prefer `127.0.0.1:0` and derive the callback URL dynamically where supported.

## P2 Implementation Status

Implemented on 2026-07-02 in branch `fix/cli-p2-review-findings`:

- Added `--no-env-file` / `PORT_NO_ENV_FILE=1` support to disable `.env` loading.
- Added `port config sources` to show config path, env-file status, and environment override presence without printing secret values.
- Clarified `cache clear` help text to say it removes local hooks, skill files, and skills config.
- Normalized selected help text (`CLI`, `comma-separated`).
- Added transient 500/502/503/504 retry support and request-body-safe retry construction.
- Added endpoint-wrapper tests for blueprint API methods as a coverage/refactor foothold.

Deferred follow-up:

- Full API command descriptor refactor remains larger P3/P4 work.
- Full help flag grouping remains partially limited by inherited persistent flags and Fang/Cobra behavior.

## P2 Findings

### 16. Help output is comprehensive but noisy

Several command families display target-org flags even when irrelevant, such as `auth`, `cache`, `completion`, and `version`.

Recommended improvements:

- Hide irrelevant persistent flags for command families where they cannot apply.
- Group flags by purpose:
  - authentication
  - source org
  - target org
  - output
  - safety
  - debugging
- Consider shorter default help plus fuller generated docs.

### 17. Help text style is inconsistent

Examples include:

- `Authenticate the cli with Port`
- `First-Time setup`
- `Comma-Separated`
- `Base org` vs `source org`
- `Target org` vs `destination org`

Recommended fixes:

- Add a help-text style guide.
- Normalize terms:
  - CLI
  - first-time
  - comma-separated
  - source organization / target organization
- Add golden tests for key help output.

### 18. `cache clear` is more destructive than the name implies

Help says:

```text
Remove everything Port CLI installed locally (hooks, skill files, and config)
```

That includes more than cache.

Recommended options:

- Rename or alias to something clearer, such as `port local clear`, `port skills reset`, or `port uninstall-local`.
- At minimum, make the short description explicit that it removes config, hooks, skills, and cache.

### 19. Current directory `.env` auto-loading is surprising

The CLI loads `.env` from the current directory and `~/.port/.env`.

Risks:

- A repo checkout can influence every `port` command run inside it.
- Tests and debugging can accidentally use real credentials.
- `--config` does not isolate from `.env`.

Recommended improvements:

- Add `--no-env-file`.
- Add `PORT_NO_ENV_FILE=1`.
- Add `port config sources` or `port doctor` to show where credentials came from.
- Consider using `.port.env` instead of generic `.env`, or warn when loading credentials from cwd.

### 20. `internal/commands/api.go` is too large and repetitive

The API command implementation repeats CRUD command patterns for many resources.

Recommended refactor:

- Introduce resource descriptors with:
  - resource name
  - routes
  - ID args
  - list/get/create/update/delete functions
  - supported flags
  - confirmation behavior
- Generate Cobra commands from descriptors.
- Keep hand-written code for unusual resources only.

Benefits:

- Less copy/paste risk.
- Easier resource additions.
- Easier uniform testing.
- Easier consistent output/format validation.

### 21. API wrapper methods have low direct coverage

Many endpoint wrappers in `internal/api/requests.go` show 0% direct coverage.

Recommended improvements:

- Add table-driven `httptest` tests.
- Validate paths, methods, query params, body shape, and response decoding.
- Consider generating request wrappers or path constants from OpenAPI.

### 22. Retry behavior should include transient 5xx responses

Current retry behavior handles network errors and 429s. It does not appear to retry common transient server errors such as 502, 503, or 504.

Recommended improvements:

- Retry 429, 500, 502, 503, and 504 where safe.
- Add jitter.
- Verify request body retry correctness for POST/PUT/PATCH.
- Respect context cancellation.
- Add tests for `Retry-After`, capped backoff, 5xx retry, and cancellation.

## Documentation Findings

### 23. README and CONTRIBUTING disagree on Go version

`go.mod` and CI use Go 1.25.9, while CONTRIBUTING says Go 1.21+.

Recommended fix:

- Update CONTRIBUTING to match the actual required Go version, or lower the module version if 1.21 compatibility is intended.

### 24. Credential source precedence should be easier to diagnose

Recommended UX:

```bash
port config sources
```

Example information to show:

- Config file path
- Env files loaded
- Which source supplied each credential value
- Default org resolution
- Whether cwd `.env` influenced the command

## Suggested Prioritized Backlog

### P0 / next PR

1. Diagnose and fix the `compare` vs `migrate --dry-run` inconsistency.
2. Add identifiers to migration dry-run output.
3. Fix `--no-color` on errors.
4. Make `--quiet` respected by `version` and representative command output.
5. Implement `PORT_CONFIG_FILE`.
6. Validate `compare --output`.
7. Make `clear` use `ShouldSkipConfirm`.
8. Make non-interactive prompt cancellation exit non-zero.
9. Write config files with `0600`.

### P1

1. Add CLI golden/smoke tests for global flags and output contracts.
2. Make release lint blocking.
3. Add `govulncheck`.
4. Add enum validation helpers for all `--format` / `--output-format` flags.
5. Add credential-source diagnostics.
6. Add `--no-env-file`.
7. Improve export/import/migrate JSON summaries.
8. Fix misleading invalid-org suggestions.

### P2

1. Refactor `internal/commands/api.go` into declarative resource command builders.
2. Add API endpoint wrapper tests.
3. Harden retry behavior for transient 5xx responses.
4. Refactor OAuth callback server.
5. Add shell completion tests.
6. Add npm package smoke tests.

### P3

1. Redesign help flag grouping.
2. Rename or clarify `cache clear`.
3. Normalize help text style.
4. Add manpage/docs generation from Cobra metadata.
5. Add structured JSON error mode for automation.

## P3 Implementation Status

Implemented on 2026-07-02 in branch `fix/cli-p2-review-findings`:

- Added `--json-errors` for structured command error output.
- Added `port docs markdown` and `port docs man` to generate CLI reference docs from Cobra metadata.
- Updated CONTRIBUTING Go prerequisite to match `go.mod` and CI (`1.25.9+`).

## Recommended Automated Guardrail

Add a GitHub Actions workflow for PRs touching CLI code or docs that:

- Builds `bin/port`.
- Runs `go test ./...`.
- Runs smoke checks:
  - `port --help`
  - `port --tree`
  - `port --no-color <known-invalid-command>` and assert no ANSI escapes
  - `port --quiet version` and assert minimal/no output
  - `PORT_CONFIG_FILE=/tmp/custom.yaml port config --show`
  - `port compare --output invalid` and assert non-zero
  - `port clear --entities < /dev/null` and assert non-zero
- Runs `govulncheck ./...`.
- Optionally runs a Tessl verifier for CLI UX invariants.

This would catch most of the review findings before a human has to rediscover them.
