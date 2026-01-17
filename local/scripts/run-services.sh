#!/bin/bash
#
# Run Vultisig services locally (native Go processes)
# Used by: make start build=local
#
# Prerequisites:
# - Docker running with postgres, redis, minio (via docker-compose.yaml)
# - Go installed
# - Sibling repos: ../verifier, ../app-recurring

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCAL_DIR="$(dirname "$SCRIPT_DIR")"
VCLI_DIR="$(dirname "$LOCAL_DIR")"
ROOT_DIR="$(dirname "$VCLI_DIR")"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}Starting services locally...${NC}"
echo ""

# Check Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}ERROR: Go is not installed${NC}"
    echo "Install Go from https://go.dev/dl/"
    exit 1
fi

# Check repos exist
if [ ! -d "$ROOT_DIR/verifier" ]; then
    echo -e "${RED}ERROR: verifier repo not found at $ROOT_DIR/verifier${NC}"
    exit 1
fi

if [ ! -d "$ROOT_DIR/app-recurring" ]; then
    echo -e "${RED}ERROR: app-recurring repo not found at $ROOT_DIR/app-recurring${NC}"
    exit 1
fi

# Create logs directory
LOG_DIR="$VCLI_DIR/logs"
mkdir -p "$LOG_DIR"

# Kill any existing processes
echo -e "${YELLOW}Cleaning up existing processes...${NC}"
pkill -f "go run.*cmd/verifier" 2>/dev/null || true
pkill -f "go run.*cmd/worker" 2>/dev/null || true
pkill -f "go run.*cmd/server" 2>/dev/null || true
pkill -f "go run.*cmd/scheduler" 2>/dev/null || true
pkill -f "go run.*cmd/tx_indexer" 2>/dev/null || true
sleep 1

# ============================================
# VERIFIER SERVICES
# ============================================

echo -e "${CYAN}Starting Verifier API...${NC}"
cd "$ROOT_DIR/verifier"

# Verifier environment
export SERVER_HOST="0.0.0.0"
export SERVER_PORT="8080"
export SERVER_JWT_SECRET="devsecret"
export LOG_FORMAT="text"
export AUTH_ENABLED="true"
export DATABASE_DSN="postgres://vultisig:vultisig@localhost:5432/vultisig-verifier?sslmode=disable"
export REDIS_HOST="localhost"
export REDIS_PORT="6379"
export REDIS_PASSWORD="vultisig"
export BLOCK_STORAGE_HOST="http://localhost:9000"
export BLOCK_STORAGE_REGION="us-east-1"
export BLOCK_STORAGE_ACCESS_KEY="minioadmin"
export BLOCK_STORAGE_SECRET="minioadmin"
export BLOCK_STORAGE_BUCKET="vultisig-verifier"
export ENCRYPTION_SECRET="dev-encryption-secret-32b"
export METRICS_ENABLED="true"
export METRICS_HOST="0.0.0.0"
export METRICS_PORT="8088"

go run ./cmd/verifier > "$LOG_DIR/verifier.log" 2>&1 &
VERIFIER_PID=$!
echo -e "  ${GREEN}✓${NC} Verifier API (PID: $VERIFIER_PID) → localhost:8080"

echo -e "${CYAN}Starting Verifier Worker...${NC}"
export VAULT_SERVICE_RELAY_SERVER="https://api.vultisig.com/router"
export VAULT_SERVICE_LOCAL_PARTY_PREFIX="verifier-dev"
export VAULT_SERVICE_ENCRYPTION_SECRET="dev-encryption-secret-32b"
export VAULT_SERVICE_DO_SETUP_MSG="false"
export METRICS_PORT="8089"

go run ./cmd/worker > "$LOG_DIR/worker.log" 2>&1 &
WORKER_PID=$!
echo -e "  ${GREEN}✓${NC} Verifier Worker (PID: $WORKER_PID)"

# ============================================
# APP-RECURRING (DCA) SERVICES
# ============================================

