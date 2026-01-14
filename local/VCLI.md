# vcli - Vultisig Development CLI

Development CLI for testing Vultisig plugins locally. Handles vault management, plugin installation (TSS reshare), and policy creation.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Docker Infrastructure                     │
├─────────────────┬─────────────────┬─────────────────────────┤
│   PostgreSQL    │     Redis       │         MinIO           │
│   localhost:5432│  localhost:6379 │     localhost:9000      │
└─────────────────┴─────────────────┴─────────────────────────┘
         │                 │                    │
         ▼                 ▼                    ▼
┌─────────────────────────────────────────────────────────────┐
│                      Go Services                             │
├─────────────────┬─────────────────┬─────────────────────────┤
│    Verifier     │ Verifier Worker │      DCA Plugin         │
│  localhost:8080 │   (background)  │    localhost:8082       │
│                 │                 ├─────────────────────────┤
│                 │                 │  DCA Worker, Scheduler  │
│                 │                 │  TX-Indexer (background)│
└─────────────────┴─────────────────┴─────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│           External: Fast Vault Server + Relay               │
│              (production or local, per cluster.yaml)        │
└─────────────────────────────────────────────────────────────┘
```

### Services & Ports

| Service | Port | Description |
|---------|------|-------------|
| PostgreSQL | 5432 | Database for all services |
| Redis | 6379 | Task queue and caching |
| MinIO | 9000 | S3-compatible keyshare storage |
| MinIO Console | 9090 | MinIO web UI |
| Verifier API | 8080 | Main verifier service |
| Verifier Worker Metrics | 8089 | Worker prometheus metrics |
| DCA Server | 8082 | DCA plugin API |
| DCA Worker Metrics | 8183 | DCA worker prometheus metrics |
| DCA Scheduler Metrics | 8185 | Scheduler prometheus metrics |
| DCA TX-Indexer Metrics | 8187 | TX indexer prometheus metrics |

## Quick Start

```bash
# Build and start everything
make local-start

# Import vault (uses vault.env)
set -a && source ./local/vault.env && set +a
./local/vcli.sh vault import -f "$VAULT_PATH" -p "$VAULT_PASSWORD" --force

# Install plugin (4-party TSS reshare)
./local/vcli.sh plugin install vultisig-dca-0000 -p "$VAULT_PASSWORD"

# Create policy
./local/vcli.sh policy create -p vultisig-dca-0000 -c local/configs/test-one-time-policy.json --password "$VAULT_PASSWORD"

# Check status
./local/vcli.sh report
./local/vcli.sh policy status <policy-id>

# Stop everything
make local-stop
```

## How It Works

### Step 1: Vault Import

```bash
./local/vcli.sh vault import -f /path/to/vault.vult -p "Password"
```

This:
- Imports the vault locally to `~/.vultisig/vaults/`
- Authenticates with the verifier via 2-party TSS keysign (vcli + Fast Vault Server)
- Stores auth token in `~/.vultisig/auth-token.json`

### Step 2: Plugin Install (4-Party TSS Reshare)

```bash
./local/vcli.sh plugin install vultisig-dca-0000 -p "Password"
```

This performs a **4-party TSS reshare** (~20 seconds):

```
Before (2-of-2):                 After (2-of-2 + 2-of-4):
┌─────────────┐                  ┌─────────────┐
│    vcli     │ ──────────────▶  │    vcli     │ (keeps 2-of-2 share)
│  (share 1)  │                  └─────────────┘
└─────────────┘                         │
       │                                │ User auth still 2-of-2
       │                                ▼
┌─────────────┐                  ┌─────────────┐
│ Fast Vault  │ ──────────────▶  │ Fast Vault  │ (keeps 2-of-2 share)
│  (share 2)  │                  └─────────────┘
└─────────────┘
                                        │
                    Reshare creates     │ Plugin ops use 2-of-4
                    new 2-of-4 shares   ▼
                                 ┌─────────────┐
                                 │  Verifier   │ (new 2-of-4 share → MinIO)
                                 │   Worker    │
                                 └─────────────┘
                                        │
                                        ▼
                                 ┌─────────────┐
                                 │ DCA Plugin  │ (new 2-of-4 share → MinIO)
                                 │   Worker    │
                                 └─────────────┘
