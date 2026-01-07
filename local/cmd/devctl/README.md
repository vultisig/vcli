# devctl - Vultisig Plugin Development CLI

A command-line tool for testing Vultisig plugins locally without the browser extension or marketplace UI.

## Prerequisites

1. **go-wrappers CGO library**: Required for TSS operations
   ```bash
   export DYLD_LIBRARY_PATH=/Users/dev/dev/vultisig/go-wrappers/includes/darwin/:$DYLD_LIBRARY_PATH
   ```

2. **Running services**:
   - Verifier server (`VS_CONFIG_NAME=devenv/config/verifier go run ./cmd/verifier`)
   - Verifier worker (`VS_WORKER_CONFIG_NAME=devenv/config/worker go run ./cmd/worker`)
   - Plugin server (e.g., DCA plugin on port 8082, Fee plugin on port 8085)
   - Docker services (PostgreSQL, Redis, MinIO)

3. **Fast Vault**: Your vault must be created with Fast Vault feature (cloud backup)

## Installation

```bash
cd verifier/cmd/devctl
CGO_ENABLED=1 go build -o devctl .
```

## Quick Start

### 1. Initialize Development Environment

Start the local infrastructure:

```bash
./devctl services init
```

This starts Docker containers for PostgreSQL, Redis, and MinIO.

### 2. Import a Vault

Import a vault backup file (`.vult`) from VultiConnect or mobile app:

```bash
./devctl vault import --file /path/to/vault-share.vult --password "your-password"
```

The vault must be a Fast Vault (created with cloud backup). The CLI will verify this and automatically authenticate with the verifier.

### 3. Install a Plugin

Install a plugin by performing a 4-party TSS reshare:

```bash
./devctl plugin install vultisig-dca-0000 --password "your-password"
```

