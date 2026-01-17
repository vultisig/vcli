# vcli

Vultisig CLI - Local development environment for testing Vultisig plugins.

---

## Quick Start (For Developers)

```bash
# Clone repos as siblings
git clone https://github.com/vultisig/vcli.git
git clone https://github.com/vultisig/verifier.git
git clone https://github.com/vultisig/app-recurring.git

# Start everything
cd vcli
make start
```

That's it! Infrastructure runs in Docker, services run natively with `go run`.

---

## Build Modes

vcli supports three build modes for different use cases:

| Mode | Command | What It Does | Best For |
|------|---------|--------------|----------|
| **local** (default) | `make start` | Infra in Docker, services run natively with `go run` | Active development |
| **volume** | `make start build=volume` | Full stack in Docker with volume mounts | Testing Docker config with hot-reload |
| **image** | `make start build=image` | Full stack in Docker with GHCR images | CI/CD, quick testing without repos |

### build=local (Default) — Active Development

**Best for:** Day-to-day development when you're making changes to verifier or app-recurring.

```bash
make start              # or explicitly: make start build=local
```

**What happens:**
1. Starts postgres, redis, minio in Docker
2. Runs verifier and app-recurring services natively with `go run`
3. Edit code → restart to apply (or use your own hot-reload setup)

**Requirements:**
```
your-folder/
  ├── vcli/           # this repo
  ├── verifier/       # git clone https://github.com/vultisig/verifier.git
  └── app-recurring/  # git clone https://github.com/vultisig/app-recurring.git
```

**Logs:** `tail -f local/logs/*.log`

### build=volume — Docker with Hot-Reload

**Best for:** Testing Docker configuration while still iterating on code.

```bash
make start build=volume
```

**What happens:**
1. Builds Docker images from local source
2. Volume-mounts source directories into containers
3. Uses `air` for hot-reload (~2-3s rebuild on file change)

**Requirements:** Same sibling repos as `build=local`

**Logs:** `docker logs -f vultisig-verifier`

### build=image — Pre-built GHCR Images

**Best for:** Quick testing, CI/CD, or when you don't need to modify verifier/app-recurring.

```bash
make start build=image
```

**What happens:**
1. Pulls pre-built images from `ghcr.io/vultisig/...`
2. No local repos required
3. No ability to modify code (use for testing only)

**Requirements:** Just Docker. No sibling repos needed.

**Logs:** `docker logs -f vultisig-verifier`

---

## Stopping Services

```bash
make stop    # Stops everything (Docker containers + native processes) and cleans state
```

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

- **Docker** - https://docs.docker.com/get-docker/
- **Docker Compose** - Usually included with Docker Desktop

That's it! No local repo clones, no library builds, no Go installation required.

## Vault Requirement

You need a **Fast Vault** (vault with cloud backup) exported from the Vultisig mobile app:

1. Create a vault in the Vultisig mobile app with "Fast Vault" enabled
2. Export the vault backup (Settings -> Export -> Backup file)
3. Transfer the `.vult` file to `local/keyshares/` directory

## Initial Setup (One-Time)

```bash
# Clone this repo
git clone https://github.com/vultisig/vcli.git
cd vcli

# Put your vault file in the keyshares directory
cp ~/Downloads/MyVault.vult local/keyshares/
```

---

## E2E Testing Flow

Follow these steps **in order, every time**. Do not skip steps.

### Step 1: START

Start all services (infrastructure + application services).

```bash
make start
```

This starts Docker containers for:
- Infrastructure: PostgreSQL, Redis, MinIO
- Verifier: API server and worker
- DCA Plugin: Server, worker, scheduler, tx-indexer

**Validation:**
```bash
make status
```

✅ **Expected:** All containers show as "running":
- vultisig-postgres, vultisig-redis, vultisig-minio
- vultisig-verifier, vultisig-worker
- vultisig-dca, vultisig-dca-worker, vultisig-dca-scheduler

❌ **If validation fails:** Check logs with `docker logs <container-name>`. Fix the issue and restart with `make stop && make start`.

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
- Verifier Worker (Docker container)
- DCA Plugin Worker (Docker container)

**Validation:**
```bash
./local/vcli.sh report
```

