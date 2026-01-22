#!/bin/bash
#
# K8s Start Script - Start Vultisig services on K8s
# Mirrors local `make start` behavior for K8s environment
#
# Usage:
#   ./k8s-start.sh              # Start with production overlay (api.vultisig.com)
#   ./k8s-start.sh --local      # Start with local overlay (internal relay)
#   ./k8s-start.sh --skip-seed  # Skip database seeding
#
# Prerequisites:
#   - kubectl configured with cluster access
#   - k8s/secrets.yaml must exist with valid secrets
#   - KUBECONFIG env var or ~/.kube/config or ./kube/config

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VCLI_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Parse flags
OVERLAY="production"
SKIP_SEED=false
for arg in "$@"; do
    case $arg in
        --local)
            OVERLAY="local"
            ;;
        --skip-seed)
            SKIP_SEED=true
            ;;
    esac
done

# Find kubeconfig
if [[ -z "$KUBECONFIG" ]]; then
    if [[ -f "$VCLI_DIR/.kube/config" ]]; then
        export KUBECONFIG="$VCLI_DIR/.kube/config"
    elif [[ -f "$HOME/.kube/config" ]]; then
        export KUBECONFIG="$HOME/.kube/config"
    fi
fi

echo -e "${CYAN}==========================================${NC}"
echo -e "${CYAN}  Vultisig K8s Start${NC}"
echo -e "${CYAN}  Overlay: $OVERLAY${NC}"
echo -e "${CYAN}==========================================${NC}"
echo ""

# Verify cluster connection
if ! kubectl cluster-info &>/dev/null; then
    echo -e "${RED}ERROR: Cannot connect to cluster${NC}"
    echo "Set KUBECONFIG or ensure kubectl is configured"
    exit 1
fi

echo -e "${YELLOW}Connected to cluster:${NC} $(kubectl config current-context)"
echo ""

# Check secrets exist
if ! kubectl get -f "$VCLI_DIR/k8s/secrets.yaml" &>/dev/null 2>&1; then
    if [[ ! -f "$VCLI_DIR/k8s/secrets.yaml" ]]; then
        echo -e "${RED}ERROR: k8s/secrets.yaml not found${NC}"
        echo "Copy secrets-template.yaml and fill in values:"
        echo "  cp k8s/secrets-template.yaml k8s/secrets.yaml"
        exit 1
    fi
fi

# ============================================
# STEP 1: Apply kustomize overlay (creates namespaces)
# ============================================

echo -e "${CYAN}Applying $OVERLAY overlay...${NC}"
kubectl apply -k "$VCLI_DIR/k8s/overlays/$OVERLAY" 2>&1 | grep -v "^#" || true
echo -e "  ${GREEN}✓${NC} Manifests applied"

# ============================================
# STEP 2: Apply secrets (namespaces now exist)
# ============================================

echo -e "${CYAN}Applying secrets...${NC}"
kubectl apply -f "$VCLI_DIR/k8s/secrets.yaml"
echo -e "  ${GREEN}✓${NC} Secrets applied"

# ============================================
# STEP 3: Delete existing jobs (they're immutable) and recreate
# ============================================

echo -e "${CYAN}Recreating jobs...${NC}"
kubectl -n infra delete job minio-init --ignore-not-found 2>/dev/null || true
kubectl -n verifier delete job seed-plugins --ignore-not-found 2>/dev/null || true
sleep 2
# Reapply to recreate jobs
kubectl apply -k "$VCLI_DIR/k8s/overlays/$OVERLAY" 2>&1 | grep -v "^#" | grep -v "unchanged" || true
echo -e "  ${GREEN}✓${NC} Jobs recreated"

# ============================================
# STEP 4: Wait for infrastructure
# ============================================

echo -e "${CYAN}Waiting for infrastructure...${NC}"

echo -e "  ${YELLOW}⏳${NC} PostgreSQL..."
kubectl -n infra wait --for=condition=ready pod -l app=postgres --timeout=300s
echo -e "  ${GREEN}✓${NC} PostgreSQL ready"

echo -e "  ${YELLOW}⏳${NC} Redis..."
kubectl -n infra wait --for=condition=ready pod -l app=redis --timeout=120s
echo -e "  ${GREEN}✓${NC} Redis ready"

echo -e "  ${YELLOW}⏳${NC} MinIO..."
kubectl -n infra wait --for=condition=ready pod -l app=minio --timeout=120s
echo -e "  ${GREEN}✓${NC} MinIO ready"

# ============================================
# STEP 5: Wait for application services
# ============================================

echo -e "${CYAN}Waiting for application services...${NC}"