This performs a 4-party reshare with:
- **CLI** (your local vault share)
- **Fast Vault Server** (production cloud backup)
- **Verifier** (local verifier service)
- **Plugin** (the plugin you're installing)

### 4. Create a Policy

Create a policy for the installed plugin:

```bash
./devctl policy create --plugin vultisig-dca-0000 --config policy.json --password "your-password"
```

Example `policy.json` for DCA plugin:
```json
{
  "recipe": {
    "from_chain": "ethereum",
    "to_chain": "ethereum",
    "from_asset": "ETH",
    "to_asset": "USDC",
    "amount": "0.001",
    "frequency": "daily"
  },
  "billing": {
    "type": "one_time",
    "amount": 0
  }
}
```

### 5. Verify Installation

Check databases to verify the reshare stored key shares:

```bash
# Check verifier stored the installation
docker exec vultisig-postgres psql -U vultisig -d "vultisig-verifier" \
  -c "SELECT * FROM plugin_installations;"

# Check plugin stored its key share (varies by plugin)
docker exec vultisig-postgres psql -U vultisig -d "vultisig-dca" \
  -c "SELECT * FROM plugin_policies;"
```

## Commands Reference

### Vault Commands

```bash
# List local vaults
./devctl vault list

# Import a vault backup
./devctl vault import --file <file.vult> --password <password>

# Export current vault to file
./devctl vault export [--output <file.json>]

# Show current vault information
./devctl vault info

# Set active vault (by public key prefix)
./devctl vault use <public-key-prefix>

# Generate a new vault with Fast Vault Server (2-of-2)
./devctl vault generate [--name <vault-name>] [--dry-run]

# Show vault addresses on chains
./devctl vault address [--chain <chain>]

# Show vault balances on chains
./devctl vault balance [--chain <chain>]

# Sign a message using TSS keysign
./devctl vault keysign --message <hex-hash> --password <password> [--derive <path>] [--eddsa]

# Reshare vault to add verifier and plugin
./devctl vault reshare --plugin <plugin-id> --password <password> [--verifier <url>]
```

### Plugin Commands

```bash
# List available plugins
./devctl plugin list

# Show plugin details
./devctl plugin info <plugin-id>

# Install plugin (4-party reshare)
./devctl plugin install <plugin-id> --password <password>

# Uninstall plugin
./devctl plugin uninstall <plugin-id>

# Show plugin recipe specification
./devctl plugin spec <plugin-id>
```

### Policy Commands

```bash
# List policies for a plugin
./devctl policy list --plugin <plugin-id>

# Create a new policy
./devctl policy create --plugin <plugin-id> --config <policy.json> --password <password>

# Show policy details
./devctl policy info <policy-id>

# Delete a policy
./devctl policy delete <policy-id>

# Show policy transaction history
./devctl policy history <policy-id>
```

### Authentication Commands

```bash
# Authenticate with verifier using TSS keysign
./devctl auth login [--vault <vault-id>] [--password <password>]

# Show current authentication status
./devctl auth status

# Clear stored authentication token
./devctl auth logout
```

### Service Management Commands

```bash
# Initialize local development environment (start Docker infrastructure)
./devctl services init

# Start services
./devctl services start --services <service1,service2,...>
./devctl services start --all

# Stop services
./devctl services stop [--all]

# View service logs
./devctl services logs [--service <name>] [--follow]
```

Available services: `infra`, `verifier`, `worker`, `fee`, `dca`, `dca-scheduler`, `dca-worker`

### Verification Commands

```bash
# Check health of all services
./devctl verify health

# Verify policy status and execution state
./devctl verify policy <policy-id>

# Check transaction history
./devctl verify transactions --policy <policy-id> [--limit <n>]
./devctl verify transactions --plugin <plugin-id> [--limit <n>]
```

### Status Command

```bash
# Check status of all services, infrastructure, and current vault
./devctl status
```

### Report Command

```bash
# Generate comprehensive validation report
./devctl report
```

The report shows:
- Service status (verifier, DCA plugin, workers) with PIDs
- Infrastructure status (PostgreSQL, Redis, MinIO)
- Vault details (name, keys, signers, auth token validity)
- Plugin installations from database
- MinIO storage contents (keyshare files with sizes)
- Useful inspection commands for debugging

## Configuration

Configuration is stored in `~/.vultisig/devctl.json` and is managed automatically by the CLI.

Default configuration values:
```json
{
  "verifier_url": "http://localhost:8080",
  "fee_plugin_url": "http://localhost:8085",
  "dca_plugin_url": "http://localhost:8082",
  "relay_server": "https://api.vultisig.com/router",
  "database_dsn": "postgres://vultisig:vultisig@localhost:5432/vultisig-verifier?sslmode=disable",
  "redis_uri": "redis://:vultisig@localhost:6379",
  "minio_host": "http://localhost:9000"
}
```

The config file also stores:
- Current vault information (`vault_name`, `public_key_ecdsa`, `public_key_eddsa`)
- Authentication token (`auth_token`, `auth_expires_at`)

Vaults are stored in `~/.vultisig/vaults/` directory.

## Progress Indicators

The CLI provides detailed progress output during operations:

### Plugin Install Progress
```
Installing plugin vultisig-dca-0000...
  Vault: iPhone Hot Vault (027b25c8...)
  Verifier: http://localhost:8080
  Fast Vault: Yes

Checking plugin availability...
  Plugin found!

Initiating 4-party TSS reshare...
  Parties: CLI + Fast Vault Server + Verifier + Plugin

INFO Starting DKLS reshare session session_id=xxx old_parties=[...] plugin_id=xxx
INFO Registering session session=xxx key=xxx
INFO Requesting Fast Vault Server to join reshare...
INFO Requesting Verifier to join reshare (with plugin)...
INFO Waiting for all parties to join... expected=4
INFO All parties joined, starting reshare session parties=[...]
INFO Running DKLS reshare protocol (ECDSA)...
INFO Running DKLS reshare protocol (EdDSA)...
INFO Reshare completed successfully ecdsa=xxx... eddsa=xxx...

✓ Plugin installed successfully!
  New signers: [party1 party2 party3 party4]

Next step: Create a policy with 'devctl policy create --plugin vultisig-dca-0000'
```

### Policy Creation Progress
```
Creating policy for plugin vultisig-dca-0000...
  Vault: iPhone Hot Vault (027b25c8...)
  Config: /path/to/policy.json
  Verifier: http://localhost:8080

Signing policy with TSS keysign (with Verifier)...
  Message to sign: base64-encoded-policy...

INFO Starting keysign with verifier session_id=xxx plugin_id=xxx
INFO Requesting Verifier to join keysign for policy...
INFO Waiting for Verifier to join...
INFO All parties joined, starting keysign parties=[...]
INFO Running DKLS keysign protocol with Verifier...

✓ Policy created successfully!
  Policy ID: policy-xxx
```

## Troubleshooting

### Fast Vault Server returns 500
- Ensure your vault is a Fast Vault (created with cloud backup)
- Verify password is correct
- Check that `reshare_type: 1` is being sent for plugin reshare

### "waiting for more parties" hangs
- Check that verifier worker is running
- Check that plugin server is running and accessible
- Verify session IDs match across all logs

### "NoSuchKey" error in worker logs
- This is expected for new parties joining reshare
- The verifier/plugin don't have existing vault files for a new reshare

### "setup message not found"
- Ensure the CLI (initiator) is running the actual TSS protocol
- Check DYLD_LIBRARY_PATH is set correctly

## Development Environment Setup

See `/devenv/README.md` for full development environment setup including:
- Docker services (PostgreSQL, Redis, MinIO)
- Database seeding
- Plugin configuration
- Verifier configuration

## Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   devctl    │────▶│  Fast Vault API  │────▶│  Fast Vault DB  │
│   (CLI)     │     │  (production)    │     │  (production)   │
└─────────────┘     └──────────────────┘     └─────────────────┘
       │
       │ TSS messages via Relay
       ▼
┌─────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   Relay     │◀───▶│  Verifier Worker │────▶│  MinIO/Postgres │
│   Server    │     │  (local)         │     │  (local)        │
└─────────────┘     └──────────────────┘     └─────────────────┘
       │
       ▼
┌─────────────┐     ┌──────────────────┐
│   Plugin    │────▶│  Plugin DB       │
│   Server    │     │  (local)         │
└─────────────┘     └──────────────────┘
```

The 4-party reshare protocol:
1. CLI creates session and invites parties
2. Fast Vault Server joins (decrypts vault with password)
3. Verifier Worker joins (creates new key share)
4. Plugin Server joins (creates new key share)
5. All parties run DKLS QC (reshare) protocol
6. New key shares are stored by each party
