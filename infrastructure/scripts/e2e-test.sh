#!/bin/bash
#
# E2E Test Script - Full end-to-end test of Vultisig DCA plugin
#
# Prerequisites:
#   - kubectl configured with cluster access
#   - k8s-start.sh has been run successfully
#   - Vault file mounted at /vault/vault.vult in vcli pod
#
# Usage:
#   ./e2e-test.sh                    # Run full E2E test
#   ./e2e-test.sh --skip-import      # Skip vault import (already imported)
#   ./e2e-test.sh --skip-install     # Skip plugin install (already installed)
#

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
SKIP_IMPORT=false
SKIP_INSTALL=false
VAULT_PASSWORD="${VAULT_PASSWORD:-Password123}"

for arg in "$@"; do
    case $arg in
        --skip-import)
            SKIP_IMPORT=true
            ;;
        --skip-install)
            SKIP_INSTALL=true
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
echo -e "${CYAN}  Vultisig E2E Test${NC}"
echo -e "${CYAN}==========================================${NC}"
echo ""

# Verify cluster connection
if ! kubectl cluster-info &>/dev/null; then
    echo -e "${RED}ERROR: Cannot connect to cluster${NC}"
    echo "Set KUBECONFIG or ensure kubectl is configured"
    exit 1
fi

# Verify vcli pod exists
if ! kubectl -n verifier get pod vcli &>/dev/null; then
    echo -e "${RED}ERROR: vcli pod not found in verifier namespace${NC}"
    echo "Run k8s-start.sh first"
    exit 1
fi

# Helper function to run vcli commands
run_vcli() {
    kubectl exec -n verifier vcli -- timeout 30 vcli "$@"
}

# ============================================
# STEP 1: Import Vault
# ============================================

if ! $SKIP_IMPORT; then
    echo -e "${CYAN}[1/4] Importing vault...${NC}"

    if run_vcli vault import --file /vault/vault.vult --password "$VAULT_PASSWORD" 2>&1; then
        echo -e "  ${GREEN}✓${NC} Vault imported successfully"
    else
        echo -e "  ${RED}✗${NC} Vault import failed"
        exit 1
    fi
else
    echo -e "${YELLOW}[1/4] Skipping vault import (--skip-import)${NC}"
fi

# Verify vault
echo -e "  ${CYAN}Verifying vault...${NC}"
VAULT_DETAILS=$(run_vcli vault details 2>&1 || echo "")
if echo "$VAULT_DETAILS" | grep -q "ECDSA\|Public Key"; then
    echo -e "  ${GREEN}✓${NC} Vault verified"
else
    echo -e "  ${RED}✗${NC} Vault verification failed"
    echo "$VAULT_DETAILS"
    exit 1
fi

# ============================================
# STEP 2: Install DCA Plugin
# ============================================

if ! $SKIP_INSTALL; then
    echo -e "${CYAN}[2/4] Installing DCA plugin (4-party TSS reshare)...${NC}"
    echo -e "  ${YELLOW}⏳${NC} This may take 10-30 seconds..."

    INSTALL_OUTPUT=$(run_vcli plugin install dca --password "$VAULT_PASSWORD" 2>&1 || echo "FAILED")

    if echo "$INSTALL_OUTPUT" | grep -q "PLUGIN INSTALLED\|successfully"; then
        echo -e "  ${GREEN}✓${NC} DCA plugin installed"

        # Verify 4 parties
        if echo "$INSTALL_OUTPUT" | grep -q "4 parties"; then
            echo -e "  ${GREEN}✓${NC} 4-party reshare confirmed"
        else
            echo -e "  ${YELLOW}⚠${NC} Party count not confirmed in output"
        fi

        # Verify MinIO buckets
        if echo "$INSTALL_OUTPUT" | grep -q "Verifier (MinIO): ✓"; then
            echo -e "  ${GREEN}✓${NC} Verifier keyshare saved"
        else
            echo -e "  ${YELLOW}⚠${NC} Verifier keyshare not confirmed"
        fi

        if echo "$INSTALL_OUTPUT" | grep -q "DCA Plugin (MinIO): ✓"; then
            echo -e "  ${GREEN}✓${NC} DCA keyshare saved"
        else
            echo -e "  ${YELLOW}⚠${NC} DCA keyshare not confirmed"
        fi
    else
        echo -e "  ${RED}✗${NC} DCA plugin install failed"
        echo "$INSTALL_OUTPUT"
        exit 1
    fi