echo -e "  ${YELLOW}⏳${NC} Verifier..."
kubectl -n verifier wait --for=condition=ready pod -l app=verifier,component=api --timeout=300s
echo -e "  ${GREEN}✓${NC} Verifier API ready"

kubectl -n verifier wait --for=condition=ready pod -l app=verifier,component=worker --timeout=120s
echo -e "  ${GREEN}✓${NC} Verifier Worker ready"

echo -e "  ${YELLOW}⏳${NC} DCA Plugin..."
kubectl -n plugin-dca wait --for=condition=ready pod -l app=dca,component=server-swap --timeout=120s 2>/dev/null || \
    kubectl -n plugin-dca wait --for=condition=ready pod -l app=dca --timeout=120s 2>/dev/null || true
echo -e "  ${GREEN}✓${NC} DCA services ready"

# ============================================
# STEP 6: Flush Redis (clean start)
# ============================================

echo -e "${CYAN}Flushing Redis for clean start...${NC}"

REDIS_PASSWORD=$(kubectl -n infra get secret redis -o jsonpath='{.data.password}' 2>/dev/null | base64 -d) || REDIS_PASSWORD=""

if [[ -n "$REDIS_PASSWORD" ]]; then
    # Redis is a StatefulSet, not Deployment
    kubectl -n infra exec redis-0 -- redis-cli -a "$REDIS_PASSWORD" FLUSHALL 2>/dev/null && \
        echo -e "  ${GREEN}✓${NC} Redis flushed" || \
        echo -e "  ${YELLOW}⚠${NC} Redis flush failed"
fi

# ============================================
# STEP 6: Run seed job
# ============================================

if ! $SKIP_SEED; then
    echo -e "${CYAN}Seeding database...${NC}"

    # Delete old seed job if exists
    kubectl -n verifier delete job seed-plugins --ignore-not-found 2>/dev/null || true
    sleep 2

    # Apply seed-plugins manifest
    if [[ -f "$VCLI_DIR/k8s/base/verifier/seed-plugins.yaml" ]]; then
        kubectl apply -f "$VCLI_DIR/k8s/base/verifier/seed-plugins.yaml"

        # Wait for seed job to complete
        echo -e "  ${YELLOW}⏳${NC} Waiting for seed job..."
        for i in {1..60}; do
            STATUS=$(kubectl -n verifier get job seed-plugins -o jsonpath='{.status.succeeded}' 2>/dev/null || echo "")
            if [[ "$STATUS" == "1" ]]; then
                echo -e "  ${GREEN}✓${NC} Database seeded"
                break
            fi
            FAILED=$(kubectl -n verifier get job seed-plugins -o jsonpath='{.status.failed}' 2>/dev/null || echo "")
            if [[ "$FAILED" -ge "3" ]]; then
                echo -e "  ${RED}✗${NC} Seed job failed"
                kubectl -n verifier logs job/seed-plugins --tail=20
                break
            fi
            sleep 2
        done
    else
        echo -e "  ${YELLOW}⚠${NC} seed-plugins.yaml not found, skipping"
    fi
else
    echo -e "${YELLOW}Skipping database seeding (--skip-seed)${NC}"
fi

# ============================================
# STEP 7: Comprehensive Verification
# ============================================

echo -e "${CYAN}==========================================${NC}"
echo -e "${CYAN}  COMPREHENSIVE VERIFICATION${NC}"
echo -e "${CYAN}==========================================${NC}"
echo ""

VERIFICATION_FAILED=false

# --- 7.1 MinIO Buckets ---
echo -e "${CYAN}[1/6] Verifying MinIO buckets...${NC}"

# Wait for minio-init job to complete
for i in {1..30}; do
    MINIO_INIT_STATUS=$(kubectl -n infra get job minio-init -o jsonpath='{.status.succeeded}' 2>/dev/null || echo "")
    if [[ "$MINIO_INIT_STATUS" == "1" ]]; then
        break
    fi
    sleep 2
done

