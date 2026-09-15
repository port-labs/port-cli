# Port CLI API Commands Reference

This document shows all available API commands and how to use them.

## Quick Reference

### Blueprints
```bash
port api blueprints list                    # List all
port api blueprints get <id>                # Get one
port api blueprints create --data <file>    # Create
port api blueprints update <id> --data <file>  # Update
port api blueprints delete <id>             # Delete
```

### Entities
```bash
port api entities list [--blueprint <id>] [--limit N] [--all]  # Search-backed list (default limit 100)
port api entities search <blueprint> --query '<json>' [--unwrap entities]
port api entities count <blueprint> --query '<json>' [--unwrap count]
port api entities search-global --query '<json>'  # Cross-blueprint (top-level combinator/rules)
port api entities get <blueprint> <entity>  # Get one
port api entities create <blueprint> --data <file>  # Create
port api entities update <blueprint> <entity> --data <file>  # Update
port api entities delete <blueprint> <entity>  # Delete
```

#### Entity search (PM OS / quarterly planning)

Blueprint-scoped search wraps filters in a `query` object. Global search uses top-level `combinator` and `rules` (no `query` wrapper).

**Q4 planning priorities (Doing):**
```bash
port api entities search planning_priority \
  --query '{"combinator":"and","rules":[
    {"relation":"quarter_r","operator":"=","value":"2026_q_4"},
    {"property":"priority_decision","operator":"=","value":"Doing"}
  ]}' \
  --limit 100 \
  --include '$identifier' '$title' priority_decision allocation category \
  --unwrap entities
```

**Count Q4 priorities:**
```bash
port api entities count planning_priority \
  --query '{"combinator":"and","rules":[{"relation":"quarter_r","operator":"=","value":"2026_q_4"}]}' \
  --unwrap count
```

**AI team quarter plan row:**
```bash
port api entities search team_quarter_planning \
  --query '{"combinator":"and","rules":[
    {"relation":"quarter_r","operator":"=","value":"2026_q_4"},
    {"relation":"team_r","operator":"=","value":"ai_team"}
  ]}' \
  --include '$identifier' insights capacity available_capacity \
  --unwrap entities
```

**Distribution by priority_decision (`groupBy` / MCP parity):**
```bash
port api entities search planning_priority \
  --query '{"combinator":"and","rules":[{"relation":"quarter_r","operator":"=","value":"2026_q_4"}]}' \
  --group-by property:priority_decision \
  --unwrap groups
```

#### MCP vs public REST

| Flags / behavior | Backend route |
|------------------|---------------|
| `--query`, `--include`, `--exclude`, `--limit`, `--from`, `--all` | `POST /v1/blueprints/:id/entities/search` |
| `--count-only` (alone) | `POST /v1/blueprints/:id/entities/search/count` |
| `--group-by`, `--group-sort`, `--sort`, `--identifiers` | `POST /v1/blueprints/:id/entities/top-search` (MCP parity) |
| `search-global` | `POST /v1/entities/search` |

Port MCP `list_entities` matches the top-search contract for grouping and sorting. The public OpenAPI page for blueprint search may not list every field MCP accepts.

### Common Flags
- `--org <name>` - Organization name
- `--format json|yaml` - Output format
- `--data <file>` - Input JSON file
- `--force` - Skip confirmations

---

## Command Structure

```
port api <resource> <action> [arguments] [flags]
```

## Available Commands

### Blueprints

#### List all blueprints
```bash
port api blueprints list [--org <org-name>] [--format json|yaml]
```

**Example:**
```bash
port api blueprints list
port api blueprints list --format yaml
port api blueprints list --org production
```

#### Get a specific blueprint
```bash
port api blueprints get <blueprint-id> [--org <org-name>] [--format json|yaml]
```

**Example:**
```bash
port api blueprints get service
port api blueprints get service --format yaml
```

#### Create a blueprint
```bash
port api blueprints create --data <file.json> [--org <org-name>]
```

**Example:**
```bash
# Create blueprint.json first
cat > blueprint.json << 'EOF'
{
  "identifier": "service",
  "title": "Service",
  "icon": "Service",
  "schema": {
    "properties": {
      "name": {
        "type": "string",
        "title": "Name"
      }
    }
  }
}
EOF

port api blueprints create --data blueprint.json
```

