# vcli

Vultisig CLI - Local development environment for testing Vultisig plugins with Docker-based infrastructure.

---

## IMPORTANT: Read Before Starting

**You MUST follow the E2E testing flow exactly as documented below.** The flow is:

```
START → IMPORT → DETAILS → INSTALL → [ GENERATE → ADD → MONITOR ] → [ repeat for more policies ]
                ↓                                                              ↑_________________↲
         (view addresses)
```

The bracketed steps (policy testing) can be repeated as many times as needed. Everything else runs once per test cycle. When completely done testing, proceed to cleanup (DELETE → UNINSTALL → STOP).

**After IMPORT:** Run `vcli vault details` to view your wallet addresses and balances before proceeding to INSTALL.

**Do NOT:**
- Restart mid-way through a test cycle
- Move keyshare files manually between directories
- Edit database records directly (no manual SQL inserts/updates)
- Skip steps (except repeating the policy loop)
- Re-use state from a previous failed run

**If something fails:** Run `make stop` (cleans all state) and start fresh from Step 1.

---

## Prerequisites

- **Go 1.23+** - https://go.dev/dl/
- **Docker** - https://docs.docker.com/get-docker/
- **Docker Compose** - Usually included with Docker Desktop

## Dependencies

Clone all required repositories into the same parent directory:

```bash
mkdir -p ~/dev/vultisig && cd ~/dev/vultisig

# This repo
git clone https://github.com/vultisig/vcli.git

# Required dependencies
git clone https://github.com/vultisig/verifier.git
git clone https://github.com/vultisig/app-recurring.git
git clone https://github.com/vultisig/go-wrappers.git
```

### Building go-wrappers (DKLS library)

The go-wrappers repo contains the native DKLS cryptographic library required for TSS operations:

```bash
cd ~/dev/vultisig/go-wrappers

# macOS
./build_darwin.sh

# Linux
./build_linux.sh
```

This creates the native library in `includes/darwin/` (macOS) or `includes/linux/` (Linux).

**Note:** The library path must be configured in `local/cluster.yaml` under `library.dyld_path`.

## Vault Requirement

You need a **Fast Vault** (vault with cloud backup) exported from the Vultisig mobile app:

1. Create a vault in the Vultisig mobile app with "Fast Vault" enabled
2. Export the vault backup (Settings -> Export -> Backup file)
3. Transfer the `.vult` file to `local/keyshares/` directory

## Initial Setup (One-Time)

```bash
cd vcli

# 1. Put your vault file in the keyshares directory
cp ~/Downloads/MyVault.vult local/keyshares/

# 2. (Optional) Edit local/cluster.yaml if your repos are in different locations
```

---

## E2E Testing Flow

Follow these steps **in order, every time**. Do not skip steps.

### Step 1: START

Start all services (infrastructure + application services).

```bash
make start
```

**Service Modes:**
```bash
make start              # Default: dev mode (recommended)
make start MODE=local   # All services run locally
make start MODE=dev     # Relay+Vultiserver production, rest local (DEFAULT)
make start MODE=prod    # All services use production endpoints
```

| Mode | Relay | Vultiserver | Verifier | Plugins |
|------|-------|-------------|----------|---------|
| local | localhost | localhost | localhost | localhost |
| dev (default) | api.vultisig.com | api.vultisig.com | localhost | localhost |
| prod | api.vultisig.com | api.vultisig.com | production | production |

**Validation:**
```bash
make status
```

✅ **Expected:** All services show as "running":
- PostgreSQL, Redis, MinIO (infrastructure)
- Verifier API, Verifier Worker (verifier stack)
- DCA Server, DCA Worker, DCA Scheduler (plugin stack)

❌ **If validation fails:** Check logs with `make logs`. Fix the issue and restart with `make stop && make start`.

---

### Step 2: IMPORT

Import your vault into the local environment.

```bash
# If vault is in local/keyshares/ (recommended):
./local/vcli.sh vault import --password "password"

# Or specify a file explicitly:
./local/vcli.sh vault import --file /path/to/vault.vult --password "password"
```

