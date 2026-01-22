# Vultisig Kubernetes Deployment Guide

Production-ready Kubernetes deployment for Vultisig services on Hetzner Cloud.

---

## Prerequisites

- `hcloud` CLI installed and configured
- SSH key registered in Hetzner Cloud
- `.env.k8s` file with secrets (see [Secrets Configuration](#secrets-configuration))
- `kubectl` installed locally

---

## Quick Start (Full E2E Deployment)

### One-Shot Deploy + Test

```bash
cd /Users/dev/dev/vultisig/vcli
source .env.k8s

# 1. Create infrastructure (if not exists)
# See "Phase 1: Infrastructure Setup" for manual hcloud commands
# Or use Terraform: cd infrastructure/terraform && terraform apply

# 2. Setup K3s on nodes
./infrastructure/scripts/setup-cluster.sh

# 3. Deploy and start services (single command)
export KUBECONFIG=$(pwd)/.kube/config
make deploy-secrets && ./infrastructure/scripts/k8s-start.sh

# 4. Run E2E test
./infrastructure/scripts/e2e-test.sh
```

### Manual E2E Test Commands

```bash
kubectl exec -n verifier vcli -- vcli vault import --file /vault/vault.vult --password "Password123"
kubectl exec -n verifier vcli -- vcli plugin install dca --password "Password123"
kubectl exec -n verifier vcli -- vcli policy generate --from usdc --to btc --amount 10 --output /tmp/policy.json
kubectl exec -n verifier vcli -- vcli policy add --plugin dca --policy-file /tmp/policy.json --password "Password123"
kubectl exec -n verifier vcli -- vcli policy list --plugin dca
```

---

## Hetzner Cloud Reference

### Locations

| Code | Name | Notes |
|------|------|-------|
| `sin` | Singapore | Asia-Pacific |
| `fsn1` | Falkenstein | Germany (EU) |
| `nbg1` | Nuremberg | Germany (EU) |
| `hel1` | Helsinki | Finland (EU) |
| `ash` | Ashburn | US East |
| `hil` | Hillsboro | US West |

**Common mistake:** The location code is `sin` not `sin1`. Some locations have datacenter suffixes in the output (e.g., `sin-dc1`) but the location code is just `sin`.

### Server Types by Location

Not all server types are available in all locations.

| Location | Recommended Type | Notes |
|----------|------------------|-------|
| Singapore (`sin`) | `cpx32` | `cpx31` NOT available |
| EU locations | `cpx31` | Full availability |
| US locations | `cpx31` or `cpx32` | Check availability |

**Check availability:**
```bash
hcloud server-type list --output columns=name,description,cores,memory
```

### SSH Key Requirement

Servers MUST be created with an SSH key for the setup scripts to work.

```bash
# List available SSH keys
hcloud ssh-key list

# Create servers with SSH key (REQUIRED)
hcloud server create \
  --type cpx32 \
  --image ubuntu-22.04 \
  --name vultisig-master-1 \
  --location sin \
  --ssh-key dev-key  # <-- REQUIRED
```

**If you forget the SSH key:** Delete and recreate the server. There's no way to add SSH keys after creation.

---

## Phase 1: Infrastructure Setup

### Create Servers

```bash
source .env.k8s
export HCLOUD_TOKEN

# Check existing servers
hcloud server list

# Delete old servers if needed
hcloud server delete vultisig-master-1 --poll-interval 5s || true
hcloud server delete vultisig-worker-1 --poll-interval 5s || true
hcloud server delete vultisig-worker-2 --poll-interval 5s || true
hcloud server delete vultisig-worker-3 --poll-interval 5s || true

# Create 4 servers in Singapore (use cpx32, cpx31 not available)
hcloud server create --type cpx32 --image ubuntu-22.04 --name vultisig-master-1 --location sin --ssh-key dev-key
hcloud server create --type cpx32 --image ubuntu-22.04 --name vultisig-worker-1 --location sin --ssh-key dev-key
hcloud server create --type cpx32 --image ubuntu-22.04 --name vultisig-worker-2 --location sin --ssh-key dev-key
hcloud server create --type cpx32 --image ubuntu-22.04 --name vultisig-worker-3 --location sin --ssh-key dev-key

# Wait for servers to be ready
sleep 30
hcloud server list
```

### Create Setup Environment

```bash
MASTER_IP=$(hcloud server ip vultisig-master-1)
WORKER1_IP=$(hcloud server ip vultisig-worker-1)
WORKER2_IP=$(hcloud server ip vultisig-worker-2)
WORKER3_IP=$(hcloud server ip vultisig-worker-3)

cat > setup-env.sh << EOF
export MASTER_IP="$MASTER_IP"
export MASTER_PRIVATE_IP="$MASTER_IP"
export WORKER_FSN1_IP="$WORKER1_IP"
export WORKER_NBG1_IP="$WORKER2_IP"
export WORKER_HEL1_IP="$WORKER3_IP"
export K3S_TOKEN="vultisig-k3s-token-$(date +%s)"
EOF

source setup-env.sh
```

---

## Phase 2: K3s Cluster Setup

```bash
# Run the cluster setup script
./infrastructure/scripts/setup-cluster.sh

# Set kubeconfig
export KUBECONFIG=$(pwd)/.kube/config

# Verify cluster
kubectl get nodes -o wide
```

**Expected output:**
```
NAME                STATUS   ROLES                  AGE   VERSION
vultisig-master-1   Ready    control-plane,master   2m    v1.28.x
vultisig-worker-1   Ready    <none>                 1m    v1.28.x
vultisig-worker-2   Ready    <none>                 1m    v1.28.x
vultisig-worker-3   Ready    <none>                 1m    v1.28.x
```

---

## Phase 3: Deploy Services

### Deploy Secrets

```bash
make deploy-secrets
```

This creates secrets for:
- PostgreSQL credentials
- Redis credentials
- MinIO credentials
- Encryption keys
- RPC endpoints
- Test vault file

### Deploy Services (GHCR Images)

```bash
# Deploy using production overlay (pulls from GHCR)
make deploy-k8s-prod

# Watch pods start
kubectl get pods -A -w
```

---

## GHCR Images

Pre-published images on GitHub Container Registry:

| Service | Image | Version |
|---------|-------|---------|
| Verifier | `ghcr.io/vultisig/verifier/verifier` | v0.1.16 |
| Verifier Worker | `ghcr.io/vultisig/verifier/worker` | v0.1.16 |
| Verifier TX Indexer | `ghcr.io/vultisig/verifier/tx_indexer` | v0.1.16 |
| DCA Server | `ghcr.io/vultisig/app-recurring/server` | v1.0.84 |
| DCA Scheduler | `ghcr.io/vultisig/app-recurring/scheduler` | v1.0.82 |
| DCA Worker | `ghcr.io/vultisig/app-recurring/worker` | v1.0.82 |
| DCA TX Indexer | `ghcr.io/vultisig/app-recurring/tx_indexer` | v1.0.82 |
| VCLI | `ghcr.io/vultisig/vcli` | v1.0.3 |

**Important version notes:**
- DCA Server v1.0.84 includes TaskQueueName fix (routes tasks to `dca_plugin_queue`)
- VCLI v1.0.3 includes billing fetch fix for policy generation

Images are configured in `k8s/overlays/production/kustomization.yaml`.

---

## Phase 4: E2E Testing

### Import Vault

```bash
kubectl exec -n verifier vcli -- vcli vault import \
  --file /vault/vault.vult \
  --password "Password123"

# Verify import
kubectl exec -n verifier vcli -- vcli vault details
```

### Install DCA Plugin

```bash
kubectl exec -n verifier vcli -- vcli plugin install dca \
  --password "Password123"
```

This performs a 4-party TSS reshare:
1. CLI (vcli in cluster)
2. Fast Vault Server (production)
3. Verifier Worker (local cluster)
4. DCA Plugin Worker (local cluster)

### Create Policy (10 USDC → BTC)

```bash
# Generate policy (fetches pricing from verifier automatically)
kubectl exec -n verifier vcli -- vcli policy generate \
  --from usdc \
  --to btc \
  --amount 10 \
  --output /tmp/policy.json

# Add policy (signs with TSS keysign)
kubectl exec -n verifier vcli -- vcli policy add \
  --plugin dca \
  --policy-file /tmp/policy.json \
  --password "Password123"

# List policies (should show active policy)
kubectl exec -n verifier vcli -- vcli policy list --plugin dca
```

### E2E Validation Checklist

#### Plugin Install
- [ ] Output shows "4 parties joined"
- [ ] Party names are distinct (e.g., `vcli-xxx`, `Server-xxx`, `verifier-xxx`, `dca-worker-xxx`)
- [ ] `Verifier (MinIO): ✓ 458.0KB`
- [ ] `DCA Plugin (MinIO): ✓ 458.0KB`

#### Policy Add
- [ ] Output shows "POLICY ADDED SUCCESSFULLY"
- [ ] Policy ID returned
- [ ] No billing count mismatch errors

#### Swap Execution (Optional)
- [ ] `vcli policy status <id>` shows "Active: true"
- [ ] Worker logs show "swap route found"
- [ ] Worker logs show "tx signed & broadcasted" with txHash

---

## Success Criteria

- [ ] All 4 nodes in Ready state
- [ ] All pods in Running state
- [ ] Vault imported successfully
- [ ] DCA plugin installed (4-party TSS reshare completed)
- [ ] Policy created with `tx_hash` (0x...)

---

## Secrets Configuration

Create `.env.k8s` with:

```bash
# Hetzner Cloud
export HCLOUD_TOKEN="your-hcloud-api-token"

# PostgreSQL
export POSTGRES_DSN="postgres://user:pass@host:5432/db"

# Redis
export REDIS_URI="redis://:password@host:6379"

# MinIO
export MINIO_HOST="http://host:9000"
export MINIO_ACCESS_KEY="access-key"
export MINIO_SECRET_KEY="secret-key"

# Encryption
export ENCRYPTION_SECRET="32-byte-hex-secret"

# Relay
export RELAY_URL="https://relay.vultisig.com"

# RPC Endpoints
export RPC_ETHEREUM_URL="https://eth-mainnet.g.alchemy.com/v2/key"
export RPC_ARBITRUM_URL="https://arb-mainnet.g.alchemy.com/v2/key"
# ... other chains

# Vault (base64 encoded)
export TEST_VAULT_BASE64="base64-encoded-vault-file"
```

---

## Troubleshooting

### SSH Connection Failed

**Symptom:** `setup-cluster.sh` fails with "Permission denied" or "Connection refused"

**Cause:** Servers created without SSH key

**Fix:**
```bash
# Check if servers have SSH key
hcloud server describe vultisig-master-1 | grep ssh_key

# If empty, delete and recreate with --ssh-key flag
hcloud server delete vultisig-master-1
hcloud server create --type cpx32 --image ubuntu-22.04 --name vultisig-master-1 --location sin --ssh-key dev-key
```

### Server Type Not Available

**Symptom:** `resource "cpx31" is not available in location "sin"`

**Fix:** Use `cpx32` for Singapore:
```bash
hcloud server create --type cpx32 --image ubuntu-22.04 --name vultisig-master-1 --location sin --ssh-key dev-key
```

### Pods Stuck in ImagePullBackOff

**Symptom:** Pods can't pull images from GHCR

**Fix:** GHCR images are public, but verify the image path:
```bash
kubectl describe pod <pod-name> -n <namespace>
# Check the image URL in the error message
```

### TSS Timeout

**Symptom:** Plugin install or policy add times out

**Cause:** TSS operations should complete within 30 seconds

**Fix:**
1. Check all pods are running: `kubectl get pods -A`
2. Check worker logs: `kubectl logs -n verifier deploy/worker`
3. Retry the operation (do NOT extend timeout)

### Nodes NotReady

**Symptom:** `kubectl get nodes` shows NotReady

**Fix:**
```bash
# Check node status
kubectl describe node <node-name>

# Check k3s on the node
ssh root@<node-ip> "systemctl status k3s"
ssh root@<node-ip> "journalctl -u k3s -f"
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Hetzner Cloud (Singapore)                 │
├─────────────────────────────────────────────────────────────┤
│  vultisig-master-1 (cpx32)                                  │
│    └── K3s control plane                                    │
│                                                             │
│  vultisig-worker-1 (cpx32)                                  │
│    ├── verifier (API + worker + tx-indexer)                 │
│    └── infra (postgres, redis, minio)                       │
│                                                             │
│  vultisig-worker-2 (cpx32)                                  │
│    └── plugin-dca (server + scheduler + worker + tx-indexer)│
│                                                             │
│  vultisig-worker-3 (cpx32)                                  │
│    └── relay                                                │
└─────────────────────────────────────────────────────────────┘
```

---

## Namespace Layout

| Namespace | Services |
|-----------|----------|
| `infra` | PostgreSQL, Redis, MinIO |
| `verifier` | Verifier API, Worker, TX Indexer, VCLI |
| `plugin-dca` | DCA Server, Scheduler, Worker, TX Indexer |
| `relay` | Relay Server |

---

## K8s vs Local Development Differences

| Component | Local (Docker) | K8s |
|-----------|----------------|-----|
| Services | `go run` processes via `run-services.sh` | GHCR container images |
| Config | Environment variables in `run-services.sh` | ConfigMaps + Secrets |
| Queue Name | `TASK_QUEUE_NAME` in env | Same, set in deployment manifests |
| Images | N/A (native Go binary) | `ghcr.io/vultisig/*` |
| Relay | `api.vultisig.com` | Same (patched in production overlay) |
| MinIO | Local Docker container | K8s StatefulSet in `infra` namespace |
| vcli | Native binary | `ghcr.io/vultisig/vcli:v1.0.3` |

---

## Startup Scripts

### k8s-start.sh (Recommended)

The `k8s-start.sh` script handles full deployment with verification:

```bash
./infrastructure/scripts/k8s-start.sh              # Production overlay
./infrastructure/scripts/k8s-start.sh --local      # Local overlay (internal relay)
./infrastructure/scripts/k8s-start.sh --skip-seed  # Skip database seeding
```

**What it does:**
1. Applies kustomize overlay (creates namespaces, deploys pods)
2. Applies secrets
3. Recreates jobs (minio-init, seed-plugins)
4. Waits for infrastructure (PostgreSQL, Redis, MinIO)
5. Waits for application services (Verifier, DCA)
6. Flushes Redis for clean state
7. Runs comprehensive verification:
   - MinIO buckets exist
   - Database seeded with DCA plugin
   - Redis responding
   - All pods healthy (Running, 0 restarts)
   - Service HTTP endpoints responding
   - Worker queue configuration correct

### k8s-stop.sh

Graceful shutdown with cleanup:

```bash
./infrastructure/scripts/k8s-stop.sh
```

---

## Makefile Commands

```bash
make deploy-secrets     # Deploy secrets to cluster
make deploy-k8s-prod    # Deploy all services using GHCR images
make deploy-k8s-local   # Deploy using local images (for development)
make delete-k8s         # Delete all Kubernetes resources
make k8s-start          # Run k8s-start.sh
make k8s-stop           # Run k8s-stop.sh
```

---

## Version History

| Date | Version | Changes |
|------|---------|---------|
| 2026-01-21 | 1.0 | Initial deployment to Singapore |

---

## Lessons Learned

### Infrastructure & Hetzner

1. **Always use `--ssh-key`** when creating Hetzner servers
2. **Check server type availability** per location (`cpx32` for Singapore)
3. **Location codes don't have suffixes** (`sin` not `sin1`)

### Kubernetes & K3s

4. **K8s 1.34+ rejects `node-role.kubernetes.io/*` labels** - use `node.kubernetes.io/role=*` instead
5. **Use kustomize overlays** for environment-specific image tags
6. **GHCR images are public** - no authentication needed

### Image Configuration

7. **GHCR images use `/app/main` binary path** - local images use component-specific paths (`/app/scheduler`, `/usr/local/bin/verifier`). Production kustomization patches commands.

### Service Configuration

8. **RPC ConfigMaps need all chains** - DCA worker requires: zksync, cronos, cosmos, tron, dash, zcash (see `k8s/base/dca/configmaps.yaml`)
9. **VCLI DCA plugin URL** must point to `server-swap.plugin-dca.svc.cluster.local:8082` (not `dca-server`)
10. **VCLI verifier URL must be LOCAL** - vcli.yaml must use `http://verifier.verifier.svc.cluster.local:8080`, NOT production verifier. This is configured via `vcli-config` ConfigMap in verifier namespace.
11. **All workers must use the SAME relay** - verifier worker and DCA worker must both use production relay (`https://api.vultisig.com/router`) for TSS coordination. Check with: `kubectl exec -n verifier deploy/worker -- env | grep RELAY`

### TSS Operations

12. **TSS timeouts are 30 seconds max** - don't extend, retry instead
13. **4-party TSS reshare requires all parties on same relay** - vcli, Fast Vault Server, verifier worker, and DCA plugin worker must all communicate through the same relay
14. **Restart workers after configmap changes** - ConfigMap changes require pod restart: `kubectl rollout restart deployment/worker -n verifier`

### Database & Plugin Configuration

15. **Plugin pricing required for policy creation** - Each plugin needs pricing entries in the `pricings` table. Without them, policy creation fails with "billing policies count does not match plugin pricing count". See `k8s/base/verifier/seed-plugins.yaml` for seeding.
16. **billing.amount must be uint64** - The verifier expects `billing.amount` as a number, not a string. vcli `policy generate` now fetches pricing from verifier and uses correct types.

### MinIO Bucket Configuration

17. **Keyshares stored in correct buckets** - Verifier stores in `vultisig-verifier`, DCA plugin stores in `vultisig-dca`. If keyshare is in wrong bucket, policy verification fails with "Invalid policy signature"

### Queue & Task Routing

18. **DCA Queue Routing** - DCA services use `TASK_QUEUE_NAME=dca_plugin_queue`. This ensures DCA worker receives reshare tasks and saves keyshares to `vultisig-dca` bucket (not `vultisig-verifier`). K8s manifests have this correctly configured in `k8s/base/dca/server.yaml`, `worker.yaml`, and `scheduler.yaml`.

19. **Policy Billing Fetch** - When generating policies, vcli fetches pricing from verifier. If fetch fails, manually add billing entries:
    ```json
    "billing": [
      {"type": "once", "amount": 0, "asset": "usdc"},
      {"type": "per-tx", "amount": 0, "asset": "usdc"}
    ]
    ```
    vcli v1.0.3+ handles this automatically.

20. **4-Party Reshare Validation** - After plugin install, verify:
    - 4 distinct parties: CLI + Fast Vault + Verifier Worker + DCA Worker
    - Both MinIO buckets have keyshares: `Verifier (MinIO): ✓` AND `DCA Plugin (MinIO): ✓`
    - Party names should be distinct (e.g., `vcli-xxx`, `Server-xxx`, `verifier-xxx`, `dca-worker-xxx`)

### Relay Configuration (Critical)

21. **Relay URL Must Match Across All Parties** - All TSS parties (vcli, verifier worker, DCA worker) must connect to the **same relay server**. vcli uses hardcoded `https://api.vultisig.com/router`, so K8s workers MUST be patched to use the production relay.
    - **Base manifests**: Use internal relay (`relay.relay.svc.cluster.local`) - for local dev only
    - **Production overlay**: Patches both namespaces to use `https://api.vultisig.com/router`
    - **Always deploy with**: `kubectl apply -k k8s/overlays/production` (NOT `k8s/base`)
    - If workers use different relays, reshare will hang at 2 parties forever

### Database Seeding

22. **Pricing Table Has No Unique Constraint** - The `pricings` table lacks a unique constraint on `(type, plugin_id, frequency)`. The `ON CONFLICT DO NOTHING` clause is ineffective without a constraint. Running the seed script multiple times creates duplicates.
    - **Fix**: Seed SQL now uses `DELETE + INSERT` pattern to prevent duplicates
    - **If duplicates exist**: Clean up with `DELETE FROM pricings WHERE plugin_id = 'vultisig-dca-0000' AND id NOT IN (SELECT id FROM pricings WHERE plugin_id = 'vultisig-dca-0000' ORDER BY created_at ASC LIMIT 2);`

---

## Common Errors & Fixes

### TSS: Only 2 parties joining (expected 4)

**Symptom:** Plugin install shows "Waiting for more parties... parties=2" forever

**Root Cause:** vcli uses hardcoded relay (`https://api.vultisig.com/router`) but K8s workers are connecting to internal relay. All parties must use the SAME relay server to coordinate the TSS session.

**Diagnosis:**
```bash
# Check relay URLs for all workers
kubectl exec -n verifier deploy/worker -- env | grep RELAY
kubectl exec -n plugin-dca deploy/worker -- env | grep RELAY

# Both should show: RELAY_URL=https://api.vultisig.com/router
# If they show relay.relay.svc.cluster.local, production overlay wasn't applied
```

**Fix:**
```bash
# Ensure production overlay is applied (patches relay URLs)
kubectl apply -k k8s/overlays/production

# Verify relay configmaps are patched
kubectl get cm relay-config -n verifier -o yaml | grep url
kubectl get cm relay-config -n plugin-dca -o yaml | grep url
# Both should show: url: "https://api.vultisig.com/router"

# Restart workers to pick up new config
kubectl rollout restart deployment/worker -n verifier
kubectl rollout restart deployment/worker -n plugin-dca
```

### Policy creation fails with 400: "billing.amount expected uint64"

**Symptom:** `vcli policy add` fails with type error

**Cause:** Policy JSON has `"amount": "0"` (string) instead of `"amount": 0` (number)

**Fix:** Use latest vcli which generates correct types, or manually edit policy JSON

### Policy creation fails with 500: "billing count mismatch"

**Symptom:** `vcli policy add` fails with "billing policies count (N) does not match plugin pricing count (M)"

**Causes:**
1. Plugin has no pricing entries (M=0)
2. Plugin has duplicate pricing entries (M>2, e.g., 6 instead of 2)

**Diagnosis:**
```bash
# Check how many pricing entries exist
kubectl exec -n infra deploy/postgres -- psql -U postgres -d vultisig-verifier -c "
SELECT type, COUNT(*) FROM pricings WHERE plugin_id = 'vultisig-dca-0000' GROUP BY type;
"
# Expected: once=1, per-tx=1 (total 2)
```

**Fix (if duplicates exist):**
```bash
# Clean up duplicates, keeping only oldest entries
kubectl exec -n infra deploy/postgres -- psql -U postgres -d vultisig-verifier -c "
DELETE FROM pricings
WHERE plugin_id = 'vultisig-dca-0000'
  AND id NOT IN (
    SELECT id FROM pricings
    WHERE plugin_id = 'vultisig-dca-0000'
    ORDER BY created_at ASC
    LIMIT 2
  );
"
```

**Fix (if no entries exist):**
```bash
# Add pricing entries to database
kubectl exec -n infra deploy/postgres -- psql -U postgres -d vultisig-verifier -c "
INSERT INTO pricings (type, frequency, amount, asset, metric, plugin_id, created_at, updated_at)
VALUES
    ('once', NULL, 0, 'usdc', 'fixed', 'vultisig-dca-0000', NOW(), NOW()),
    ('per-tx', NULL, 0, 'usdc', 'fixed', 'vultisig-dca-0000', NOW(), NOW());
"
```

### Policy creation fails with 403: "Invalid policy signature"

**Symptom:** `vcli policy add` fails after TSS keysign succeeds

**Cause:** Plugin keyshare not found in correct MinIO bucket

**Fix:**
```bash
# Check where keyshare is stored
kubectl exec -n infra minio-0 -- mc ls local/vultisig-verifier/
kubectl exec -n infra minio-0 -- mc ls local/vultisig-dca/

# If keyshare is in wrong bucket, copy it:
kubectl exec -n infra minio-0 -- mc cp \
  local/vultisig-verifier/vultisig-dca-0000-<pubkey>.vult \
  local/vultisig-dca/vultisig-dca-0000-<pubkey>.vult
```
