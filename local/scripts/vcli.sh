#!/bin/bash
# Wrapper script for vcli that auto-sets DYLD_LIBRARY_PATH from cluster.yaml
#
# Why DYLD_LIBRARY_PATH? The DKLS cryptographic library path is baked into
# the Go binary at compile time. macOS requires DYLD_LIBRARY_PATH to override
# the embedded rpath.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Handle both direct execution (from scripts/) and symlink execution (from local/)
if [[ "$SCRIPT_DIR" == */scripts ]]; then
    LOCAL_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
else
    LOCAL_DIR="$SCRIPT_DIR"
fi
VCLI_BIN="$LOCAL_DIR/vcli"

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
