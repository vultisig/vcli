#!/bin/bash
# Wrapper script for vcli that:
# 1. Auto-sets DYLD_LIBRARY_PATH from cluster.yaml (for macOS DKLS library)
# 2. Auto-loads vault.env for VAULT_PASSWORD and other env vars
#
# Why DYLD_LIBRARY_PATH? The DKLS cryptographic library path is baked into
# the Go binary at compile time. macOS requires DYLD_LIBRARY_PATH to override
# the embedded rpath.
#
# Why vault.env? Allows you to run commands without passing -p every time:
#   ./vcli.sh plugin install dca   # Uses VAULT_PASSWORD from vault.env

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Handle both direct execution (from scripts/) and symlink execution (from local/)
if [[ "$SCRIPT_DIR" == */scripts ]]; then
    LOCAL_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
else
    LOCAL_DIR="$SCRIPT_DIR"
fi
VCLI_BIN="$LOCAL_DIR/vcli"

# Auto-load vault.env if it exists (for VAULT_PASSWORD, VAULT_PATH, etc.)
# Only load if VAULT_PASSWORD not already set (allows override)
if [ -z "$VAULT_PASSWORD" ]; then
    for envfile in "$LOCAL_DIR/vault.env" "$HOME/.vultisig/vault.env"; do
        if [ -f "$envfile" ]; then
            set -a
            source "$envfile"
            set +a
            break
        fi
    done
fi

# Set DYLD_LIBRARY_PATH if not already set
if [ -z "$DYLD_LIBRARY_PATH" ]; then
    # Try to find cluster.yaml and extract dyld_path
    CLUSTER_YAML=""
    for path in "$LOCAL_DIR/cluster.yaml" "$HOME/.vultisig/cluster.yaml"; do
        if [ -f "$path" ]; then
            CLUSTER_YAML="$path"
            break
        fi
    done

    if [ -n "$CLUSTER_YAML" ]; then
        # Extract dyld_path from cluster.yaml (simple grep approach)
        DYLD_PATH=$(grep 'dyld_path:' "$CLUSTER_YAML" | cut -d: -f2 | tr -d ' ' | sed "s|~|$HOME|g")
        if [ -n "$DYLD_PATH" ]; then
            export DYLD_LIBRARY_PATH="$DYLD_PATH"
        else
            echo "Warning: dyld_path not found in cluster.yaml" >&2
        fi
    else
        echo "Warning: cluster.yaml not found, DYLD_LIBRARY_PATH may not be set correctly" >&2
    fi
fi

exec "$VCLI_BIN" "$@"
