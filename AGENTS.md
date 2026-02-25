# AGENTS.md

## Project Overview

Hacktivator is a Go CLI tool for activating Azure PIM (Privileged Identity Management) eligible roles from the terminal. It shells out to the Azure CLI (`az`) for authentication and API calls, and provides an interactive TUI for role selection using the Charm stack (Bubble Tea, Bubbles, Lipgloss).

## Commands

```bash
# Build
go build -o hacktivator .

# Build (release-style, stripped binary)
CGO_ENABLED=0 go build -ldflags="-s -w" -o hacktivator .

# Run directly
go run .

# Tidy dependencies
go mod tidy

# Release (uses GoReleaser)
goreleaser release
```

There is no Makefile, no test suite, no linter configuration, and no CI workflow files.

## Project Structure

```
main.go                      # CLI entry point: cobra commands (root/list/status), flag definitions, command handlers
internal/
  azure/
    cli.go                   # Azure CLI wrappers: auth checks, user info, subscriptions, az rest calls
    pim.go                   # PIM API logic: eligible/active role fetching, role activation, scope parsing
  ui/
    selector.go              # Interactive role/subscription selectors using Bubble Tea list + viewport preview
    spinner.go               # Generic spinner component with SpinWithResult[T] and SpinWithAction helpers
    styles.go                # Lipgloss style definitions (colors, borders, formatting)
    table.go                 # Plain-text table rendering for eligible/active roles
    textprompt.go            # Text input prompt (justification entry)
```

The codebase is small (~7 files of source) with a flat two-package internal structure.

## Architecture & Patterns

### Azure CLI Dependency

All Azure interactions shell out to `az` via `os/exec`. There is no Azure SDK usage. Key functions:

- `runAzCommand(args...)` — general-purpose `az` executor returning stdout string
- `AzRest(method, url, body)` — wraps `az rest` for ARM API calls
- Auth checks use `exec.LookPath("az")` and `az account show`

The PIM API calls target `https://management.azure.com` using `roleEligibilityScheduleInstances` (list) and `roleAssignmentScheduleRequests` (activate) endpoints at API version `2020-10-01`.

### Bubble Tea TUI Pattern

All interactive UI components follow the standard Bubble Tea `Model` interface pattern (`Init`, `Update`, `View`). Components:

- **`selectorModel`** — Fuzzy-filterable list with optional side preview pane (responsive: preview hidden when terminal < 60 cols)
- **`spinnerModel`** — Runs a background function while showing a spinner, uses `resultMsg` to deliver results
- **`textPromptModel`** — Single-line text input

The generic `SpinWithResult[T]` function uses Go generics to wrap any `func() (T, error)` with a spinner. Falls back to plain `fmt.Printf` when not a TTY or in non-interactive mode.

### Non-Interactive Mode

The `--non-interactive` flag propagates through most functions. When set:
- Spinners print plain text instead of TUI
- Single-item lists auto-select
- Multi-item selections fail with an error
- Justification is skipped if not provided via `--reason`

### Verbose/Debug Output

`azure.Verbose` is a package-level bool set from the `--verbose` flag. Debug output goes to `stderr` via the `debugf()` helper.

## Naming Conventions

- **Packages**: lowercase single-word (`azure`, `ui`)
- **Exported types**: PascalCase with descriptive names (`EligibleRole`, `ActivationRequest`)
- **Bubble Tea models**: unexported `xxxModel` structs with `newXxxModel` constructors
- **list.Item implementations**: unexported `xxxItem` structs with `Title()`, `Description()`, `FilterValue()`
- **Error handling**: `fmt.Errorf` with `%w` wrapping throughout; errors bubble up to `main.go` where cobra handles display

## Style Conventions

- Tabs for indentation (standard `gofmt`)
- JSON struct tags on all API response types
- `var stdout, stderr bytes.Buffer` pattern for capturing command output
- Inline anonymous structs for one-off API response parsing (see `getEligibilityScheduleID`)
- No interfaces defined — concrete types only
- Lipgloss styles defined as package-level `var` block in `styles.go`

## Key Gotchas

1. **No tests exist.** There are no `_test.go` files anywhere. When adding features, consider that there's no test infrastructure to validate against.

2. **Group-based eligibility.** The activation flow explicitly fetches the *current user's* principal ID separately from the eligibility's principal ID, because roles assigned via Azure AD groups have a different principal on the eligibility record. See `ActivateRole()` in `pim.go:234`.

3. **Eligibility schedule linking.** Activation requires a `linkedRoleEligibilityScheduleId`. The code first tries to look up the schedule via a filtered API query (`getEligibilityScheduleID`), then falls back to extracting the instance name from the eligibility ID. This two-step approach handles edge cases where the schedule query returns empty.

4. **Pagination.** `fetchEligibleRoles` follows `nextLink` for paginated API responses. `GetActiveRoleAssignments` does NOT handle pagination — it only fetches the first page.

5. **Error swallowing on subscription iteration.** In `GetEligibleRoleAssignments`, errors from individual subscription scope queries are silently ignored (`continue`). This is intentional — users may not have PIM access to all subscriptions.

6. **`AzRest` returns stderr on error.** When `az rest` fails, `AzRest()` returns `stderr.Bytes()` as the byte slice (not stdout), which is useful for error messages but could be confusing if you expect stdout.

7. **No vendor directory.** Dependencies are managed via Go modules only (`go.sum` present, `vendor/` in `.gitignore`).

8. **GoReleaser ldflags.** The release build injects `main.version`, `main.commit`, and `main.date` via ldflags, but these variables are not currently declared in `main.go`. They would need to be added for version display functionality.

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework (commands, flags) |
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/bubbles` | TUI components (list, spinner, textinput, viewport) |
| `github.com/charmbracelet/lipgloss` | Terminal styling |
| `github.com/google/uuid` | Generating activation request IDs |
| `github.com/mattn/go-isatty` | TTY detection for spinner fallback |

## Release

Releases are built with GoReleaser (`.goreleaser.yaml`). Targets: linux/darwin (amd64/arm64) + windows. Archives are `.tar.gz` (`.zip` for Windows). Published to GitHub at `ica-js/hacktivator`.
