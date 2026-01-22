# Deployment Notes - 2026-01-22

Tracking issues encountered during one-shot E2E deployment.

## Prerequisites Provided
- [x] Hetzner API key (terraform.tfvars)
- [ ] Vault keyshare (local/keyshares/FastPlugin1-a06a-share2of2.vult)
- [ ] Vault password

## Deployment Steps

### Step 1: Terraform Apply
Status: COMPLETE
- Master: 49.13.58.177 (cax11, fsn1)
- Worker: 167.235.246.209 (cax31, fsn1)
- Volumes: postgres, redis, minio attached

### Step 2: K3s Cluster Setup
Status: COMPLETE (manual worker install needed due to script issue)
- Master: vultisig-master (49.13.58.177) Ready
- Worker: vultisig-worker-fsn1 (167.235.246.209) Ready
- CSI Driver: Installed

### Step 3: K8s Services Deployment
Status: COMPLETE
- All pods running successfully
- Infrastructure: postgres, redis, minio
- Verifier: verifier, worker, tx-indexer, vcli
- Plugin-DCA: server-swap, server-send, worker, scheduler, tx-indexer

### Step 4: E2E Test
Status: PASSED ✅
- vault import: Success (7.4s)
- plugin install: Success (31s) - 4-party TSS reshare
- policy generate: Success
- policy add: Success (9.1s) - Policy ID: e0db3699-e574-4e36-8395-4a523e4d307f

---

## Issues Encountered

### Issue 1: SSH key path wrong in setup-env.sh
- **Problem**: Terraform generated setup-env.sh with relative path `./../../.ssh/id_ed25519` which doesn't work from vcli root
- **Fix**: Changed main.tf to use `./.ssh/id_ed25519` (relative to vcli root where setup-env.sh lives)

### Issue 2: setup-cluster.sh hardcodes 3 workers
- **Problem**: Script loops over fsn1/nbg1/hel1 but terraform only creates 1 worker in fsn1
- **Fix TODO**: Script should dynamically get worker list from setup-env.sh instead of hardcoding regions

### Issue 3: GHCR images are AMD64 only - ARM nodes won't work
- **Problem**: cax* servers are ARM64, but GHCR images only have AMD64 builds
- **Error**: "exec format error" and "no match for platform in manifest: not found"
- **Fix**: Must use cpx* (AMD64) servers instead of cax* (ARM64)
- **Cost Impact**: cpx31 (€15.59/mo) vs cax31 (€14.99/mo) - similar cost

### Issue 4: cpx* servers unavailable in all EU regions
- **Problem**: cpx* servers are out of capacity in fsn1, nbg1, and hel1
- **Fix**: Switched to dedicated AMD ccx* servers (ccx13 for master, ccx23 for worker) in hel1 (Helsinki)
- **Cost**: ccx13 ~€13/mo, ccx23 ~€25/mo (higher than cpx but available)

---

## TODO for Later

(to be filled as issues arise)
