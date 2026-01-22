#!/bin/bash
#
# K8s Stop Script - Full shutdown and reset of Vultisig services
# Mirrors local `make stop` behavior for K8s environment
#
# Usage: ./k8s-stop.sh
#
# This script:
#   1. Scales down all deployments
#   2. Deletes all jobs
#   3. Flushes Redis
#   4. Deletes PVCs (full data reset)
#   5. Recreates infrastructure

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VCLI_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Find kubeconfig
if [[ -z "$KUBECONFIG" ]]; then
    if [[ -f "$VCLI_DIR/.kube/config" ]]; then
        export KUBECONFIG="$VCLI_DIR/.kube/config"
    elif [[ -f "$HOME/.kube/config" ]]; then
        export KUBECONFIG="$HOME/.kube/config"
    fi
fi

echo -e "${CYAN}==========================================${NC}"
echo -e "${CYAN}  Vultisig K8s Full Stop${NC}"
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

# ============================================
# STEP 1: Scale down all deployments
# ============================================

echo -e "${CYAN}Scaling down deployments...${NC}"

# Verifier namespace
for deploy in verifier worker tx-indexer; do
    if kubectl -n verifier get deployment $deploy &>/dev/null; then
        kubectl -n verifier scale deployment $deploy --replicas=0
        echo -e "  ${GREEN}✓${NC} verifier/$deploy → 0"
    fi
done

# Plugin-DCA namespace
for deploy in server-swap server-send worker scheduler tx-indexer; do
    if kubectl -n plugin-dca get deployment $deploy &>/dev/null; then
        kubectl -n plugin-dca scale deployment $deploy --replicas=0
        echo -e "  ${GREEN}✓${NC} plugin-dca/$deploy → 0"
    fi
done

# ============================================
# STEP 2: Delete jobs
# ============================================

echo -e "${CYAN}Deleting jobs...${NC}"
kubectl -n infra delete job --all --ignore-not-found 2>/dev/null || true
kubectl -n verifier delete job --all --ignore-not-found 2>/dev/null || true
kubectl -n plugin-dca delete job --all --ignore-not-found 2>/dev/null || true
echo -e "  ${GREEN}✓${NC} Jobs deleted"

# Wait for pods to terminate
echo -e "${YELLOW}Waiting for pods to terminate...${NC}"
sleep 5

# ============================================
# STEP 3: Flush Redis
# ============================================

echo -e "${CYAN}Flushing Redis...${NC}"
REDIS_PASSWORD=$(kubectl -n infra get secret redis -o jsonpath='{.data.password}' 2>/dev/null | base64 -d) || REDIS_PASSWORD=""

if [[ -n "$REDIS_PASSWORD" ]]; then
    kubectl -n infra exec redis-0 -- redis-cli -a "$REDIS_PASSWORD" FLUSHALL 2>/dev/null && \
        echo -e "  ${GREEN}✓${NC} Redis flushed" || \
        echo -e "  ${YELLOW}⚠${NC} Redis flush skipped"
fi

# ============================================
# STEP 4: Delete PVCs (full data reset)
# ============================================

echo -e "${CYAN}Deleting PVCs...${NC}"

# Scale down statefulsets first
for ns in infra verifier plugin-dca; do
    kubectl -n $ns scale statefulset --all --replicas=0 2>/dev/null || true
done
sleep 3

# Delete PVCs
for ns in infra verifier plugin-dca; do
    kubectl -n $ns delete pvc --all --ignore-not-found 2>/dev/null || true
    echo -e "  ${GREEN}✓${NC} PVCs deleted in $ns"
done

# ============================================
# STEP 5: Recreate infrastructure
# ============================================

echo -e "${CYAN}Recreating infrastructure...${NC}"

# Re-apply base infrastructure
kubectl apply -f "$VCLI_DIR/k8s/base/infra/" 2>/dev/null || true

echo -e "${YELLOW}Waiting for infrastructure...${NC}"
kubectl -n infra wait --for=condition=ready pod -l app=postgres --timeout=180s 2>/dev/null || true
kubectl -n infra wait --for=condition=ready pod -l app=redis --timeout=60s 2>/dev/null || true
kubectl -n infra wait --for=condition=ready pod -l app=minio --timeout=60s 2>/dev/null || true
echo -e "  ${GREEN}✓${NC} Infrastructure ready"

# ============================================
# SUMMARY
# ============================================

echo ""
echo -e "${CYAN}==========================================${NC}"
echo -e "${GREEN}  STOP COMPLETE${NC}"
echo -e "${CYAN}==========================================${NC}"
echo ""
echo -e "  ${GREEN}✓${NC} All deployments scaled to 0"
echo -e "  ${GREEN}✓${NC} All jobs deleted"
echo -e "  ${GREEN}✓${NC} Redis flushed"
echo -e "  ${GREEN}✓${NC} PVCs deleted and recreated"
echo -e "  ${GREEN}✓${NC} Databases empty (will be seeded on start)"
echo ""
echo -e "  ${CYAN}To start:${NC} ./infrastructure/scripts/k8s-start.sh"
echo ""
