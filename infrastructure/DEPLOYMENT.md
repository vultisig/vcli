# Vultisig Kubernetes Deployment Guide

Production-ready Kubernetes deployment for Vultisig services on Hetzner Cloud.

---

## Prerequisites

- `hcloud` CLI installed and configured
- SSH key registered in Hetzner Cloud
- `.env.k8s` file with secrets (see [Secrets Configuration](#secrets-configuration))
- `kubectl` installed locally

---

## Quick Start (Full Deployment)

```bash
cd /Users/dev/dev/vultisig/vcli

# 1. Check server type availability before deploying
./infrastructure/scripts/check-availability.sh

# 2. Create infrastructure with Terraform
cd infrastructure/terraform
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your Hetzner API token
terraform init && terraform apply

# 3. Setup K3s on nodes
cd ../..
source setup-env.sh
./infrastructure/scripts/setup-cluster.sh

# 4. Deploy and start services
export KUBECONFIG=$(pwd)/.kube/config
./infrastructure/scripts/k8s-start.sh
```

### E2E Test Commands

```bash
# Import vault
kubectl exec -n verifier vcli -- vcli vault import --file /vault/vault.vult --password "Password123"

# Install DCA plugin (4-party TSS reshare)
kubectl exec -n verifier vcli -- vcli plugin install dca --password "Password123"

# Generate swap policy
kubectl exec -n verifier vcli -- vcli policy generate --from usdc --to btc --amount 10 --output /tmp/policy.json

# Add policy
kubectl exec -n verifier vcli -- vcli policy add --plugin dca --policy-file /tmp/policy.json --password "Password123"

# Monitor
kubectl exec -n verifier vcli -- vcli policy list --plugin dca
kubectl -n plugin-dca logs -f deploy/worker
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

**IMPORTANT: GHCR images are AMD64 only.** ARM servers (cax*) will fail with "exec format error".

| Type Family | Architecture | Notes |
|-------------|--------------|-------|
| `cax*` | ARM64 | **NOT compatible with GHCR images** |
| `cpx*` | AMD64 (shared) | Often out of stock in EU regions |
| `ccx*` | AMD64 (dedicated) | **Recommended** - available everywhere |
| `cx*` | Intel (shared) | Check availability |

**Current working configuration:**
```
Master: ccx13 (2 dedicated vCPU, 8GB RAM) - ~€13/mo
Worker: ccx23 (4 dedicated vCPU, 16GB RAM) - ~€25/mo
Region: hel1 (Helsinki)
```

**Check availability before deployment:**
```bash
# Use the availability script
./infrastructure/scripts/check-availability.sh

# Or manually
hcloud server-type describe ccx23 -o json | jq '.prices[] | "\(.location): \(.price_monthly.gross)"'
```

**If cpx* is out of stock:**
Switch to dedicated ccx* servers (slightly more expensive but always available).

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

## Phase 1: Infrastructure Setup (Terraform)

The recommended approach uses Terraform for reproducible infrastructure.

### Prerequisites

1. **Hetzner API Token**: Get from Hetzner Cloud Console → Security → API Tokens
2. **Vault keyshare file**: Export from Vultisig mobile app to `local/keyshares/`
3. **hcloud CLI**: `brew install hcloud` (macOS) or download from Hetzner

### Check Availability

Before deploying, check server type availability in your target region:

```bash
./infrastructure/scripts/check-availability.sh
```

This shows:
- Specs for master (ccx13) and worker (ccx23) server types
- Pricing and availability per region
- Which regions have capacity

### Deploy with Terraform

```bash
cd infrastructure/terraform

# Create config file
cp terraform.tfvars.example terraform.tfvars

# Edit with your Hetzner API token
echo 'hcloud_token = "your-token-here"' > terraform.tfvars

# Optional: customize server types or region
# Edit variables.tf to change defaults, or override:
# terraform apply -var="worker_server_type=ccx33" -var="regions=[\"nbg1\"]"

# Initialize and apply
terraform init
terraform apply
```

**What Terraform creates:**
- Master node (ccx13) with K3s control plane
- Worker node (ccx23) for workloads
- Persistent volumes: PostgreSQL (50GB), Redis (10GB), MinIO (50GB)
- Private network for cluster communication
- Firewall rules for SSH, K8s API, and NodePorts
- SSH keys (generated if not provided)
- `setup-env.sh` script with all connection details

### Generated Files

After `terraform apply`, these files are created in the vcli root:
- `setup-env.sh` - Environment variables for cluster setup
- `.ssh/id_ed25519` - SSH private key (if generated)
- `.ssh/id_ed25519.pub` - SSH public key

### Manual Server Creation (Alternative)

If you prefer manual control:

```bash
export HCLOUD_TOKEN="your-token"

# Create servers (use ccx* for AMD64 compatibility)
hcloud server create --type ccx13 --image ubuntu-24.04 --name vultisig-master --location hel1 --ssh-key your-key
hcloud server create --type ccx23 --image ubuntu-24.04 --name vultisig-worker --location hel1 --ssh-key your-key

# Create volumes
hcloud volume create --name vultisig-postgres --size 50 --location hel1 --format ext4
hcloud volume create --name vultisig-redis --size 10 --location hel1 --format ext4
hcloud volume create --name vultisig-minio --size 50 --location hel1 --format ext4

# Attach volumes to worker
hcloud volume attach vultisig-postgres --server vultisig-worker --automount
hcloud volume attach vultisig-redis --server vultisig-worker --automount
hcloud volume attach vultisig-minio --server vultisig-worker --automount

# Create setup-env.sh manually
cat > setup-env.sh << EOF
export MASTER_IP="$(hcloud server ip vultisig-master)"
export MASTER_PRIVATE_IP="10.1.0.10"
export K3S_TOKEN="$(openssl rand -hex 32)"
export WORKER_HEL1_IP="$(hcloud server ip vultisig-worker)"
export SSH_KEY_PATH="./.ssh/id_ed25519"
EOF
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

**Default deployment (single worker):**
```
┌───────────────────────────────────────────────────────────────┐
│                  Hetzner Cloud (Helsinki - hel1)              │
├───────────────────────────────────────────────────────────────┤
│  vultisig-master (ccx13 - 2 vCPU, 8GB)                        │
│    └── K3s control plane                                      │
│                                                               │
│  vultisig-worker-hel1 (ccx23 - 4 vCPU, 16GB)                  │
│    ├── infra: postgres, redis, minio (with Hetzner volumes)   │
│    ├── verifier: API + worker + tx-indexer + vcli             │
│    └── plugin-dca: server + scheduler + worker + tx-indexer   │
│                                                               │
│  Persistent Volumes (attached to worker):                     │
│    ├── vultisig-postgres (50GB)                               │
│    ├── vultisig-redis (10GB)                                  │
│    └── vultisig-minio (50GB)                                  │
└───────────────────────────────────────────────────────────────┘
```

**Multi-worker deployment (optional):**
```
# To deploy multiple workers, edit variables.tf:
variable "regions" {
  default = ["hel1", "fsn1", "nbg1"]  # Multiple regions
}
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
./infrastructure/scripts/k8s-start.sh              # Deploy and verify
./infrastructure/scripts/k8s-start.sh --skip-seed  # Skip database seeding
```

**External services (production endpoints):**
- Relay: `https://api.vultisig.com/router`
- VultiServer/FastVault: `https://api.vultisig.com`

**What it deploys:**
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
make k8s-start          # Deploy + verify (recommended)
make k8s-stop           # Graceful shutdown
make delete-k8s         # Delete all Kubernetes resources
```

---

## Teardown

### Stop Services (Keep Infrastructure)

```bash
./infrastructure/scripts/k8s-stop.sh
```

This stops K8s services but keeps the servers and volumes for redeployment.

### Full Teardown (Destroy Infrastructure)

```bash
cd infrastructure/terraform
terraform destroy
```

This removes:
- All Hetzner servers (master + workers)
- All Hetzner volumes (postgres, redis, minio data is DELETED)
- Network resources (VPC, subnets)
- Firewall rules
- SSH keys (if generated by Terraform)

**Manual teardown (if Terraform state is lost):**

```bash
export HCLOUD_TOKEN="your-token"

# Delete servers
hcloud server delete vultisig-master
hcloud server delete vultisig-worker-hel1

# Delete volumes (DATA WILL BE LOST)
hcloud volume delete vultisig-postgres
hcloud volume delete vultisig-redis
hcloud volume delete vultisig-minio

# Delete network
hcloud network delete vultisig-network

# Delete firewall
hcloud firewall delete vultisig-firewall

# Delete SSH key (if generated)
hcloud ssh-key delete vultisig-cluster-key
```

---

## Version History

| Date | Version | Changes |
|------|---------|---------|
| 2026-01-22 | 1.1 | Switched to Terraform, AMD64 servers (ccx13/ccx23), added availability check script |
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

21. **Relay URL Must Match Across All Parties** - All TSS parties (vcli, verifier worker, DCA worker) must connect to the **same relay server**. The production overlay configures all workers to use `https://api.vultisig.com/router`.
    - **Deploy command**: `./infrastructure/scripts/k8s-start.sh` (uses production overlay)
    - If workers use different relays, reshare will hang at 2 parties forever

### Database Seeding

22. **Pricing Table Has No Unique Constraint** - The `pricings` table lacks a unique constraint on `(type, plugin_id, frequency)`. The `ON CONFLICT DO NOTHING` clause is ineffective without a constraint. Running the seed script multiple times creates duplicates.
    - **Fix**: Seed SQL now uses `DELETE + INSERT` pattern to prevent duplicates
    - **If duplicates exist**: Clean up with `DELETE FROM pricings WHERE plugin_id = 'vultisig-dca-0000' AND id NOT IN (SELECT id FROM pricings WHERE plugin_id = 'vultisig-dca-0000' ORDER BY created_at ASC LIMIT 2);`

### Node Sizing & Deployment

23. **Node Sizing** - Worker nodes should be ccx23 (4 dedicated vCPU, 16GB) minimum for all services to run without CPU throttling.

24. **Test Vault Secret** - `k8s-start.sh` auto-creates `test-vault` secret from `local/keyshares/FastPlugin1-a06a-share2of2.vult`. Ensure this file exists before running the script.

25. **Rolling Restart After Infra** - After infrastructure restart, application pods need rolling restart to pick up fresh service IPs (e.g., postgres moving nodes). Automated in `k8s-start.sh` as STEP 4.5.

### Architecture Compatibility

26. **GHCR Images are AMD64 Only** - All GHCR images (`ghcr.io/vultisig/*`) are built for AMD64. ARM64 servers (cax*) will fail with "exec format error" or "no match for platform in manifest".
    - **DO NOT use:** cax11, cax21, cax31, cax41 (ARM64)
    - **Use instead:** ccx13, ccx23, ccx33 (AMD64 dedicated) or cpx11, cpx21, cpx31 (AMD64 shared)

27. **Server Availability Varies by Region** - cpx* (shared AMD64) servers are often out of stock in EU regions (fsn1, nbg1, hel1). Use `./infrastructure/scripts/check-availability.sh` to verify before deployment.
    - **Fallback:** ccx* dedicated servers are always available (slightly higher cost)

28. **Terraform Manages Infrastructure** - Use `infrastructure/terraform/` for reproducible deployments:
    ```bash
    terraform apply                    # Create infrastructure
    terraform destroy                  # Tear down all resources
    terraform apply -var="regions=[\"nbg1\"]"  # Override region
    ```

---

## Common Errors & Fixes

### TSS: Only 2 parties joining (expected 4)

**Symptom:** Plugin install shows "Waiting for more parties... parties=2" forever

**Root Cause:** All TSS parties must use the SAME relay server to coordinate sessions.

**Diagnosis:**
```bash
# Check relay URLs for all workers
kubectl exec -n verifier deploy/worker -- env | grep RELAY
kubectl exec -n plugin-dca deploy/worker -- env | grep RELAY

# Both should show: RELAY_URL=https://api.vultisig.com/router
```

**Fix:**
```bash
# Redeploy with k8s-start.sh
./infrastructure/scripts/k8s-start.sh

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