echo -e "${CYAN}Starting DCA Plugin Server...${NC}"
cd "$ROOT_DIR/app-recurring"

# DCA environment
export MODE="swap"
export SERVER_PORT="8082"
export SERVER_HOST="0.0.0.0"
export SERVER_TASKQUEUENAME="dca_plugin_queue"
export SERVER_ENCRYPTIONSECRET="dev-encryption-secret-32b"
export POSTGRES_DSN="postgres://vultisig:vultisig@localhost:5432/vultisig-dca?sslmode=disable"
export REDIS_URI="redis://:vultisig@localhost:6379"
export BLOCKSTORAGE_HOST="http://localhost:9000"
export BLOCKSTORAGE_REGION="us-east-1"
export BLOCKSTORAGE_ACCESSKEY="minioadmin"
export BLOCKSTORAGE_SECRETKEY="minioadmin"
export BLOCKSTORAGE_BUCKET="vultisig-dca"
export VERIFIER_URL="http://localhost:8080"
export METRICS_ENABLED="true"
export METRICS_HOST="0.0.0.0"
export METRICS_PORT="8181"

go run ./cmd/server > "$LOG_DIR/dca-server.log" 2>&1 &
DCA_SERVER_PID=$!
echo -e "  ${GREEN}✓${NC} DCA Server (PID: $DCA_SERVER_PID) → localhost:8082"

echo -e "${CYAN}Starting DCA Worker...${NC}"
export TASK_QUEUE_NAME="dca_plugin_queue"
export VERIFIER_PARTYPREFIX="verifier"
export VERIFIER_SENDTOKEN="local-dev-dca-apikey"
export VERIFIER_SWAPTOKEN="local-dev-dca-apikey"
export VAULTSERVICE_LOCALPARTYPREFIX="dca-worker"
export VAULTSERVICE_RELAY_SERVER="https://api.vultisig.com/router"
export VAULTSERVICE_ENCRYPTIONSECRET="dev-encryption-secret-32b"
export VAULTSERVICE_DOSETUPMSG="true"
export METRICS_PORT="8183"
# RPC endpoints
export THORCHAIN_URL="https://thornode.ninerealms.com"
export ONEINCH_BASEURL="https://api.1inch.dev/swap/v6.0"
export MAYACHAIN_URL="https://mayanode.mayachain.info"
export RPC_ETHEREUM_URL="https://ethereum-rpc.publicnode.com"
export RPC_ARBITRUM_URL="https://arbitrum-one-rpc.publicnode.com"
export RPC_BASE_URL="https://base-rpc.publicnode.com"
export RPC_POLYGON_URL="https://polygon-bor-rpc.publicnode.com"
export RPC_BSC_URL="https://bsc-rpc.publicnode.com"
export RPC_AVALANCHE_URL="https://avalanche-c-chain-rpc.publicnode.com"
export RPC_OPTIMISM_URL="https://optimism-rpc.publicnode.com"
export RPC_BLAST_URL="https://blast-rpc.publicnode.com"
export RPC_SOLANA_URL="https://api.mainnet-beta.solana.com"
export RPC_BITCOIN_URL="https://mempool.space/api"
export BTC_BLOCKCHAIRURL="https://api.vultisig.com/blockchair"
export LTC_BLOCKCHAIRURL="https://api.vultisig.com/blockchair"
export DOGE_BLOCKCHAIRURL="https://api.vultisig.com/blockchair"
export SOLANA_JUPITERAPIURL="https://quote-api.jup.ag/v6"

go run ./cmd/worker > "$LOG_DIR/dca-worker.log" 2>&1 &
DCA_WORKER_PID=$!
echo -e "  ${GREEN}✓${NC} DCA Worker (PID: $DCA_WORKER_PID)"

echo -e "${CYAN}Starting DCA Scheduler...${NC}"
export HEALTHPORT="8184"
export METRICS_PORT="8185"

