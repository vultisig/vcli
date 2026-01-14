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
# 1. Setup: copy vault.env.example and fill in your values
cp local/vault.env.example local/vault.env
# Edit local/vault.env with your VAULT_PATH and VAULT_PASSWORD

# 2. Build and start everything
make local-start

# 3. Import vault (auto-loads vault.env!)
./local/scripts/vcli.sh vault import

# 4. Install plugin using alias (4-party TSS reshare)
./local/scripts/vcli.sh plugin install dca

# 5. Add policy
./local/scripts/vcli.sh policy add --plugin dca -c local/configs/policies/test-one-time-policy.json

# 6. Check status
./local/scripts/vcli.sh report
./local/scripts/vcli.sh policy status <policy-id>

# 7. Stop everything
make local-stop
```

### Plugin Aliases

Use short names instead of full IDs:

| Alias | Full ID | Description |
|-------|---------|-------------|
| `dca` | `vultisig-dca-0000` | Recurring Swaps |
| `fee`, `fees` | `vultisig-fees-feee` | Fee Collection |
| `sends` | `vultisig-recurring-sends-0000` | Recurring Sends |

Run `vcli plugin aliases` to see available aliases.

### Environment Variables

The `vcli.sh` wrapper auto-loads `vault.env` - no manual sourcing needed!

```bash
# vault.env (auto-loaded from local/ or ~/.vultisig/)
VAULT_PATH=/path/to/vault.vult
VAULT_PASSWORD=your-password
```

### Flag Conventions

- `-p, --password` = Vault password (all commands)
- `--plugin` = Plugin ID or alias (no `-p` short flag!)
- `-c, --config` = Config file path

## How It Works

### Step 1: Vault Import

```bash
./local/scripts/vcli.sh vault import
# Or explicitly: ./local/scripts/vcli.sh vault import -f /path/to/vault.vult -p "Password"
```

This:
- Imports the vault locally to `~/.vultisig/vaults/`
- Authenticates with the verifier via 2-party TSS keysign (vcli + Fast Vault Server)
- Stores auth token in `~/.vultisig/auth-token.json`

### Step 2: Plugin Install (4-Party TSS Reshare)

```bash
./local/scripts/vcli.sh plugin install dca
# Or with full ID: ./local/scripts/vcli.sh plugin install vultisig-dca-0000 -p "Password"
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

### Step 3: Policy Add

```bash
./local/scripts/vcli.sh policy add --plugin dca -c policy.json
# Or explicitly: ./local/scripts/vcli.sh policy add --plugin vultisig-dca-0000 -c policy.json -p "Password"
```

This:
1. Fetches policy template from plugin server
2. Signs the policy with 2-of-2 TSS (vcli + Fast Vault)
3. Submits to verifier, which syncs to plugin server
4. Scheduler picks up the policy and executes based on frequency

## E2E Testing Checklist

1. **Setup vault.env**: `cp local/vault.env.example local/vault.env` (edit with your values)
2. **Start services**: `make local-start`
3. **Import vault**: `./local/scripts/vcli.sh vault import`
4. **Install plugin**: `./local/scripts/vcli.sh plugin install dca`
5. **Add policy**: `./local/scripts/vcli.sh policy add --plugin dca -c local/configs/policies/test-one-time-policy.json`
6. **VERIFY EXECUTION**: Wait 30s for scheduler, then check:
   ```bash
   ./local/scripts/vcli.sh policy status <policy-id>
   ./local/scripts/vcli.sh policy transactions <policy-id>
   tail -f /tmp/dca-worker.log
   ```
7. **Check overall status**: `./local/scripts/vcli.sh report`
8. **Uninstall**: `./local/scripts/vcli.sh plugin uninstall dca`
9. **Stop**: `make local-stop`

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

### Password with Special Characters