**Validation:**
```bash
./local/vcli.sh vault list
./local/vcli.sh report
```

✅ **Expected:**
- `vault list` shows your imported vault
- `report` displays vault name, public keys (ECDSA/EdDSA), and signers

❌ **If validation fails:** Verify your `.vult` file is in `local/keyshares/` and password is correct. The vault must be a Fast Vault.

**Next step:** Run `vcli vault details` to view your wallet addresses and balances before proceeding.

**Inspect your vault (recommended for first-time users):**

Before creating policies, view your wallet addresses and token balances:

```bash
# Show all addresses and balances
./local/vcli.sh vault details

# Or check a specific chain
./local/vcli.sh vault details --chain ethereum
```

This displays:
- Wallet addresses for each supported chain (Ethereum, Bitcoin, Solana, etc.)
- Native token balances (ETH, BTC, SOL, etc.)
- Common ERC20 token balances (USDT, USDC, DAI, etc.)

Use this information when creating swap policies in Step 4.

---

### Step 3: INSTALL

Install a plugin. This performs a 4-party TSS reshare.

```bash
./local/vcli.sh plugin install vultisig-dca-0000 --password "password"
```

**What happens:** A 4-party reshare occurs between:
- CLI (your local vault share)
- Fast Vault Server (production cloud backup)
- Verifier Worker (local)
- DCA Plugin Worker (local)

**Validation:**
```bash
./local/vcli.sh report
```

✅ **Expected:**
- Report shows plugin installation in database
- Report shows keyshare files stored in MinIO (4 parties)
- Signers list now includes verifier and plugin parties

❌ **If validation fails:** Check that both workers are running (`make status`). Check logs for TSS errors. **Do not attempt to fix manually** - run `make stop && make start` and restart from Step 1.

---

### Policy Testing Loop (Steps 4-6)

Once a plugin is installed, you can create and test multiple policies without restarting. Repeat Steps 4-6 as many times as needed:

```
┌─────────────────────────────────────────────────┐
│  GENERATE → ADD → MONITOR  (repeat for more)   │
└─────────────────────────────────────────────────┘
```

This is the **only** valid shortcut. You may:
- Test different policy configurations (different assets, amounts, frequencies)
- Create multiple policies simultaneously
- Monitor execution across different policies
- Test edge cases and error conditions

**Continue testing more policies** by repeating Steps 4-6. When completely done with all testing, proceed to cleanup (Steps 7-9).

---

### Step 4: GENERATE

Generate a policy configuration file using the `vcli policy generate` command.

```bash
# Generate a swap policy (RECOMMENDED)
./local/vcli.sh policy generate --from eth --to usdc --amount 0.01 --output my-policy.json

# Generate with custom frequency
./local/vcli.sh policy generate --from usdt --to btc --amount 10 --frequency daily --output my-policy.json
```

**Why use `policy generate`:**
- Auto-derives wallet addresses from your imported vault
- Converts amounts to smallest units automatically
- Validates the recipe with the plugin server
- Supports asset shortcuts: `eth`, `btc`, `sol`, `usdc`, `usdt`, `dai`, etc.

**Asset shortcuts:**
```
eth, btc, sol, rune, bnb, avax, matic  - Native tokens
usdc, usdt, dai                        - Stablecoins (Ethereum)
usdc:arbitrum                          - Specify chain explicitly
```

**Frequency options:** `one-time` (default), `minutely`, `hourly`, `daily`, `weekly`, `bi-weekly`, `monthly`

**Example output:**
```
Policy written to my-policy.json

Policy Summary:
  From: 0.01 eth (Ethereum)
        0x2d63088Dacce3a87b0966982D52141AEe53be224 [FastPlugin1]
  To:   usdc (Ethereum)
        0x2d63088Dacce3a87b0966982D52141AEe53be224 [FastPlugin1]
  Amount: 10000000000000000 (smallest unit)
  Frequency: one-time
```

