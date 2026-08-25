package config

import "fmt"

// Canonical CLI command strings referenced in user-facing auth errors.
const (
	CmdAuthLogin   = "port auth login --org ORG_NAME"
	CmdConfigInit  = "port config --init"
	CmdConfigShow  = "port config --show"
	CmdExportCreds = "port export --client-id YOUR_CLIENT_ID --client-secret YOUR_CLIENT_SECRET"
	CmdImportCreds = "port import --target-client-id YOUR_CLIENT_ID --target-client-secret YOUR_CLIENT_SECRET"
)

// MissingAuthCredentialsMessage is shown when no organization or credentials are configured.
func MissingAuthCredentialsMessage(configPath string) string {
	if configPath == "" {
		configPath = DefaultConfigPath()
	}
	return fmt.Sprintf(`missing authentication credentials

To authenticate, use one of the following methods:

1. Login with the CLI (recommended):
   %s

2. CLI flags:
   %s

3. Environment variables:
   export PORT_CLIENT_ID="your-client-id"
   export PORT_CLIENT_SECRET="your-client-secret"

4. Configuration file:
   Run: %s
   Then edit: %s`, CmdAuthLogin, CmdExportCreds, CmdConfigInit, configPath)
}

// MissingCredentialsForOrgMessage is shown when partial CLI/env overrides omit client credentials.
func MissingCredentialsForOrgMessage(orgType, configPath string) string {
	return fmt.Sprintf(`missing credentials for %s org

To authenticate, provide credentials using one of these methods:

1. Login with the CLI (recommended):
   %s

2. CLI flags:
   %s
   %s

3. Environment variables:
   export PORT_CLIENT_ID="your-client-id"
   export PORT_CLIENT_SECRET="your-client-secret"
   export PORT_TARGET_CLIENT_ID="your-target-client-id"
   export PORT_TARGET_CLIENT_SECRET="your-target-client-secret"

4. Configuration file:
   Run: %s
   Then edit: %s`, orgType, CmdAuthLogin, CmdExportCreds, CmdImportCreds, CmdConfigInit, configPath)
}
