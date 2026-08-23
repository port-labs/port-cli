# Port CLI (npm package)

This is the npm package distribution of Port CLI. For the full documentation, see the [main repository](https://github.com/port-labs/port-cli).

## Installation

```bash
# Global installation
npm install -g @port-labs/port-cli

# Or use with npx (no installation needed)
npx @port-labs/port-cli --version

# Or install locally in your project
npm install @port-labs/port-cli
```

## Usage

After installation, use the `port` command:

```bash
port --version
port config --init
port export --output backup.tar.gz
```

## Verifying Installation

To verify that Port CLI is installed correctly:

```bash
# Check if the command is available
which port

# Check the version
port --version

# Test with help command
port --help
```

If installation was successful, you should see output from the `port` command. If you get a "command not found" error, make sure:

1. The npm global bin directory is in your PATH
2. You installed with `-g` flag for global installation
3. You've restarted your terminal after installation

To check where npm installs global packages:

```bash
npm root -g
```

## Platform Support

This package includes binaries for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

The correct binary for your platform will be automatically selected during installation.
