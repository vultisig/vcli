#!/bin/bash
# Wrapper script for vcli
#
# The go-wrappers CGO libraries are automatically downloaded on first run.
# DYLD_LIBRARY_PATH is set automatically by vcli.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Handle both direct execution (from scripts/) and symlink execution (from local/)
if [[ "$SCRIPT_DIR" == */scripts ]]; then
    LOCAL_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
else
    LOCAL_DIR="$SCRIPT_DIR"
fi
VCLI_BIN="$LOCAL_DIR/bin/vcli"

# Auto-load vault password from local/vault.env if it exists
if [ -f "$LOCAL_DIR/vault.env" ]; then
    # shellcheck disable=SC1091
    source "$LOCAL_DIR/vault.env"
fi

exec "$VCLI_BIN" "$@"