✅ **Expected:** Policy JSON file created with addresses auto-filled.

❌ **If validation fails:** Check that your vault is imported and the asset names are valid.

---

### Step 5: ADD

Add the policy to the installed plugin.

```bash
./local/vcli.sh policy add --plugin vultisig-dca-0000 --policy-file my-policy.json --password "password"
```

**Validation:**
```bash
./local/vcli.sh policy list --plugin vultisig-dca-0000
```

✅ **Expected:** Your policy appears in the list with a policy ID.

❌ **If validation fails:** Check that the plugin is installed (Step 3). Check verifier logs for signing errors.

---

### Step 6: MONITOR

Monitor the policy execution and check its status.

```bash
# Check policy status and next execution time
./local/vcli.sh policy status <policy-id>

# View executed transactions
./local/vcli.sh policy transactions <policy-id>

# View transaction history
./local/vcli.sh policy history <policy-id>

# List all policies for the plugin
./local/vcli.sh policy list --plugin vultisig-dca-0000

# Watch logs in real-time
make logs

# Or watch specific service logs
tail -f /tmp/dca-worker.log
tail -f /tmp/dca-scheduler.log
```

**Next steps:**
- **Test another policy:** Go back to Step 4 (GENERATE) to create a new policy
- **Monitor existing policies:** Use `policy status`, `policy transactions`, or `policy history` commands
- **When done testing:** Proceed to cleanup steps (DELETE → UNINSTALL → STOP)

✅ **Expected:** Policy shows execution history, pending/completed transactions, and next scheduled execution time.

❌ **If validation fails:** Check scheduler and worker logs for errors. Verify the policy frequency and chain configuration.

---

## Cleanup Steps (When Done Testing)

When you're completely finished testing all policies, follow these cleanup steps:

### Step 7: DELETE (Optional)

Delete individual policies if you want to clean up specific ones.

```bash
./local/vcli.sh policy delete <policy-id> --password "password"
```

**Note:** You can skip this step if you want to keep policies for future testing. Policies will be cleaned up when you uninstall the plugin.

**Validation:**
```bash
./local/vcli.sh policy list --plugin vultisig-dca-0000
```

✅ **Expected:** The deleted policy no longer appears in the list.

---

### Step 8: UNINSTALL

Uninstall the plugin. This will remove all policies associated with the plugin.

```bash
./local/vcli.sh plugin uninstall vultisig-dca-0000
```

**Validation:**
```bash
./local/vcli.sh report
```

✅ **Expected:** Plugin installation no longer appears in the report.

❌ **If validation fails:** Check for remaining policies - all policies will be automatically removed when uninstalling.

---

### Step 9: STOP

Stop all services and clean all state (vaults, logs, docker volumes).

```bash
make stop
```

**Validation:**
```bash
make status
```

✅ **Expected:** All services show as stopped or not running.

**Note:** Only run this when you're completely done with testing. If you want to test more policies, stay in the policy testing loop (Steps 4-6).

---

## What NOT To Do

| Bad Practice | Why It Breaks Things |
|--------------|---------------------|
| Restarting mid-test | TSS sessions are stateful; partial state causes reshare failures |
| Moving keyshare files | Keyshares are tied to specific party IDs and sessions |
| Manual DB edits | Breaks consistency between DB records and MinIO keyshares |
| Skipping UNINSTALL | Leaves orphaned keyshares that conflict with future installs |
| Re-using failed state | Corrupted state propagates; always clean start |

**The only recovery from a failed test is:** `make stop` then start fresh from Step 1.

---

## Service Modes

Use `--mode` flag when starting services (or override with `cluster.yaml`):

```bash
make start              # Default: dev mode
make start MODE=local   # All local
make start MODE=dev     # Relay+Vultiserver production, rest local
make start MODE=prod    # All production
```

| Mode | Description | Use Case |
|------|-------------|----------|
| **local** | All services run from source | Testing relay/vultiserver changes |
| **dev** | Relay+Vultiserver use production, rest local | **Recommended for plugin development** |
| **prod** | All services use production endpoints | Integration testing against live |