# Check buckets exist using mc or direct API
MINIO_POD=$(kubectl -n infra get pods -l app=minio -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [[ -n "$MINIO_POD" ]]; then
    BUCKETS=$(kubectl -n infra exec "$MINIO_POD" -- ls /data/ 2>/dev/null || echo "")

    if echo "$BUCKETS" | grep -q "vultisig-verifier"; then
        echo -e "  ${GREEN}✓${NC} vultisig-verifier bucket exists"
    else
        echo -e "  ${RED}✗${NC} vultisig-verifier bucket MISSING"
        VERIFICATION_FAILED=true
    fi

    if echo "$BUCKETS" | grep -q "vultisig-dca"; then
        echo -e "  ${GREEN}✓${NC} vultisig-dca bucket exists"
    else
        echo -e "  ${RED}✗${NC} vultisig-dca bucket MISSING"
        VERIFICATION_FAILED=true
    fi
else
    echo -e "  ${RED}✗${NC} MinIO pod not found"
    VERIFICATION_FAILED=true
fi

# --- 7.2 Database Connectivity & Seeding ---
echo -e "${CYAN}[2/6] Verifying database...${NC}"

# Check if DCA plugin is seeded
PLUGIN_COUNT=$(kubectl -n infra exec postgres-0 -- psql -U postgres -d vultisig_verifier -t -c "SELECT COUNT(*) FROM plugins WHERE id = 'vultisig-dca-0000';" 2>/dev/null | tr -d ' ' || echo "0")
if [[ "$PLUGIN_COUNT" == "1" ]]; then
    echo -e "  ${GREEN}✓${NC} DCA plugin seeded (vultisig-dca-0000)"
else
    echo -e "  ${RED}✗${NC} DCA plugin NOT seeded (count: $PLUGIN_COUNT)"
    VERIFICATION_FAILED=true
fi

# Check verifier tables exist
TABLES=$(kubectl -n infra exec postgres-0 -- psql -U postgres -d vultisig_verifier -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | tr -d ' ' || echo "0")
if [[ "$TABLES" -gt "5" ]]; then
    echo -e "  ${GREEN}✓${NC} Verifier database tables exist ($TABLES tables)"
else
    echo -e "  ${RED}✗${NC} Verifier database tables missing ($TABLES tables)"
    VERIFICATION_FAILED=true
fi

# --- 7.3 Redis Connectivity ---
echo -e "${CYAN}[3/6] Verifying Redis...${NC}"

REDIS_PASSWORD=$(kubectl -n infra get secret redis -o jsonpath='{.data.password}' 2>/dev/null | base64 -d) || REDIS_PASSWORD=""
REDIS_PING=$(kubectl -n infra exec redis-0 -- redis-cli -a "$REDIS_PASSWORD" PING 2>/dev/null || echo "")
if [[ "$REDIS_PING" == "PONG" ]]; then
    echo -e "  ${GREEN}✓${NC} Redis responding"
else
    echo -e "  ${RED}✗${NC} Redis not responding"
    VERIFICATION_FAILED=true
fi

# Check Redis is empty (no stale sessions)
REDIS_KEYS=$(kubectl -n infra exec redis-0 -- redis-cli -a "$REDIS_PASSWORD" DBSIZE 2>/dev/null | grep -o '[0-9]*' || echo "0")
if [[ "$REDIS_KEYS" == "0" ]]; then
    echo -e "  ${GREEN}✓${NC} Redis is clean (0 keys)"
else
    echo -e "  ${YELLOW}⚠${NC} Redis has $REDIS_KEYS keys (may have stale sessions)"
fi

# --- 7.4 Pod Health (Running + No Restarts) ---
echo -e "${CYAN}[4/6] Verifying pod health...${NC}"

check_pod_health() {
    local namespace=$1
    local label=$2
    local name=$3

    POD_INFO=$(kubectl -n "$namespace" get pods -l "$label" -o jsonpath='{.items[0].status.phase}:{.items[0].status.containerStatuses[0].restartCount}' 2>/dev/null || echo ":")
    POD_PHASE=$(echo "$POD_INFO" | cut -d: -f1)
    RESTART_COUNT=$(echo "$POD_INFO" | cut -d: -f2)

    if [[ "$POD_PHASE" == "Running" ]] && [[ "$RESTART_COUNT" == "0" ]]; then
        echo -e "  ${GREEN}✓${NC} $name: Running (0 restarts)"
        return 0
    elif [[ "$POD_PHASE" == "Running" ]]; then
        echo -e "  ${YELLOW}⚠${NC} $name: Running ($RESTART_COUNT restarts)"
        return 0
    else
        echo -e "  ${RED}✗${NC} $name: $POD_PHASE"
        return 1
    fi
}

check_pod_health "verifier" "app=verifier,component=api" "Verifier API" || VERIFICATION_FAILED=true
check_pod_health "verifier" "app=verifier,component=worker" "Verifier Worker" || VERIFICATION_FAILED=true
check_pod_health "plugin-dca" "app=dca,component=server-swap" "DCA Server (swap)" || VERIFICATION_FAILED=true
check_pod_health "plugin-dca" "app=dca,component=worker" "DCA Worker" || VERIFICATION_FAILED=true
check_pod_health "plugin-dca" "app=dca,component=scheduler" "DCA Scheduler" || VERIFICATION_FAILED=true

# --- 7.5 Service Health Endpoints ---
echo -e "${CYAN}[5/6] Verifying service endpoints...${NC}"

# Verifier API
if kubectl -n verifier exec deploy/verifier -- wget -qO- --timeout=5 http://localhost:8080/ &>/dev/null; then
    echo -e "  ${GREEN}✓${NC} Verifier API HTTP responding"
else
    echo -e "  ${RED}✗${NC} Verifier API HTTP not responding"
    VERIFICATION_FAILED=true
fi

# DCA Server (swap)
if kubectl -n plugin-dca exec deploy/server-swap -- wget -qO- --timeout=5 http://localhost:8082/ &>/dev/null; then
    echo -e "  ${GREEN}✓${NC} DCA Server HTTP responding"
else
    echo -e "  ${RED}✗${NC} DCA Server HTTP not responding"
    VERIFICATION_FAILED=true
fi

# --- 7.6 Worker Queue Configuration ---
echo -e "${CYAN}[6/6] Verifying worker queue configuration...${NC}"

# Check DCA worker logs for queue connection
DCA_WORKER_LOGS=$(kubectl -n plugin-dca logs deploy/worker --tail=50 2>/dev/null || echo "")
if echo "$DCA_WORKER_LOGS" | grep -q "dca_plugin_queue\|starting worker"; then
    echo -e "  ${GREEN}✓${NC} DCA Worker started (should use dca_plugin_queue)"
else
    echo -e "  ${YELLOW}⚠${NC} DCA Worker queue config not confirmed in logs"
fi

# Check Verifier worker logs
VERIFIER_WORKER_LOGS=$(kubectl -n verifier logs deploy/worker --tail=50 2>/dev/null || echo "")
if echo "$VERIFIER_WORKER_LOGS" | grep -q "starting worker\|asynq"; then
    echo -e "  ${GREEN}✓${NC} Verifier Worker started (uses default queue)"
else
    echo -e "  ${YELLOW}⚠${NC} Verifier Worker queue config not confirmed in logs"
fi

# ============================================
# VERIFICATION RESULT
# ============================================

echo ""
if $VERIFICATION_FAILED; then
    echo -e "${RED}==========================================${NC}"
    echo -e "${RED}  VERIFICATION FAILED${NC}"
    echo -e "${RED}==========================================${NC}"
    echo ""
    echo -e "  ${RED}Some checks failed. Review above and fix before proceeding.${NC}"
    echo ""
    echo -e "  ${CYAN}Debug commands:${NC}"
    echo -e "    kubectl -n infra logs job/minio-init"
    echo -e "    kubectl -n verifier logs job/seed-plugins"
    echo -e "    kubectl -n plugin-dca logs deploy/worker"
    echo ""
    exit 1
fi

# ============================================
# SUMMARY
# ============================================

echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}  ALL VERIFICATIONS PASSED${NC}"
echo -e "${GREEN}==========================================${NC}"
echo ""
echo -e "${CYAN}==========================================${NC}"
echo -e "${GREEN}  STARTUP COMPLETE${NC}"
echo -e "${CYAN}==========================================${NC}"
echo ""
echo -e "  ${CYAN}Overlay:${NC} $OVERLAY"
if [[ "$OVERLAY" == "production" ]]; then
    echo -e "  ${CYAN}Relay:${NC} https://api.vultisig.com/router"
    echo -e "  ${CYAN}VultiServer:${NC} https://api.vultisig.com"
