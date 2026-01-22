# vcli - Local Development Environment

Vultisig CLI for local development and testing of Vultisig plugins.

---

## Why Local Development?

The Vultisig stack is tightly coupled:

- **vcli** depends on verifier (TSS protocols)
- **verifier** depends on recipes (chain abstraction)
- **app-recurring** depends on recipes + verifier (policy execution)
- All depend on **go-wrappers** (cryptographic primitives)

Changes in one repo often require changes in others. Docker images create version drift - the local vcli binary may be incompatible with pre-built Docker images due to protocol or signature changes.

Local development lets you:
- Edit any repo and test immediately
- Debug with IDE breakpoints
- Use native architecture (no emulation issues on ARM Macs)

---

## Prerequisites

- Go 1.24+
- Docker (for postgres, redis, minio only)
- Git

---

## Setup (One-Time)

Clone all repos as siblings:

```bash
mkdir vultisig && cd vultisig
git clone https://github.com/vultisig/vcli.git
git clone https://github.com/vultisig/verifier.git
git clone https://github.com/vultisig/app-recurring.git
git clone https://github.com/vultisig/recipes.git
git clone https://github.com/vultisig/go-wrappers.git
```

Directory structure:
```
vultisig/
├── vcli/           # This tool
├── verifier/       # Policy verification + TSS
├── app-recurring/  # DCA plugin
├── recipes/        # Chain abstraction layer
└── go-wrappers/    # Rust crypto (auto-downloaded, but useful to have)
```

---

## Quick Start

```bash
cd vcli

# Build vcli (REQUIRED - do this first!)
cd local && go build -o vcli ./cmd/vcli && cd ..

# Start all services
make start    # Starts postgres/redis/minio in Docker, services run natively
```

> **⚠️ IMPORTANT:** You MUST build the vcli binary before using any `./local/vcli.sh` commands. If you see "No such file or directory" errors, run the build command above.

---

## How It Works

`make start`:
1. Starts infrastructure in Docker (postgres, redis, minio)
2. Runs verifier (API + worker) natively with `go run`
3. Runs app-recurring (server + worker + scheduler) natively with `go run`

Logs: `tail -f local/logs/*.log`

---

## Developing

Edit code in any sibling repo, then:
```bash
make stop && make start
```

Or restart individual services manually.

---

## Vault Requirement

You need a **Fast Vault** (vault with cloud backup) exported from the Vultisig mobile app:

1. Create a vault in the Vultisig mobile app with "Fast Vault" enabled
2. Export the vault backup (Settings -> Export -> Backup file)
3. Transfer the `.vult` file to `local/keyshares/` directory

## Initial Setup (One-Time)

```bash
# Put your vault file in the keyshares directory
cp ~/Downloads/MyVault.vult local/keyshares/
```

---

## IMPORTANT: Read Before Starting

**You MUST follow the E2E testing flow exactly as documented below.** The flow is:

```
START → IMPORT → INSTALL → [ GENERATE → ADD → MONITOR ] ─┐
                                                         │
                                  (repeat as needed) ←───┘
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

## E2E Testing Flow

Follow these steps **in order, every time**. Do not skip steps.

### Step 1: START

Start all services (infrastructure + application services).

```bash
make start
```

This starts:
- Infrastructure in Docker: PostgreSQL, Redis, MinIO
- Services natively: Verifier API/worker, DCA plugin server/worker/scheduler

**Validation:**
```bash
make status
```

✅ **Expected:** Infrastructure containers show as "running" (postgres, redis, minio)

❌ **If validation fails:** Check logs with `tail -f local/logs/*.log`. Fix the issue and restart with `make stop && make start`.

---

### Step 2: IMPORT

Import your vault into the local environment.

```bash
# Recommended: auto-find vault in local/keyshares/
./local/vcli.sh vault import --password "password"

# Or specify a file with ABSOLUTE path:
./local/vcli.sh vault import --file /full/path/to/vault.vult --password "password"
```

**Note:** When using `--file`, you must provide an absolute path (starting with `/`). Relative paths will not work because the vcli.sh wrapper changes directories internally.

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
- Verifier Worker (running locally)
- DCA Plugin Worker (running locally)

**Validation:**
```bash
./local/vcli.sh report
```

✅ **Expected:**
- Report shows plugin installation in database
- Report shows keyshare files stored in MinIO (4 parties)
- Signers list now includes verifier and plugin parties

❌ **If validation fails:** Check logs with `tail -f local/logs/*.log`. **Do not attempt to fix manually** - run `make stop && make start` and restart from Step 1.

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
# Generate a swap policy (use ABSOLUTE path for --output)
./local/vcli.sh policy generate --from eth --to usdc --amount 0.01 --output /full/path/to/vcli/local/policies/my-policy.json

# Generate with custom frequency
./local/vcli.sh policy generate --from usdt --to btc --amount 10 --frequency daily --output /full/path/to/vcli/local/policies/my-policy.json
```

> **⚠️ IMPORTANT:** The `--output` flag requires an **absolute path** (starting with `/`). Relative paths will fail because `vcli.sh` changes directories internally. Use `$(pwd)/local/policies/my-policy.json` or the full path.

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
./local/vcli.sh policy add --plugin vultisig-dca-0000 --policy-file local/policies/my-policy.json --password "password"
```

**Validation:**
```bash
./local/vcli.sh policy list --plugin vultisig-dca-0000
```

✅ **Expected:** Your policy appears in the list with a policy ID.

❌ **If validation fails:** Check that the plugin is installed (Step 3). Check logs with `tail -f local/logs/verifier.log`.

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
tail -f local/logs/dca-worker.log
tail -f local/logs/dca-scheduler.log
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

# Policy management (use absolute paths for file arguments)
./local/vcli.sh policy generate --from <asset> --to <asset> --amount <amount> --output $(pwd)/local/policies/<file.json>
./local/vcli.sh policy add --plugin <plugin-id> --policy-file $(pwd)/local/policies/<config.json> --password "password"
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

| Service | Port | Notes |
|---------|------|-------|
| PostgreSQL | 5432 | Docker container |
| Redis | 6379 | Docker container |
| MinIO | 9000 | Docker container |
| MinIO Console | 9090 | Docker container |
| Verifier API | 8080 | Native (go run) |
| Verifier Worker | - | Native (go run) |
| DCA Server | 8082 | Native (go run) |
| DCA Worker | - | Native (go run) |
| DCA Scheduler | - | Native (go run) |
| DCA TX Indexer | - | Native (go run) |

## Queue Isolation (4-Party TSS)

When installing a plugin, a 4-party TSS reshare occurs:
- **CLI** (vcli)
- **Fast Vault Server** (production)
- **Verifier Worker** (running locally)
- **DCA Plugin Worker** (running locally)

The workers use separate task queues to prevent task stealing.

## Make Commands

```bash
make start     # Start infra in Docker + services natively
make stop      # Stop all services and clean state
make status    # Show infrastructure container status
make logs      # Show how to view service logs
make help      # Show all available commands
```

## Directory Structure

```
vcli/
├── local/
│   ├── cmd/vcli/                  # vcli source code
│   ├── scripts/
│   │   ├── vcli.sh                # vcli wrapper script
│   │   └── run-services.sh        # Runs services natively
│   ├── keyshares/                 # Put your .vult files here
│   ├── policies/                  # Policy JSON templates
│   ├── configs/                   # Service environment files (*.env)
│   ├── logs/                      # Service logs
│   ├── docker-compose.yaml        # Infrastructure only (postgres, redis, minio)
│   └── cluster.yaml               # Cluster configuration
├── Makefile                       # Main entry point: make start/stop/status
└── README.md
```

**Sibling repos (required):**
```
vultisig/
├── vcli/
├── verifier/
├── app-recurring/
├── recipes/
└── go-wrappers/
```

## Troubleshooting

### Service Won't Start

```bash
# Check service logs
tail -f local/logs/verifier.log
tail -f local/logs/worker.log
tail -f local/logs/dca-server.log

# Check infrastructure containers
docker compose -f local/docker-compose.yaml ps
```

### Port Conflicts

```bash
lsof -i :5432   # Check PostgreSQL
lsof -i :8080   # Check Verifier
make stop       # Force stop everything
```

**On shared machines:** If another user has processes running on required ports, you'll need to coordinate with them or use sudo to kill the processes:
```bash
# Check what's using a port
lsof -i :8080

# If owned by another user, coordinate with them or:
sudo pkill -9 -f "go-build.*/verifier"
```

### TSS Reshare Stuck at 3 Parties

Check queue isolation - both workers must use separate queues:
```bash
docker exec vultisig-redis redis-cli -a vultisig KEYS "asynq:*queue*"
```

### View Logs

```bash
# View specific service logs
tail -f local/logs/verifier.log      # Verifier server
tail -f local/logs/worker.log        # Verifier worker
tail -f local/logs/dca-server.log    # DCA plugin server
tail -f local/logs/dca-worker.log    # DCA plugin worker
tail -f local/logs/dca-scheduler.log # DCA scheduler

# View all logs
tail -f local/logs/*.log
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

---

## Kubernetes Deployment

For production Kubernetes deployment on Hetzner Cloud, see **[infrastructure/DEPLOYMENT.md](infrastructure/DEPLOYMENT.md)**.

The K8s deployment guide covers:
- Terraform-based infrastructure provisioning
- K3s cluster setup
- Service deployment with kustomize overlays
- E2E testing in Kubernetes
- Server type and region selection (AMD64 required for GHCR images)
- Troubleshooting guide with common errors and fixes
