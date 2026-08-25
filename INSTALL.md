# Port CLI Installation

This document describes how to install the Port CLI binary.

## Installation Methods

Port CLI can be installed using several methods. Choose the one that best fits your workflow:

1. **npm** - Recommended for Node.js users and CI/CD pipelines
2. **Install Script** - Quick setup for Linux/macOS
3. **Binary Releases** - Manual download and installation
4. **Build from Source** - For development or custom builds

## npm Installation (Recommended)

The easiest way to install Port CLI is via npm. This method works on all platforms and automatically selects the correct binary for your system.

### Global Installation

```bash
npm install -g @port-labs/port-cli
```

After installation, use the `port` command directly:

```bash
port --version
port config --init
```

### Using npx (No Installation Required)

You can use Port CLI without installing it globally:

```bash
npx @port-labs/port-cli --version
npx @port-labs/port-cli export --output backup.tar.gz
```

This is useful for one-off commands or CI/CD pipelines where you don't want to install globally.

### Local Installation

Install Port CLI as a dependency in your project:

```bash
npm install @port-labs/port-cli
```

Then use it via `npx` or add it to your `package.json` scripts:

```json
{
  "scripts": {
    "port:export": "port export --output backup.tar.gz"
  }
}
```

**Platform Support:** The npm package includes binaries for Linux (amd64, arm64), macOS (amd64, arm64), and Windows (amd64, arm64). The correct binary is automatically selected during installation.

## Quick Install Script

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/port-labs/port-cli/main/scripts/install.sh | bash
```

### Windows

Download the latest release from [GitHub Releases](https://github.com/port-labs/port-cli/releases) and extract `port-windows-amd64.exe` to a directory in your PATH.

## Manual Installation

### 1. Download Binary

Download the appropriate binary for your platform from [GitHub Releases](https://github.com/port-labs/port-cli/releases):

- **Linux**: `port-cli_X.X.X_linux_amd64.tar.gz` or `port-cli_X.X.X_linux_arm64.tar.gz`
- **macOS**: `port-cli_X.X.X_darwin_amd64.tar.gz` or `port-cli_X.X.X_darwin_arm64.tar.gz`
- **Windows**: `port-cli_X.X.X_windows_amd64.zip`

### 2. Extract and Install

#### Linux / macOS

```bash
# Extract the archive
tar -xzf port-cli_X.X.X_linux_amd64.tar.gz

# Move to a directory in your PATH (choose one)
sudo mv port /usr/local/bin/port        # System-wide
# OR
mkdir -p ~/.local/bin
mv port ~/.local/bin/port                # User-specific

# Make executable
chmod +x /usr/local/bin/port  # or ~/.local/bin/port

# Add ~/.local/bin to PATH if needed (add to ~/.bashrc or ~/.zshrc)
export PATH="$HOME/.local/bin:$PATH"
```

#### Windows

1. Extract `port-windows-amd64.exe` from the ZIP file
2. Rename it to `port.exe`
3. Move it to a directory in your PATH (e.g., `C:\Program Files\Port CLI\`)
4. Add the directory to your PATH environment variable

### 3. Verify Installation

```bash
port --version
```

Expected output:
```
Port CLI version X.X.X
Build date: YYYY-MM-DDTHH:MM:SSZ
Git commit: abc1234
Go version: go1.21.X
Platform: darwin/arm64
```

## Configuration

After installation, configure your credentials:

```bash
# Option 1: Initialize config file
port config --init

# Option 2: Use environment variables
export PORT_CLIENT_ID="your-client-id"
export PORT_CLIENT_SECRET="your-client-secret"

# Option 3: Use CLI flags (recommended for scripts)
port export --client-id YOUR_ID --client-secret YOUR_SECRET ...
```

## Updating

To update to the latest version:

```bash
# Using install script
curl -fsSL https://raw.githubusercontent.com/port-labs/port-cli/main/scripts/install.sh | bash

# Or manually download and replace the binary
```

## Troubleshooting

### Binary not found

Make sure the binary is in a directory that's in your PATH:

```bash
# Check current PATH
echo $PATH  # Linux/macOS
echo %PATH% # Windows

# Verify binary location
which port  # Linux/macOS
where port  # Windows
```

### Permission denied

Make sure the binary is executable:

```bash
chmod +x /path/to/port
```

### Authentication errors

See the [Configuration](#configuration) section above or check the main [README.md](README.md) for detailed authentication options.

# Port CLI Installation

This document describes how to install the Port CLI binary.

## Quick Install

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/port-labs/port-cli/main/scripts/install.sh | bash
```

### Windows

