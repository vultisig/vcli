# Vultisig Cluster

Local development environment for testing Vultisig plugins with Docker-based infrastructure.

## Prerequisites

- **Go 1.23+** - https://go.dev/dl/
- **Docker** - https://docs.docker.com/get-docker/
- **Docker Compose** - Usually included with Docker Desktop

## Dependencies

Clone all required repositories into the same parent directory:

```bash
mkdir -p ~/dev/vultisig && cd ~/dev/vultisig

# This repo
git clone https://github.com/vultisig/vultisig-cluster.git

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

## Quick Start

```bash
cd vultisig-cluster

# 1. Configure paths to your local repos
cp local/cluster.yaml.example local/cluster.yaml
# Edit cluster.yaml with your repo paths

# 2. Configure vault credentials
cp local/vault.env.example local/vault.env
# Edit vault.env with your vault file path and password

# 3. Start all services
make local-start

# 4. Import your vault
./local/vcli.sh vault import -f /path/to/vault.vult -p "password" --force

# 5. Install a plugin (4-party TSS reshare)
./local/vcli.sh plugin install vultisig-dca-0000 -p "password"

# 6. Add a policy
./local/vcli.sh policy add --plugin vultisig-dca-0000 -c local/configs/policies/test-one-time-policy.json --password "password"

# 7. Check status
./local/vcli.sh report

# 8. Stop all services
make local-stop
```

## Vault Requirement

You need a **Fast Vault** (vault with cloud backup) exported from the Vultisig mobile app:

1. Create a vault in the Vultisig mobile app with "Fast Vault" enabled
2. Export the vault backup (Settings -> Export -> Backup file)
3. Transfer the `.vult` file to your development machine
4. Configure the path in `local/vault.env`

## Service Modes

In `local/cluster.yaml`, each service can be `local` or `production`:

```yaml
services:
  relay: production      # Use api.vultisig.com/router
  vultiserver: production  # Use api.vultisig.com
  verifier: local        # Run from source
  dca_server: local      # Run from source
```

This lets you test local changes against production relay/vultiserver, or run everything locally.

## vcli Commands

The `vcli` CLI manages vaults, plugins, and policies:

```bash
# Vault management
./local/vcli.sh vault import -f /path/to/vault.vult -p "password" --force
./local/vcli.sh vault list

# Plugin management
./local/vcli.sh plugin list
./local/vcli.sh plugin install <plugin-id> -p "password"
./local/vcli.sh plugin uninstall <plugin-id>

# Policy management
./local/vcli.sh policy add --plugin <plugin-id> -c <config.json> --password "password"
./local/vcli.sh policy list -p <plugin-id>
./local/vcli.sh policy delete <policy-id> --password "password"
./local/vcli.sh policy status <policy-id>

# Status and reporting
./local/vcli.sh report
./local/vcli.sh status
```

See [local/docs/VCLI.md](local/docs/VCLI.md) for detailed usage.

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
make local-build    # Build vcli
make local-start    # Build and start all services
make local-stop     # Stop all services
make local-status   # Show service status
make local-logs     # Tail all logs
make local-clean    # Remove binaries and configs
```

## Directory Structure

```
vultisig-cluster/
├── local/
│   ├── cmd/vcli/             # vcli source code
│   ├── scripts/              # Shell scripts (vcli.sh, tests)
│   ├── docs/                 # Documentation (VCLI.md)
│   ├── configs/
│   │   ├── policies/         # Policy JSON templates
│   │   ├── *.env             # Service environment files
│   │   └── docker-compose.yaml
│   ├── cluster.yaml.example  # Config template
│   ├── vault.env.example     # Vault config template
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
make local-stop # Force stop everything
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