else
    echo -e "  ${CYAN}Relay:${NC} relay.relay.svc.cluster.local"
    echo -e "  ${CYAN}VultiServer:${NC} vultiserver.vultiserver.svc.cluster.local"
fi
echo ""
echo -e "  ${CYAN}Services:${NC}"
echo -e "    verifier/verifier     (API)"
echo -e "    verifier/worker       (TSS worker)"
echo -e "    plugin-dca/server     (DCA API)"
echo -e "    plugin-dca/worker     (DCA TSS worker)"
echo -e "    plugin-dca/scheduler  (Job scheduler)"
echo ""
echo -e "  ${CYAN}Infrastructure:${NC}"
echo -e "    infra/postgres"
echo -e "    infra/redis"
echo -e "    infra/minio"
echo ""

# Show pod status
echo -e "  ${CYAN}Pod Status:${NC}"
kubectl get pods -n verifier --no-headers 2>/dev/null | awk '{print "    verifier/" $1 ": " $3}'
kubectl get pods -n plugin-dca --no-headers 2>/dev/null | awk '{print "    plugin-dca/" $1 ": " $3}'
echo ""

echo -e "  ${CYAN}Logs:${NC}"
echo -e "    kubectl -n verifier logs -f deploy/verifier"
echo -e "    kubectl -n verifier logs -f deploy/worker"
echo -e "    kubectl -n plugin-dca logs -f deploy/worker"
echo ""
echo -e "  ${CYAN}Stop:${NC} ./infrastructure/scripts/k8s-stop.sh"
echo ""
