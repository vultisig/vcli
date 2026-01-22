#!/bin/bash
# Check Hetzner server type availability by region before deployment
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

MASTER_TYPE="${MASTER_TYPE:-ccx13}"
WORKER_TYPE="${WORKER_TYPE:-ccx23}"

echo "Checking Hetzner server type availability..."
echo ""

check_type() {
    local type=$1
    local role=$2

    echo "=== $role: $type ==="

    # Get JSON output for reliable parsing
    local json
    json=$(hcloud server-type describe "$type" -o json 2>/dev/null) || {
        echo -e "  ${RED}ERROR: Server type '$type' not found${NC}"
        return 1
    }

    local arch cores memory
    arch=$(echo "$json" | jq -r '.architecture')
    cores=$(echo "$json" | jq -r '.cores')
    memory=$(echo "$json" | jq -r '.memory')

    echo "  Specs: ${cores} vCPU, ${memory}GB RAM, ${arch}"
    echo ""

    # Check each location from prices array
    echo "$json" | jq -r '.prices[] | "\(.location) \(.price_monthly.gross) \(.deprecation.unavailable_after // "available")"' | while read -r location price status; do
        if [[ "$status" == "available" ]]; then
            printf "  %-6s ${GREEN}Available${NC} (€%s/mo)\n" "$location:" "$price"
        elif [[ "$status" == "null" ]]; then
            printf "  %-6s ${GREEN}Available${NC} (€%s/mo)\n" "$location:" "$price"
        else
            # Check if deprecation date has passed
            local unavail_date="${status%%T*}"
            local now_date
            now_date=$(date +%Y-%m-%d)
            if [[ "$unavail_date" < "$now_date" ]]; then
                printf "  %-6s ${RED}Unavailable${NC} (deprecated)\n" "$location:"
            else
                printf "  %-6s ${YELLOW}Deprecating${NC} (until %s)\n" "$location:" "$unavail_date"
            fi
        fi
    done
    echo ""
}

check_type "$MASTER_TYPE" "Master"
check_type "$WORKER_TYPE" "Worker"

# Optional: Test actual stock by attempting server creation (will fail fast if out of stock)
if [[ "${TEST_STOCK:-false}" == "true" ]]; then
    echo "=== Stock Test (attempts create, cancels immediately) ==="
    echo ""
    echo "  Note: This creates real API requests. Failures are expected for out-of-stock."
    echo ""
    for region in fsn1 nbg1 hel1 ash hil; do
        printf "  %-6s $WORKER_TYPE... " "$region:"
        # Try to create - will fail immediately if out of stock
        result=$(hcloud server create --name "stock-check-$$" --type "$WORKER_TYPE" --image ubuntu-24.04 --location "$region" 2>&1) || true
        if echo "$result" | grep -qi "unavailable"; then
            echo -e "${RED}OUT OF STOCK${NC}"
        elif echo "$result" | grep -q "Server [0-9]"; then
            # Server was created - delete it immediately
            server_id=$(echo "$result" | grep -o "Server [0-9]*" | awk '{print $2}')
            hcloud server delete "$server_id" --poll-interval 1s >/dev/null 2>&1 || true
            echo -e "${GREEN}IN STOCK${NC} (test server deleted)"
        else
            echo -e "${YELLOW}ERROR: $result${NC}"
        fi
    done
    echo ""
fi

echo "=== Quick Reference ==="
echo ""
echo "AMD64 server types (for GHCR images):"
echo "  cpx*  - Shared AMD (deprecated in EU, available in US)"
echo "  ccx*  - Dedicated AMD (available everywhere)"
echo "  cx*   - Shared Intel (check availability)"
echo ""
echo "ARM64 server types (need custom images):"
echo "  cax*  - ARM (available everywhere)"
echo ""