Download the latest release from [GitHub Releases](https://github.com/port-labs/port-cli/releases) and extract `port-windows-amd64.exe` to a directory in your PATH.

## Manual Installation

### 1. Download Binary

Download the appropriate binary for your platform from [GitHub Releases](https://github.com/port-labs/port-cli/releases):

- **Linux**: `port-cli_X.X.X_linux_amd64.tar.gz` or `port-cli_X.X.X_linux_arm64.tar.gz`
- **macOS**: `port-cli_X.X.X_darwin_amd64.tar.gz` or `port-cli_X.X.X_darwin_arm64.tar.gz`
- **Windows**: `port-cli_X.X.X_windows_amd64.zip`

### 2. Extract and Install

#### Linux / macOS

```bash
# Extract the archive
tar -xzf port-cli_X.X.X_linux_amd64.tar.gz

# Move to a directory in your PATH (choose one)
sudo mv port /usr/local/bin/port        # System-wide
# OR
mkdir -p ~/.local/bin
mv port ~/.local/bin/port                # User-specific

# Make executable
chmod +x /usr/local/bin/port  # or ~/.local/bin/port

# Add ~/.local/bin to PATH if needed (add to ~/.bashrc or ~/.zshrc)
export PATH="$HOME/.local/bin:$PATH"
```

#### Windows

1. Extract `port-windows-amd64.exe` from the ZIP file
2. Rename it to `port.exe`
3. Move it to a directory in your PATH (e.g., `C:\Program Files\Port CLI\`)
4. Add the directory to your PATH environment variable

### 3. Verify Installation

```bash
port --version
```

Expected output:
```
Port CLI version X.X.X
Build date: YYYY-MM-DDTHH:MM:SSZ
Git commit: abc1234
Go version: go1.21.X
Platform: darwin/arm64
```

## Configuration

After installation, configure your credentials. The CLI supports multiple authentication methods:

### Option 1: Configuration File (Recommended)

```bash
# Initialize config file
port config --init

# Edit ~/.port/config.yaml
```

Example `~/.port/config.yaml`:
```yaml
default_org: production

organizations:
  production:
    client_id: your-client-id
    client_secret: your-client-secret
    api_url: https://api.getport.io/v1
    
  staging:
    client_id: staging-client-id
    client_secret: staging-client-secret
    api_url: https://api.getport.io/v1
```

### Option 2: Environment Variables

```bash
# Base org credentials
export PORT_CLIENT_ID="your-client-id"
export PORT_CLIENT_SECRET="your-client-secret"
export PORT_API_URL="https://api.getport.io/v1"

# Target org credentials (for import/migrate)
export PORT_TARGET_CLIENT_ID="target-client-id"
export PORT_TARGET_CLIENT_SECRET="target-client-secret"
export PORT_TARGET_API_URL="https://api.getport.io/v1"
```

### Option 3: CLI Flags (Recommended for Scripts)

```bash
# Export from base org
port export --client-id YOUR_ID --client-secret YOUR_SECRET -o backup.tar.gz

# Import to target org
port import --target-client-id TARGET_ID --target-client-secret TARGET_SECRET -i backup.tar.gz

# Migrate with both orgs
port migrate \
  --source-org prod --target-org staging \
  --client-id SOURCE_ID --client-secret SOURCE_SECRET \
  --target-client-id TARGET_ID --target-client-secret TARGET_SECRET
```

**Credential Precedence:** CLI flags > Environment variables > Config file > Defaults

## Updating

To update to the latest version:

```bash
# Using npm
npm update -g @port-labs/port-cli

# Using install script
curl -fsSL https://raw.githubusercontent.com/port-labs/port-cli/main/scripts/install.sh | bash

# Or manually download and replace the binary
```

## Building from Source

If you prefer to build from source:

```bash
git clone https://github.com/port-labs/port-cli.git
git clone https://github.com/port-labs/port-cli.git
cd port-cli
make build
./bin/port --help
```

## Troubleshooting

### Binary not found

Make sure the binary is in a directory that's in your PATH:

```bash
# Check current PATH
echo $PATH  # Linux/macOS
echo %PATH% # Windows

# Verify binary location
which port  # Linux/macOS
where port  # Windows
```

### Permission denied

Make sure the binary is executable:

```bash
chmod +x /path/to/port
```

### Authentication errors

The CLI provides clear error messages when credentials are missing. Common solutions:

1. **Check credential precedence**: CLI flags override env vars, which override config file
2. **Verify environment variables**: Ensure they're set correctly (`echo $PORT_CLIENT_ID`)
3. **Check config file**: Run `port config --show` to view current configuration
4. **Use CLI flags**: For scripts, use `--client-id` and `--client-secret` flags directly

See the [Configuration](#configuration) section above or check the main [README.md](README.md) for detailed authentication options.

## Standalone Binary Benefits

The Port CLI is distributed as a standalone binary with no external dependencies:

- ✅ **No runtime dependencies** - Works out of the box
- ✅ **Fast startup** - Instant command execution (~50ms)
- ✅ **Small footprint** - Optimized binary size (~10-15MB)
- ✅ **Cross-platform** - Works on Linux, macOS, and Windows
- ✅ **Easy distribution** - Single file to copy and run
- ✅ **Concurrent operations** - Fast and efficient processing
