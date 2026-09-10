# Hacktivator

A CLI tool to quickly activate Azure PIM (Privileged Identity Management) eligible roles from the command line.

## Features

- 🔍 **Fuzzy finder interface** - Quickly search and select from your eligible roles
- 🔐 **Uses Azure CLI authentication** - No need to manage separate credentials
- ⚡ **Fast activation** - Cached eligibility and parallel discovery avoid repeated subscription scans
- 📋 **Ticket integration** - Support for ticket numbers and systems for compliance

## Prerequisites

- **Azure CLI** - Install from [Microsoft's documentation](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli)
- **Go 1.21+** - Install from [go.dev](https://go.dev/dl/)

## Installation

```bash
go install github.com/ica-js/hacktivator@latest
```

## Usage

### Basic Usage

First, ensure you're logged into Azure CLI:

```bash
az login
```

Then run hacktivator:

```bash
hacktivator
```

This will:
1. Check your Azure CLI authentication
2. Load your eligible PIM roles from cache, or fetch them across all subscriptions
3. Present an interactive fuzzy finder to select a role
4. Prompt for justification (optional)
5. Activate the selected role

### Command Line Options

```
hacktivator [flags]
hacktivator [command]

Available Commands:
  list        List all eligible PIM role assignments
  status      Show currently active PIM role assignments

Flags:
  -d, --duration int           Activation duration in minutes (default 480 = 8 hours)
  -r, --reason string          Justification reason for activation
      --ticket-number string   Ticket number for activation request
      --ticket-system string   Ticket system name (e.g., ServiceNow, Jira)
      --non-interactive        Fail if user input is required
      --refresh                Fetch fresh eligible roles and update the cache
      --cache-ttl duration     Eligibility cache lifetime (default 720h / 30 days; 0 disables caching)
  -v, --verbose                Enable verbose/debug output
  -h, --help                   Help for hacktivator
```

### Examples

List all your eligible roles:

```bash
hacktivator list
```

Check currently active PIM roles:

```bash
hacktivator status
```

Activate with a specific duration and reason:

```bash
hacktivator -d 60 -r "Emergency maintenance"
```

Activate with ticket information:

```bash
hacktivator --ticket-number "INC001234" --ticket-system "ServiceNow" -r "Incident response"
```

Non-interactive mode (useful in scripts, will fail if multiple roles are eligible):

```bash
hacktivator --non-interactive -r "Automated activation"
```

Debug mode for troubleshooting:

```bash
hacktivator -v -r "Testing"
```

### Eligibility Cache

Eligible role assignments are cached for **30 days** by default. Both activation and
`list` use the cache; a cache hit skips subscription discovery and eligibility API
calls. Azure CLI authentication and user lookup still run. The CLI displays the age
of cached results and a reminder to use `--refresh`.

```bash
hacktivator                       # Reuse fresh cached eligibility
hacktivator --refresh             # Fetch live eligibility and replace the cache
hacktivator list --refresh        # Discover newly granted roles immediately
hacktivator --cache-ttl 2h        # Use a shorter cache lifetime for this run
hacktivator --cache-ttl 0         # Disable cache reads and writes for this run
```

A missing or expired cache triggers a live fetch, querying up to four scopes at a
time. Cache age is measured from the last successful complete fetch, not the last
use. `--refresh` bypasses an otherwise fresh cache; `--cache-ttl 0` takes precedence
and prevents cache writes too.

Cache files live in the OS user-cache directory under `hacktivator/`:

- macOS: `~/Library/Caches/hacktivator/`
- Linux: `$XDG_CACHE_HOME/hacktivator/` or `~/.cache/hacktivator/`
- Windows: `%LocalAppData%\hacktivator\`

Entries are separated by user object ID, tenant, cloud, and selected subscription.
If the current identity cannot be established reliably, caching is skipped. Only
role metadata is stored—never Azure tokens or credentials. Files are written
atomically with owner-only permissions on Unix. Deleting this directory clears
all cached eligibility.

**Freshness and failures:**

- Newly granted roles may not appear until a refresh or cache expiry. Revoked roles
  may remain in the list, but Azure still checks authorization and PIM policies
  when activating; the cache cannot grant access.
- Expired eligibility and assignments that have not started are excluded from
  displayed results, including on cache hits.
- If any scope lookup fails, the CLI warns that results are incomplete and does
  not create or replace the cache. Persistent access errors at a scope therefore
  prevent caching until resolved. If every scope fails, the command fails.
- Refresh failures never silently fall back to stale data. An existing cache is
  left untouched. Unreadable or corrupt cache files trigger a live fetch; cache
  write failures warn but do not prevent using successfully fetched roles.
- `hacktivator status` always queries live active assignments; eligibility cache
  flags do not change its results.

## How It Works

Hacktivator uses the Azure Resource Manager PIM APIs to:

1. **List eligible role assignments** via `roleEligibilityScheduleInstances` API
2. **Activate roles** via `roleAssignmentScheduleRequests` API with `SelfActivate` request type

All API calls are authenticated using your existing Azure CLI session, so no additional credentials are needed.

## Supported Scopes

- ✅ Subscriptions
- ✅ Resource Groups
- ✅ Management Groups

## Troubleshooting

### "No eligible role assignments found"

- Ensure you have PIM eligible roles assigned (not just active roles)
- Try running `az login` again to refresh your token
- Check that your account has access to the subscriptions
- Run `hacktivator list --refresh` if your eligible roles have recently changed

### "az command failed"

- Verify Azure CLI is installed: `az --version`
- Ensure you're logged in: `az account show`
- Try `az login` to re-authenticate

### Role activation fails

- Check if the role requires approval (not currently supported)
- Verify the justification meets policy requirements
- Check if ticket information is required by policy
- Use `-v` (verbose) flag to see detailed API requests and responses

### "InsufficientPermissions" error

This usually means the eligibility is through a group membership. The tool automatically
handles this by using your user principal ID for activation requests.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License - see [LICENSE](LICENSE) file for details.