else
    echo -e "${YELLOW}[2/4] Skipping plugin install (--skip-install)${NC}"
fi

# ============================================
# STEP 3: Generate Policy
# ============================================

echo -e "${CYAN}[3/4] Generating swap policy (USDC → BTC)...${NC}"

GENERATE_OUTPUT=$(run_vcli policy generate \
    --from usdc \
    --to btc \
    --amount 10 \
    --output /tmp/policy.json 2>&1 || echo "FAILED")

if echo "$GENERATE_OUTPUT" | grep -q "Policy saved\|written to"; then
    echo -e "  ${GREEN}✓${NC} Policy generated: /tmp/policy.json"
else
    echo -e "  ${RED}✗${NC} Policy generation failed"
    echo "$GENERATE_OUTPUT"
    exit 1
fi

# ============================================
# STEP 4: Add Policy
# ============================================

echo -e "${CYAN}[4/4] Adding policy to DCA plugin...${NC}"

ADD_OUTPUT=$(run_vcli policy add \
    --plugin dca \
    --policy-file /tmp/policy.json \
    --password "$VAULT_PASSWORD" 2>&1 || echo "FAILED")

if echo "$ADD_OUTPUT" | grep -q "POLICY ADDED SUCCESSFULLY\|Policy ID"; then
    echo -e "  ${GREEN}✓${NC} Policy added successfully"

    # Extract policy ID
    POLICY_ID=$(echo "$ADD_OUTPUT" | grep -o 'Policy ID:.*' | awk '{print $3}' || echo "")
    if [[ -n "$POLICY_ID" ]]; then
        echo -e "  ${CYAN}Policy ID:${NC} $POLICY_ID"
    fi
else
    echo -e "  ${RED}✗${NC} Policy add failed"
    echo "$ADD_OUTPUT"
    exit 1
fi

# ============================================
# VERIFICATION
# ============================================

echo ""
echo -e "${CYAN}==========================================${NC}"
echo -e "${CYAN}  VERIFICATION${NC}"
echo -e "${CYAN}==========================================${NC}"
echo ""

# List policies
echo -e "${CYAN}Listing policies...${NC}"
POLICIES=$(run_vcli policy list --plugin dca 2>&1 || echo "")
if echo "$POLICIES" | grep -q "Policy\|ID\|Active"; then
    echo -e "  ${GREEN}✓${NC} Policies retrieved"
    echo "$POLICIES" | head -20
else
    echo -e "  ${YELLOW}⚠${NC} Could not list policies"
fi

# Check DCA worker logs for swap activity
echo ""
echo -e "${CYAN}Checking DCA worker for swap activity...${NC}"
DCA_LOGS=$(kubectl -n plugin-dca logs deploy/worker --tail=20 2>/dev/null || echo "")
if echo "$DCA_LOGS" | grep -q "swap route found\|tx signed"; then
    echo -e "  ${GREEN}✓${NC} Swap activity detected in worker logs"
    echo "$DCA_LOGS" | grep -E "swap route|tx signed|txHash" | tail -5
else
    echo -e "  ${YELLOW}⚠${NC} No swap activity yet (policy may be scheduled for later)"
fi

# ============================================
# RESULT
# ============================================

echo ""
echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}  E2E TEST PASSED${NC}"
echo -e "${GREEN}==========================================${NC}"
echo ""
echo -e "  ${GREEN}✓${NC} Vault imported"
echo -e "  ${GREEN}✓${NC} DCA plugin installed (4-party TSS)"
echo -e "  ${GREEN}✓${NC} Policy generated (USDC → BTC)"
echo -e "  ${GREEN}✓${NC} Policy added to plugin"
echo ""
echo -e "  ${CYAN}Next steps:${NC}"
echo -e "    - Check policy status: kubectl exec -n verifier vcli -- vcli policy status <POLICY_ID>"
echo -e "    - Monitor worker logs: kubectl -n plugin-dca logs -f deploy/worker"
echo -e "    - Stop services: ./infrastructure/scripts/k8s-stop.sh"
echo ""