✅ **Expected:**
- Report shows plugin installation in database
- Report shows keyshare files stored in MinIO (4 parties)
- Signers list now includes verifier and plugin parties

❌ **If validation fails:** Check that containers are running (`make status`). Check logs with `docker logs vultisig-worker`. **Do not attempt to fix manually** - run `make stop && make start` and restart from Step 1.

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

❌ **If validation fails:** Check that the plugin is installed (Step 3). Check verifier logs with `docker logs vultisig-verifier`.

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
docker logs -f vultisig-dca-worker
docker logs -f vultisig-dca-scheduler
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

## vcli Commands Reference

```bash
# Vault management (put .vult file in local/keyshares/ first)
./local/vcli.sh vault import --password "password"
./local/vcli.sh vault list
./local/vcli.sh vault details

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

| Service | Port | Container Name |
|---------|------|----------------|
| PostgreSQL | 5432 | vultisig-postgres |
| Redis | 6379 | vultisig-redis |
| MinIO | 9000 | vultisig-minio |
| MinIO Console | 9090 | vultisig-minio |
| Verifier API | 8080 | vultisig-verifier |
| Verifier Worker | - | vultisig-worker |
| DCA Server | 8082 | vultisig-dca |
| DCA Worker | - | vultisig-dca-worker |
| DCA Scheduler | - | vultisig-dca-scheduler |
| DCA TX Indexer | - | vultisig-dca-tx-indexer |

## Queue Isolation (4-Party TSS)

When installing a plugin, a 4-party TSS reshare occurs:
- **CLI** (vcli)
- **Fast Vault Server** (production)
- **Verifier Worker** (listens on `default_queue`)
- **DCA Plugin Worker** (listens on `dca_plugin_queue`)

The workers use separate task queues to prevent task stealing.

## Make Commands

```bash
# Build modes
make start                  # Default: build=local (infra Docker, services native)
make start build=local      # Infra in Docker, services run natively with go run
make start build=volume     # Full stack in Docker with volume mounts + hot-reload
make start build=image      # Full stack in Docker with GHCR images

# Other commands
make stop                   # Stop all services and clean state
make status                 # Show container status
make help                   # Show all available commands
```

## Directory Structure

```
vcli/
├── local/
│   ├── cmd/vcli/                  # vcli source code
│   ├── scripts/
│   │   ├── vcli.sh                # vcli wrapper script
│   │   └── run-services.sh        # Runs services natively (build=local)
│   ├── keyshares/                 # Put your .vult files here
│   ├── policies/                  # Policy JSON templates
│   ├── configs/                   # Service environment files (*.env)
│   ├── logs/                      # Native service logs (build=local)
│   ├── docker-compose.yaml        # Infrastructure only (postgres, redis, minio)
│   ├── docker-compose.local.yaml  # Full stack with volume mounts (build=volume)
│   ├── docker-compose.full.yaml   # Full stack with GHCR images (build=image)
│   └── cluster.yaml               # Cluster configuration
├── Makefile                       # Main entry point: make start/stop/status
└── README.md
```

**Sibling repos (required for build=local and build=volume):**
```
your-folder/
├── vcli/
├── verifier/
└── app-recurring/
```

## Troubleshooting

### Container Won't Start

```bash
# Check container logs
docker logs vultisig-verifier
docker logs vultisig-worker
docker logs vultisig-dca

# Check all container status
docker compose -f local/docker-compose.full.yaml ps
```

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
# View specific container logs
docker logs -f vultisig-verifier      # Verifier server
docker logs -f vultisig-worker        # Verifier worker
docker logs -f vultisig-dca           # DCA plugin server
docker logs -f vultisig-dca-worker    # DCA plugin worker
docker logs -f vultisig-dca-scheduler # DCA scheduler

# View all logs
docker compose -f local/docker-compose.full.yaml logs -f
```

### go-wrappers Library

The go-wrappers CGO library is automatically downloaded on first run. If you encounter library loading issues:

```bash
# Check if libraries are downloaded
ls -la ~/.vultisig/lib/

# Force re-download by removing the marker file
rm ~/.vultisig/lib/darwin/.downloaded-master  # macOS
rm ~/.vultisig/lib/linux/.downloaded-master   # Linux

# Then run any vcli command to trigger download
./local/vcli.sh --help
```