go run ./cmd/scheduler > "$LOG_DIR/dca-scheduler.log" 2>&1 &
DCA_SCHEDULER_PID=$!
echo -e "  ${GREEN}✓${NC} DCA Scheduler (PID: $DCA_SCHEDULER_PID)"

echo -e "${CYAN}Starting DCA TX Indexer...${NC}"
export HEALTHPORT="8186"
export METRICS_PORT="8187"
export BASE_DATABASE_DSN="postgres://vultisig:vultisig@localhost:5432/vultisig-dca?sslmode=disable"
export BASE_INTERVAL="10s"
export BASE_ITERATIONTIMEOUT="30s"
export BASE_MARKLOSTAFTER="1h"
export BASE_CONCURRENCY="5"
export BASE_RPC_ETHEREUM_URL="https://ethereum-rpc.publicnode.com"
export BASE_RPC_ARBITRUM_URL="https://arbitrum-one-rpc.publicnode.com"
export BASE_RPC_AVALANCHE_URL="https://avalanche-c-chain-rpc.publicnode.com"
export BASE_RPC_BSC_URL="https://bsc-rpc.publicnode.com"
export BASE_RPC_BASE_URL="https://base-rpc.publicnode.com"
export BASE_RPC_OPTIMISM_URL="https://optimism-rpc.publicnode.com"
export BASE_RPC_BLAST_URL="https://blast-rpc.publicnode.com"
export BASE_RPC_POLYGON_URL="https://polygon-bor-rpc.publicnode.com"
export BASE_RPC_BITCOIN_URL="https://bitcoin-rpc.publicnode.com"
export BASE_RPC_SOLANA_URL="https://solana-rpc.publicnode.com"

go run ./cmd/tx_indexer > "$LOG_DIR/dca-tx-indexer.log" 2>&1 &
DCA_TX_INDEXER_PID=$!
echo -e "  ${GREEN}✓${NC} DCA TX Indexer (PID: $DCA_TX_INDEXER_PID)"

# ============================================
# SUMMARY
# ============================================

echo ""
echo -e "${CYAN}=========================================${NC}"
echo -e "${GREEN}  STARTUP COMPLETE${NC}"
echo -e "${CYAN}=========================================${NC}"
echo ""
echo -e "  ${CYAN}Services (native Go processes):${NC}"
echo -e "    Verifier API         localhost:8080"
echo -e "    Verifier Worker      (background)"
echo -e "    DCA Plugin API       localhost:8082"
echo -e "    DCA Plugin Worker    (background)"
echo -e "    DCA Scheduler        (background)"
echo -e "    DCA TX Indexer       (background)"
echo ""
echo -e "  ${CYAN}Infrastructure (Docker):${NC}"
echo -e "    PostgreSQL           localhost:5432"
echo -e "    Redis                localhost:6379"
echo -e "    MinIO                localhost:9000 (console: 9090)"
echo ""
echo -e "  ${CYAN}Logs:${NC}"
echo -e "    tail -f $LOG_DIR/verifier.log"
echo -e "    tail -f $LOG_DIR/dca-server.log"
echo -e "    (or any file in $LOG_DIR/)"
echo ""
echo -e "  ${CYAN}Stop:${NC} make stop"
echo ""
echo -e "${GREEN}Edit code in ../verifier or ../app-recurring, then restart with 'make start'${NC}"
echo ""

# Save PIDs for later cleanup
echo "$VERIFIER_PID" > "$LOG_DIR/verifier.pid"
echo "$WORKER_PID" > "$LOG_DIR/worker.pid"
echo "$DCA_SERVER_PID" > "$LOG_DIR/dca-server.pid"
echo "$DCA_WORKER_PID" > "$LOG_DIR/dca-worker.pid"
echo "$DCA_SCHEDULER_PID" > "$LOG_DIR/dca-scheduler.pid"
echo "$DCA_TX_INDEXER_PID" > "$LOG_DIR/dca-tx-indexer.pid"
