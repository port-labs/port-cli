# CLI P0 Remediation Plan

Date: 2026-07-02

Source findings: `docs/reviews/cli-review-findings-2026-07-02.md`

## Goal

Fix the P0 CLI trust, safety, and automation issues with small, reviewable changes that preserve existing command behavior except where the current behavior is unsafe or contradicts documented contracts.

## P0 Scope

- Diagnose and fix `compare` vs `migrate --dry-run` inconsistency.
- Add resource identifiers to migration dry-run output.
- Fix `--no-color` on error output.
- Make `--quiet` respected by representative commands, starting with `version`.
- Implement `PORT_CONFIG_FILE`.
- Validate `compare --output`.
- Make `clear` honor global `--yes` / `-y`.
- Make non-interactive prompt cancellation fail non-zero.
- Write config files with `0600` permissions.

## Non-Goals

- Full API command refactor.
- Full help redesign.
- Broad output rewrite across every command.
- Live destructive testing against real orgs.
- Changing migration semantics before the compare/dry-run mismatch is understood.

## Workstream 1: Establish CLI Contract Regression Tests

**Purpose:** Lock in desired behavior before changing implementation.

**Files likely touched:**

- `internal/commands/*_test.go`
- possibly new CLI smoke test helper under `internal/commands`
- possibly `cmd/port` test harness if root command construction needs extraction

**Tasks:**

- [ ] Add a test helper that builds the root command with injectable stdout/stderr where practical.
- [ ] Add a no-color error test:
  - run an invalid command with `--no-color`
  - assert stderr contains no ANSI escapes
- [ ] Add quiet version test:
  - run `port --quiet version`
  - assert banner/prose is suppressed according to the chosen contract
- [ ] Add `PORT_CONFIG_FILE` precedence tests:
  - explicit `--config` wins
  - env var is used when `--config` is absent
  - default path is used when neither is set
- [ ] Add `compare --output invalid` test asserting non-zero error.
- [ ] Add `clear` prompt tests:
  - `--force` skips prompt
  - global `--yes` skips prompt
  - global `-y` skips prompt
  - non-interactive stdin without force/yes returns non-zero
- [ ] Add config write-permission test asserting new config files are `0600` on Unix.

**Acceptance criteria:**

- Tests fail on current implementation for each known bug.
- Tests do not require real Port credentials or network access.

## Workstream 2: Fix Global Flag and Config Contracts

**Purpose:** Make documented and expected global flags reliable.

**Files likely touched:**

- `cmd/port/main.go`
- `internal/config/config.go`
- `internal/config/loader.go`
- `internal/commands/version.go`
- `internal/output/*`

**Tasks:**

- [ ] Preserve `noColor` in the top-level error handler instead of reinitializing output with color enabled.
- [ ] Ensure `FormatError` and suggestion rendering respect no-color mode.
- [ ] Define `--quiet` behavior for `version`:
  - preferred: print only the semantic version or nothing unless explicitly requested
  - document in tests
- [ ] Replace direct version banner printing with output-layer-aware printing.
- [ ] Implement `PORT_CONFIG_FILE` in config path resolution.
- [ ] Confirm `--config` has higher precedence than `PORT_CONFIG_FILE`.
- [ ] Update README/command help if needed to clarify precedence.

**Acceptance criteria:**

- `port --no-color <invalid>` emits no ANSI escapes.
- `port --quiet version` follows the tested quiet contract.
- `PORT_CONFIG_FILE=/tmp/config.yaml port config --show` uses `/tmp/config.yaml`.
- Existing config behavior remains unchanged when env var is absent.

## Workstream 3: Harden `clear` Safety and Non-Interactive Behavior

**Purpose:** Prevent destructive commands from silently doing nothing or ignoring global confirmation flags.

**Files likely touched:**

- `internal/commands/clear.go`
- `internal/commands/prompts.go`
- `internal/commands/clear_test.go`

**Tasks:**

- [ ] Replace local `if !force` confirmation logic with `ShouldSkipConfirm(cmd, force)`.
- [ ] Replace raw `fmt.Scanln` prompt with `confirmPrompt` or a testable prompt abstraction.
- [ ] If prompting would occur and stdin is not a TTY, return `ErrNonInteractiveRequired`.
- [ ] Ensure cancellation returns a clear non-zero error in non-interactive contexts.
- [ ] Keep interactive user-declined cancellation behavior explicit and documented.

**Acceptance criteria:**

- `port clear --entities --yes` skips confirmation.
- `port clear --entities -y` skips confirmation.
- `port clear --entities < /dev/null` exits non-zero unless `--force` or `--yes` is provided.
- No destructive test performs a real API deletion.

## Workstream 4: Validate Output Format Enums

**Purpose:** Prevent silent fallback when users request machine-readable output.

**Files likely touched:**

- `internal/commands/compare.go`
- possibly shared validation helper in `internal/commands`
- tests in `internal/commands/compare_test.go`

**Tasks:**

- [ ] Add a shared helper for validating enum-style flags.
- [ ] Validate `compare --output` against `text`, `json`, and `html`.
- [ ] Return an actionable error that names valid values.
- [ ] Audit other P0-adjacent format flags and record follow-up issues for non-P0 cleanup.

**Acceptance criteria:**

- `port compare --output xml ...` exits non-zero before doing network/file work.
- Error message includes valid values.
- Existing `text`, `json`, and `html` behavior remains unchanged.

## Workstream 5: Secure Config File Writes

