#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source environment if exists
if [ -f "$ROOT_DIR/setup-env.sh" ]; then
    source "$ROOT_DIR/setup-env.sh"
else
    echo "Error: Run 'terraform apply' first to generate setup-env.sh"
    exit 1
fi

# Source .env.k8s for HCLOUD_TOKEN and other secrets
if [ -f "$ROOT_DIR/.env.k8s" ]; then
    set -a
    source "$ROOT_DIR/.env.k8s"
    set +a
fi

SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
if [ -n "${SSH_KEY:-}" ]; then
    SSH_OPTS="-i $SSH_KEY $SSH_OPTS"
fi

echo "=== Vultisig k3s Cluster Setup ==="
echo ""

# Step 1: Install master
echo "[1/4] Installing k3s master on $MASTER_IP..."
ssh $SSH_OPTS root@"$MASTER_IP" "
    export K3S_TOKEN='$K3S_TOKEN'
    export MASTER_IP='$MASTER_IP'
    bash -s
" < "$SCRIPT_DIR/k3s-install-master.sh"

# Step 2: Get kubeconfig
echo ""
echo "[2/4] Fetching kubeconfig..."
mkdir -p "$ROOT_DIR/.kube"
scp $SSH_OPTS root@"$MASTER_IP":/etc/rancher/k3s/k3s.yaml "$ROOT_DIR/.kube/config"
sed -i.bak "s/127.0.0.1/$MASTER_IP/g" "$ROOT_DIR/.kube/config"
rm -f "$ROOT_DIR/.kube/config.bak"

export KUBECONFIG="$ROOT_DIR/.kube/config"

# Step 3: Install workers (PARALLEL - 3x faster)
echo ""
echo "[3/4] Installing worker nodes in parallel..."

pids=""
for region in fsn1 nbg1 hel1; do
    case $region in
        fsn1) WORKER_IP="$WORKER_FSN1_IP" ;;
        nbg1) WORKER_IP="$WORKER_NBG1_IP" ;;
        hel1) WORKER_IP="$WORKER_HEL1_IP" ;;
    esac

    if [ -n "$WORKER_IP" ]; then
        echo "  Spawning worker install in $region ($WORKER_IP)..."
        ssh $SSH_OPTS root@"$WORKER_IP" "
            export K3S_TOKEN='$K3S_TOKEN'
            export MASTER_URL='$MASTER_PRIVATE_IP'
            export REGION='$region'
            bash -s
        " < "$SCRIPT_DIR/k3s-install-worker.sh" &
        pids="$pids $!"
    fi
done

# Wait for all workers to complete
echo "  Waiting for all workers to join cluster..."
for pid in $pids; do
    if wait $pid; then
        echo "  ✓ Worker joined cluster"
    else
        echo "  ✗ Worker failed to join"
    fi
done

# Step 4: Verify cluster
echo ""
echo "[4/4] Verifying cluster..."
sleep 10
kubectl get nodes -o wide

# Install Hetzner CSI driver for volumes
echo ""
echo "Installing Hetzner CSI driver..."

# Create hcloud secret with API token (required by CSI driver)
if [ -n "${HCLOUD_TOKEN:-}" ]; then
    kubectl create secret generic hcloud \
        --namespace kube-system \
        --from-literal=token="$HCLOUD_TOKEN" \
        --dry-run=client -o yaml | kubectl apply -f -
    echo "  ✓ Created hcloud secret with API token"
else
    echo "  ⚠ WARNING: HCLOUD_TOKEN not set - CSI driver will fail"
    echo "    Set HCLOUD_TOKEN in .env.k8s and run:"
    echo "    kubectl create secret generic hcloud -n kube-system --from-literal=token=\$HCLOUD_TOKEN"
fi

kubectl apply -f https://raw.githubusercontent.com/hetznercloud/csi-driver/main/deploy/kubernetes/hcloud-csi.yml

echo ""
echo "=== Cluster setup complete ==="
echo ""
echo "KUBECONFIG=$ROOT_DIR/.kube/config"
echo ""
echo "To use: export KUBECONFIG=$ROOT_DIR/.kube/config"