```bash
# Use double quotes for passwords with special chars
./local/scripts/vcli.sh vault import -f vault.vult -p "YourPassword!"

# -p always means password, --plugin for plugin ID
./local/scripts/vcli.sh policy add --plugin dca -c policy.json -p "YourPassword!"
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
rm -f ~/.vultisig/vcli.json
rm -f ~/.vultisig/auth-token.json
```

## Policy Generate

Generate policy JSON files with easy asset aliases instead of manually writing JSON.

### Usage

```bash
# Basic: ETH to USDC swap
./local/scripts/vcli.sh policy generate --from eth --to usdc --amount 0.01

# With frequency
./local/scripts/vcli.sh policy generate --from usdt --to btc --amount 100 --frequency daily

# Output to file
./local/scripts/vcli.sh policy generate --from eth --to usdc --amount 0.01 -o swap.json

# Chain override (for L2s)
./local/scripts/vcli.sh policy generate --from eth:arbitrum --to usdc:arbitrum --amount 0.001
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--from` | `-f` | (required) | Source asset |
| `--to` | `-t` | (required) | Destination asset |
| `--amount` | `-a` | (required) | Amount in human units (e.g., 0.01 ETH) |
| `--frequency` | | `one-time` | Execution frequency |
| `--plugin` | | `dca` | Plugin ID or alias |
| `--output` | `-o` | stdout | Output file path |

### Asset Aliases

**Native tokens:**
| Alias | Chain |
|-------|-------|
| `eth` | Ethereum |
| `btc` | Bitcoin |
| `sol` | Solana |
| `rune` | THORChain |
| `bnb` | BSC |
| `avax` | Avalanche |
| `matic` | Polygon |

**Stablecoins (Ethereum by default):**
| Alias | Token |
|-------|-------|
| `usdc` | 0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48 |
| `usdt` | 0xdAC17F958D2ee523a2206206994597C13D831ec7 |
| `dai` | 0x6B175474E89094C44Da98b954EesfdDAD3Ef9ebA0 |

**Chain override:** Use `asset:chain` format for L2s:
- `usdc:arbitrum` → USDC on Arbitrum
- `eth:base` → ETH on Base
- `usdc:optimism` → USDC on Optimism

### Frequency Options

| Value | Description |
|-------|-------------|
| `one-time` | Execute once (default) |
| `minutely` | Every 60 seconds |
| `hourly` | Every hour |
| `daily` | Every 24 hours |
| `weekly` | Every 7 days |
| `bi-weekly` | Every 14 days |
| `monthly` | Every 30 days |

### Amount Conversion

Amounts are automatically converted to smallest units:
- ETH/EVM native: 18 decimals (0.01 → 10000000000000000)
- Bitcoin: 8 decimals (0.001 → 100000)
- Solana: 9 decimals (1 → 1000000000)
- USDC/USDT: 6 decimals (100 → 100000000)

### Example Workflow

```bash
# 1. Generate policy
./local/scripts/vcli.sh policy generate --from eth --to usdc --amount 0.01 -o /tmp/swap.json

# 2. Review it
cat /tmp/swap.json

# 3. Add the policy (addresses auto-filled from vault)
./local/scripts/vcli.sh policy add --plugin dca -c /tmp/swap.json
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
- Use `policy generate` to create these files automatically

## Files

| File | Purpose |
|------|---------|
| `local/vcli` | Built binary |
| `local/scripts/vcli.sh` | Wrapper script (auto-loads vault.env, sets DYLD_LIBRARY_PATH) |
| `local/cluster.yaml` | Configuration (copy from .example) |
| `local/vault.env` | Vault credentials - auto-loaded by vcli.sh (copy from .example) |
| `local/vault.env.example` | Template for vault.env |
| `local/configs/` | Test configuration files |
| `~/.vultisig/vaults/` | Local vault storage |
| `~/.vultisig/vault.env` | Alternative vault.env location (auto-loaded if local/ not found) |
| `~/.vultisig/vcli.json` | vcli configuration and auth token |