#### Update a blueprint
```bash
port api blueprints update <blueprint-id> --data <file.json> [--org <org-name>]
```

**Example:**
```bash
port api blueprints update service --data updated-blueprint.json
```

#### Delete a blueprint
```bash
port api blueprints delete <blueprint-id> [--org <org-name>] [--force]
```

**Example:**
```bash
port api blueprints delete service
port api blueprints delete service --force  # Skip confirmation
```

### Entities

#### List entities
```bash
port api entities list [--blueprint <blueprint-id>] [--org <org-name>] [--format json|yaml]
```

**Examples:**
```bash
# List all entities across all blueprints
port api entities list

# List entities for a specific blueprint
port api entities list --blueprint service

# List with YAML output
port api entities list --blueprint service --format yaml
```

#### Get a specific entity
```bash
port api entities get <blueprint-id> <entity-id> [--org <org-name>] [--format json|yaml]
```

**Example:**
```bash
port api entities get service my-service-1
port api entities get service my-service-1 --format yaml
```

#### Create an entity
```bash
port api entities create <blueprint-id> --data <file.json> [--org <org-name>]
```

**Example:**
```bash
# Create entity.json
cat > entity.json << 'EOF'
{
  "identifier": "my-service-1",
  "title": "My Service",
  "properties": {
    "name": "My Service"
  }
}
EOF

port api entities create service --data entity.json
```

#### Update an entity
```bash
port api entities update <blueprint-id> <entity-id> --data <file.json> [--org <org-name>]
```

**Example:**
```bash
port api entities update service my-service-1 --data updated-entity.json
```

#### Delete an entity
```bash
port api entities delete <blueprint-id> <entity-id> [--org <org-name>] [--force]
```

**Example:**
```bash
port api entities delete service my-service-1
port api entities delete service my-service-1 --force
```

## Common Flags

All commands support these flags:

- `--org <org-name>` - Specify organization (uses default from config if not specified)
- `--format json|yaml` - Output format (default: json)
- `--data <file>` - Input data file for create/update operations (JSON format)
- `--force` - Skip confirmation prompts (for delete operations)

## Global Flags

These flags work with all commands:

- `--config <path>` - Path to configuration file (default: ~/.port/config.yaml)
- `--client-id <id>` - Override client ID from config
- `--client-secret <secret>` - Override client secret from config
- `--api-url <url>` - Override API URL from config

**Example:**
```bash
port api blueprints list \
  --config /path/to/config.yaml \
  --client-id my-client-id \
  --client-secret my-secret \
  --api-url https://api.getport.io/v1
```

## Configuration

Commands use credentials from (in priority order):
1. CLI flags (`--client-id`, `--client-secret`, `--api-url`)
2. Environment variables (`PORT_CLIENT_ID`, `PORT_CLIENT_SECRET`, `PORT_API_URL`)
3. Configuration file (`~/.port/config.yaml`)

## Examples

### Complete Workflow

```bash
# 1. List all blueprints
port api blueprints list

# 2. Get a specific blueprint
port api blueprints get service

# 3. Create an entity
port api entities create service --data entity.json

# 4. List entities for the blueprint
port api entities list --blueprint service

# 5. Get the entity
port api entities get service my-service-1

# 6. Update the entity
port api entities update service my-service-1 --data updated-entity.json

# 7. Delete the entity
port api entities delete service my-service-1
```

### Working with Multiple Organizations

```bash
# List blueprints in production
port api blueprints list --org production

# Create entity in staging
port api entities create service --data entity.json --org staging

# Get entity from production
port api entities get service my-service-1 --org production
```

### Output Formatting

```bash
# JSON output (default)
port api blueprints list

# YAML output
port api blueprints list --format yaml

# Pipe to jq for filtering
port api blueprints list | jq '.[] | select(.identifier == "service")'

# Save to file
port api blueprints list --format yaml > blueprints.yaml
```

## Error Handling

Commands will show descriptive error messages:

```bash
$ port api blueprints get nonexistent
Error: API request failed: 404 Not Found - {"ok":false,"error":"not_found","message":"Blueprint not found"}
```

## Help

Get help for any command:

```bash
port api --help
port api blueprints --help
port api blueprints list --help
```