```

- User auth remains 2-of-2 (vcli + Fast Vault)
- Plugin operations use 2-of-4 (Verifier Worker + Plugin Worker)
- Keyshares stored in MinIO (~458KB each)

### Step 3: Policy Create

```bash
./local/vcli.sh policy create --plugin vultisig-dca-0000 --config policy.json --password "Password"
```

This:
1. Fetches policy template from plugin server
2. Signs the policy with 2-of-2 TSS (vcli + Fast Vault)
3. Submits to verifier, which syncs to plugin server
4. Scheduler picks up the policy and executes based on frequency

## E2E Testing Checklist

1. **Start services**: `make local-start`
2. **Import vault**:
   ```bash
   set -a && source ./local/vault.env && set +a
   ./local/vcli.sh vault import -f "$VAULT_PATH" -p "$VAULT_PASSWORD" --force
   ```
3. **Install plugin**: `./local/vcli.sh plugin install vultisig-dca-0000 -p "$VAULT_PASSWORD"`
4. **Create policy**: `./local/vcli.sh policy create -p vultisig-dca-0000 -c <config.json> --password "$VAULT_PASSWORD"`
5. **VERIFY EXECUTION**: Wait 30s for scheduler, then check:
   ```bash
   ./local/vcli.sh policy status <policy-id>
   ./local/vcli.sh policy transactions <policy-id>
   tail -f /tmp/dca-worker.log
   ```
6. **Check overall status**: `./local/vcli.sh report`
7. **Uninstall**: `./local/vcli.sh plugin uninstall vultisig-dca-0000`
8. **Stop**: `make local-stop`

## Configuration

### cluster.yaml

Controls which services run locally vs use production:

```yaml
services:
  relay: production        # or "local" to run from source
  vultiserver: production  # or "local" to run from source
  verifier: local
  dca_server: local
  dca_worker: local
```

### Critical: Encryption Secret

The encryption secret **must match** across all services for vault decryption:

- `verifier.json`: `"encryption_secret": "dev-encryption-secret-32b"`
- `dca-server.env`: `SERVER_ENCRYPTIONSECRET=dev-encryption-secret-32b`
- `dca-worker.env`: `VAULTSERVICE_ENCRYPTIONSECRET=dev-encryption-secret-32b`

### envconfig Naming Convention

The DCA services use `kelseyhightower/envconfig` which requires specific env var naming:

```bash
# Correct (struct field names concatenated, no extra underscores)
BLOCKSTORAGE_HOST=http://localhost:9000
BLOCKSTORAGE_ACCESSKEY=minioadmin
VAULTSERVICE_ENCRYPTIONSECRET=secret
SERVER_ENCRYPTIONSECRET=secret

# Wrong (will NOT work)
BLOCK_STORAGE_HOST=http://localhost:9000
BLOCK_STORAGE_ACCESS_KEY=minioadmin
```

### Queue Isolation

The verifier and DCA plugin use **separate task queues** to prevent task stealing:

| Service | Queue Name | Env Var |
|---------|-----------|---------|
| Verifier Worker | `default_queue` | (default) |
| DCA Server | `dca_plugin_queue` | `SERVER_TASKQUEUENAME` |
| DCA Worker | `dca_plugin_queue` | `TASK_QUEUE_NAME` |

**Critical**: Both DCA Server and DCA Worker must use the same queue name. The server enqueues tasks, the worker consumes them. If they don't match, tasks will be orphaned.

If verifier and DCA workers share queues, they'll steal each other's tasks and the 4-party reshare will fail (only 3 parties will join).

## Common Gotchas

### Environment Variables

Always use `set -a` to export environment variables:
```bash
# WRONG - variables not exported
source ./local/vault.env

# CORRECT - variables exported to subprocesses
set -a && source ./local/vault.env && set +a
```

### Password with Special Characters

```bash
# Use double quotes for passwords with special chars
./local/vcli.sh vault import -f vault.vult -p "Ashley89!"

# For policy create, use --password (not -p which is for --plugin)
./local/vcli.sh policy create --plugin vultisig-dca-0000 --config policy.json --password "YourPassword!"
```

### Billing Array

Use `"billing": []` for plugins with no pricing (like vultisig-dca-0000):
```json
{
  "recipe": { ... },
  "billing": []
}
```

If you get: `billing policies count (1) does not match plugin pricing count (0)`, your billing array doesn't match the plugin's pricing.

### Scheduler Delay

The DCA scheduler polls every 30 seconds. For testing:
- Use `"frequency": "one-time"` for immediate execution
- Check worker logs: `tail -f /tmp/dca-worker.log`
- Use `policy trigger` to force immediate execution

### Policy Frequency Values

- `"one-time"` - Execute once immediately
- `"daily"` - Execute every 24 hours
- `"weekly"` - Execute every 7 days
- `"monthly"` - Execute every 30 days

## Troubleshooting

### Library Not Found Error

```
dyld: Library not loaded: libgodkls.dylib
```

**Solution:** The vcli.sh wrapper should handle this, but if running manually:
```bash
export DYLD_LIBRARY_PATH=/path/to/go-wrappers/includes/darwin:$DYLD_LIBRARY_PATH
```

### Port Conflicts

```bash
# Check what's using ports
lsof -i :5432
lsof -i :8080
lsof -i :8082