**Purpose:** Avoid writing secrets into world-readable config files.

**Files likely touched:**

- `internal/config/loader.go`
- `internal/config/config_test.go`
- possibly credential/token config code if similar writes are found

**Tasks:**

- [ ] Change config writes from `0644` to `0600`.
- [ ] Apply the same mode in `Write` and `WriteBytes`.
- [ ] Audit related credential/token write paths and either fix them or file P1 follow-ups.
- [ ] Add tests that skip or adapt on Windows if mode semantics differ.

**Acceptance criteria:**

- New config files are created with `0600` permissions on Unix.
- Existing config content remains unchanged except file mode.
- Tests pass cross-platform or are correctly platform-gated.

## Workstream 6: Diagnose Compare vs Migration Dry-Run Mismatch

**Purpose:** Restore trust in compare and dry-run safety before migration users rely on results.

**Files likely touched:**

- `internal/modules/compare/*`
- `internal/modules/migrate/*`
- `internal/modules/import_module/diff.go`
- tests/fixtures for export data

**Tasks:**

- [ ] Reproduce mismatch locally with sanitized export fixtures if possible.
- [ ] Identify the single blueprint reported as created by migration dry-run.
- [ ] Identify the blueprint permission reported as updated.
- [ ] Compare normalization rules between compare and migration diff:
  - default fields
  - nil vs empty objects/slices
  - system blueprints
  - permissions included/excluded
  - generated or server-managed fields
- [ ] Decide the source of truth:
  - If compare is missing a resource category, update compare.
  - If migration diff is over-reporting, normalize or skip the false difference.
- [ ] Add fixture-based regression tests so identical inputs produce zero compare diffs and zero migration dry-run creates/updates.

**Acceptance criteria:**

- For equivalent fixture data, compare and migrate dry-run agree.
- The live-org mismatch can be explained in the PR notes without exposing secrets.
- No live writes are needed to verify the fix.

## Workstream 7: Add Dry-Run Detail to Migration Output

**Purpose:** Make dry-run counts actionable.

**Files likely touched:**

- `internal/modules/migrate/*`
- `internal/modules/import_module/*`
- `internal/commands/migrate.go`
- migrate output tests

**Tasks:**

- [ ] Extend dry-run result structs to include identifiers for resources to create/update/skip where available.
- [ ] Include identifiers in JSON output for at least:
  - blueprints to create
  - blueprints to update
  - blueprint permissions to update
- [ ] Add text-mode `--verbose` detail for create/update identifiers.
- [ ] Keep default text output concise.
- [ ] Add tests for JSON schema and verbose text output.

**Acceptance criteria:**

- `migrate --dry-run --output-format json` includes actionable identifiers for non-zero counts.
- `migrate --dry-run --verbose` shows identifiers in text mode.
- Existing consumers of summary counts continue to work.

## Recommended PR Sequence

### PR 1: CLI Contract Guardrails

Fixes:

- `--no-color` on errors
- `--quiet version`
- `PORT_CONFIG_FILE`
- `compare --output` validation
- config file `0600`

Why first: self-contained, low risk, mostly local behavior, good regression tests.

### PR 2: Destructive Command Safety

Fixes:

- `clear --yes` / `-y`
- non-interactive `clear` failure behavior
- prompt test coverage

Why separate: changes safety semantics and deserves focused review.

### PR 3: Compare vs Migration Diff Consistency

Fixes:

- mismatch diagnosis
- normalization or resource coverage fix
- fixture regression tests

Why separate: domain-specific and may require careful review of migration semantics.

### PR 4: Migration Dry-Run Detail

Fixes:

- identifiers in JSON dry-run output
- verbose text identifiers

Why separate: output schema expansion should be reviewed independently from diff semantics.

## Validation Plan

Run locally before each PR:

```bash
go test ./...
make build
./bin/port --help >/dev/null
```

Additional targeted checks after all P0s:

```bash
./bin/port --no-color export --include nope --output /tmp/x.tar.gz 2> /tmp/err.txt
# assert /tmp/err.txt has no ANSI escapes

PORT_CONFIG_FILE=/tmp/port-config.yaml ./bin/port config --show

./bin/port compare --source a --target b --output xml
# assert non-zero

./bin/port clear --entities < /dev/null
# assert non-zero unless --yes/--force is used
```

Live smoke checks, read-only/dry-run only:

- `api blueprints list` for base and target
- `compare --source base --target target --output json`
- `compare --include entities --output json`
- `export --skip-entities --output-format json`
- `import --dry-run --output-format json`
- `migrate --skip-entities --dry-run --output-format json`

## Risks and Mitigations

- **Risk:** `--quiet version` behavior may surprise users who expect version output.
  - **Mitigation:** choose and document a clear contract; optionally keep `port version --short` or root `--version` as the script-friendly version path.

- **Risk:** Non-interactive `clear` exit-code change may break scripts that relied on silent cancellation.
  - **Mitigation:** document the change and provide explicit `--yes` / `--force` migration path.

- **Risk:** Migration diff normalization can hide real differences.
  - **Mitigation:** add fixture tests for both identical and intentionally different resources.

- **Risk:** JSON dry-run schema expansion can affect consumers.
  - **Mitigation:** add fields without removing existing count fields.

## Done Criteria

- All P0 tests pass.
- `go test ./...` passes.
- `make build` passes.
- Review findings document is updated with resolved/follow-up status.
- `bd` issues are closed or updated if issues are created from this plan.
- No live destructive operations are used for validation.