The mode flag overrides `cluster.yaml` service settings at runtime.

## vcli Commands Reference

```bash
# Vault management (put .vult file in local/keyshares/ first)
./local/vcli.sh vault import --password "password"
./local/vcli.sh vault list

# Plugin management
./local/vcli.sh plugin list
./local/vcli.sh plugin install <plugin-id> --password "password"
./local/vcli.sh plugin uninstall <plugin-id>

# Policy management
./local/vcli.sh policy generate --from <asset> --to <asset> --amount <amount> --output <file.json>
./local/vcli.sh policy add --plugin <plugin-id> --policy-file <config.json> --password "password"
./local/vcli.sh policy list --plugin <plugin-id>
./local/vcli.sh policy status <policy-id>        # Check status and next execution
./local/vcli.sh policy transactions <policy-id>   # View executed transactions
./local/vcli.sh policy history <policy-id>        # View transaction history
./local/vcli.sh policy delete <policy-id> --password "password"  # Cleanup only

# Status and reporting
./local/vcli.sh report
./local/vcli.sh status
```

## Services & Ports

| Service | Port | Description |
|---------|------|-------------|
| PostgreSQL | 5432 | Database |
| Redis | 6379 | Task queue |
| MinIO | 9000 | Keyshare storage |
| MinIO Console | 9090 | MinIO web UI |
| Verifier API | 8080 | Main verifier |
| Verifier Worker | 8089 | Worker metrics |
| DCA Server | 8082 | DCA plugin API |
| DCA Worker | 8183 | DCA worker metrics |

## Queue Isolation (4-Party TSS)

When installing a plugin, a 4-party TSS reshare occurs:
- **CLI** (vcli)
- **Fast Vault Server** (production)
- **Verifier Worker** (listens on `default_queue`)
- **DCA Plugin Worker** (listens on `dca_plugin_queue`)

The workers use separate task queues to prevent task stealing. This is configured in:
- `local/configs/dca-server.env`: `SERVER_TASKQUEUENAME=dca_plugin_queue`
- `local/configs/dca-worker.env`: `TASK_QUEUE_NAME=dca_plugin_queue`

## Make Commands

```bash
make build                  # Build vcli
make start                  # Start services (default: dev mode)
make start MODE=local       # Start all services locally
make start MODE=dev         # Relay+Vultiserver production, rest local
make start MODE=prod        # All production endpoints
make stop                   # Stop all services and clean all state
make status                 # Show service status
make logs                   # Tail all logs
```

## Directory Structure

```
vcli/
├── local/
│   ├── cmd/vcli/             # vcli source code
│   ├── scripts/              # Shell scripts (vcli.sh)
│   ├── keyshares/            # Put your .vult files here
│   ├── policies/             # Policy JSON templates
│   ├── configs/              # Service environment files (*.env)
│   ├── docker-compose.yaml   # Docker infrastructure
│   ├── cluster.yaml          # Cluster configuration
│   └── Dockerfile            # vcli container image
└── Makefile
```

## Troubleshooting

### Library Not Found Error

```
dyld: Library not loaded: libgodkls.dylib
```

**Solution:** Use `./local/vcli.sh` wrapper which sets `DYLD_LIBRARY_PATH` from `cluster.yaml`.

### Port Conflicts

```bash
lsof -i :5432   # Check PostgreSQL
lsof -i :8080   # Check Verifier
make stop       # Force stop everything
```

### TSS Reshare Stuck at 3 Parties

Check queue isolation - both workers must use separate queues:
```bash
docker exec vultisig-redis redis-cli -a vultisig KEYS "asynq:*queue*"
```

### View Logs

```bash
tail -f /tmp/verifier.log      # Verifier server
tail -f /tmp/worker.log        # Verifier worker
tail -f /tmp/dca.log           # DCA plugin server
tail -f /tmp/dca-worker.log    # DCA plugin worker
```