# Force stop everything
make local-stop
```

### Plugin Install Fails / TSS Stuck

1. Check all services are running:
   ```bash
   curl http://localhost:8080/plugins  # Verifier
   curl http://localhost:8082/healthz  # DCA Server
   ```

2. Check worker logs:
   ```bash
   tail -f /tmp/worker.log      # Verifier worker
   tail -f /tmp/dca-worker.log  # DCA worker
   ```

3. Verify queue separation:
   ```bash
   docker exec vultisig-redis redis-cli -a vultisig KEYS "asynq:*"
   ```

### TSS Reshare Stuck at 3 Parties

The DCA plugin worker isn't processing tasks. This happens when:
- Queue mismatch: DCA server enqueues to one queue, worker listens on another
- Task stealing: Both verifier and DCA workers listen on the same queue

**Diagnosis:**
```bash
# Check Redis queues - should see both default_queue and dca_plugin_queue
docker exec vultisig-redis redis-cli -a vultisig KEYS "asynq:*queue*"

# Check if task is pending in DCA queue
docker exec vultisig-redis redis-cli -a vultisig LRANGE "asynq:{dca_plugin_queue}:pending" 0 -1

# Check DCA worker is consuming from correct queue
grep "queue" /tmp/dca-worker.log
```

**Fix:** Ensure queue names match:
```bash
# dca-server.env
SERVER_TASKQUEUENAME=dca_plugin_queue

# dca-worker.env
TASK_QUEUE_NAME=dca_plugin_queue
```

### Policy Creation Fails with "Invalid policy signature"

Check DCA server logs:
```bash
tail -20 /tmp/dca.log
```

Common causes:
- Missing `SERVER_ENCRYPTIONSECRET` in dca-server.env
- Wrong `BLOCKSTORAGE_*` env var names
- Encryption secret mismatch between verifier and DCA server

### MinIO Access Denied

If keyshares show "Not found" or "Access Denied":
```bash
docker exec vultisig-minio mc alias set local http://localhost:9000 minioadmin minioadmin
docker exec vultisig-minio mc ls local/vultisig-verifier/
```

### Rule Validation Errors

If you see errors like `tx target is wrong`:
- The policy rules validate that transactions match expected parameters
- This can happen when DEX router addresses change or get upgraded
- Check the rule target vs actual target in the error message

## Useful Commands

### Logs
```bash
tail -f /tmp/verifier.log      # Verifier server
tail -f /tmp/worker.log        # Verifier worker
tail -f /tmp/dca.log           # DCA plugin server
tail -f /tmp/dca-worker.log    # DCA plugin worker
tail -f /tmp/dca-scheduler.log # DCA scheduler
tail -f /tmp/dca-tx-indexer.log # DCA TX indexer
```

### Database
```bash
# Connect to verifier DB
docker exec -it vultisig-postgres psql -U vultisig -d vultisig-verifier

# Connect to DCA DB
docker exec -it vultisig-postgres psql -U vultisig -d vultisig-dca

# Check plugin installations
docker exec vultisig-postgres psql -U vultisig -d vultisig-verifier -c \
  "SELECT plugin_id, public_key, installed_at FROM plugin_installations;"

# Check scheduler
docker exec vultisig-postgres psql -U vultisig -d vultisig-dca -c \
  "SELECT * FROM scheduler ORDER BY next_execution LIMIT 5;"

# Check transactions
docker exec vultisig-postgres psql -U vultisig -d vultisig-dca -c \
  "SELECT * FROM tx_indexer ORDER BY created_at DESC LIMIT 5;"
```

### Redis
```bash
docker exec vultisig-redis redis-cli -a vultisig KEYS '*'
```

### MinIO
```bash
# Console: http://localhost:9090 (minioadmin/minioadmin)

# List keyshares
docker exec vultisig-minio mc ls local/vultisig-verifier/
docker exec vultisig-minio mc ls local/vultisig-dca/
```

## Cleanup

```bash
# Stop services only
make local-stop

# Stop and remove all Docker data (full reset)
make local-stop
docker compose -f local/configs/docker-compose.yaml down -v

# Clear local vault data (start fresh)
rm -rf ~/.vultisig/vaults/*
rm -f ~/.vultisig/devctl.json
rm -f ~/.vultisig/auth-token.json
```

## Test Policy Examples

### ETH to USDC (one-time)
```json
{
  "recipe": {
    "from": { "chain": "Ethereum", "token": "", "address": "0x..." },
    "to": { "chain": "Ethereum", "token": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", "address": "0x..." },
    "fromAmount": "1000000000000000",
    "frequency": "one-time"
  },
  "billing": []
}
```

Notes:
- `token: ""` means native token (ETH)
- `token: "0x..."` means ERC20 token address
- `fromAmount` is in wei (1000000000000000 = 0.001 ETH)
- `billing: []` for plugins with no pricing configured

## Files

| File | Purpose |
|------|---------|
| `local/vcli` | Built binary |
| `local/vcli.sh` | Wrapper script (sets DYLD_LIBRARY_PATH) |
| `local/cluster.yaml` | Configuration (copy from .example) |
| `local/vault.env` | Vault credentials (copy from .example) |
| `local/configs/` | Test configuration files |
| `~/.vultisig/vaults/` | Local vault storage |
| `~/.vultisig/auth-token.json` | Authentication token cache |
| `~/.vultisig/devctl.json` | vcli configuration |
