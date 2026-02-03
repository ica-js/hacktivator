# Hacktivator

A CLI tool to quickly activate Azure PIM (Privileged Identity Management) eligible roles from the command line.

## Features

- 🔍 **Fuzzy finder interface** - Quickly search and select from your eligible roles
- 🔐 **Uses Azure CLI authentication** - No need to manage separate credentials
- ⚡ **Fast activation** - Activate roles in seconds without navigating the Azure Portal
- 📋 **Ticket integration** - Support for ticket numbers and systems for compliance

## Prerequisites

- **Azure CLI** - Install from [Microsoft's documentation](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli)
- **GitHub CLI** - Install from [cli.github.com](https://cli.github.com/) (for installation only)

## Installation

### Quick Install (Recommended)

Make sure you're authenticated with the GitHub CLI, then run:

```bash
# One-time setup (if not already done)
gh auth login

# Install hacktivator
gh release download --repo ica-js/hacktivator --pattern '*darwin_arm64*' -O - | tar -xz && sudo mv hacktivator /usr/local/bin/
```

Replace `darwin_arm64` with your platform:
- `darwin_arm64` - macOS Apple Silicon
- `darwin_amd64` - macOS Intel
- `linux_amd64` - Linux x86_64
- `linux_arm64` - Linux ARM64

### Install Script

Clone the repo and run the install script:

```bash
git clone https://github.com/ica-js/hacktivator.git
cd hacktivator
./install.sh
```

The script auto-detects your OS and architecture.

### Manual Download

1. Go to the [Releases page](https://github.com/ica-js/hacktivator/releases)
2. Download the archive for your platform
3. Extract and move to your PATH:

```bash
tar -xzf hacktivator_*.tar.gz
sudo mv hacktivator /usr/local/bin/
```

### Build from Source

```bash
git clone https://github.com/ica-js/hacktivator.git
cd hacktivator
go build -o hacktivator .
sudo mv hacktivator /usr/local/bin/
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
2. Fetch all your eligible PIM roles across all subscriptions
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