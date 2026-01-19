# Setup Issues Log

Issues encountered during vcli local development setup by Claude.

---

## 1. Missing Build Step in Documentation

**What happened:** After `make start` completed successfully, running `./local/vcli.sh vault import` failed with:
```
./local/vcli.sh: line 23: /Users/eng/eng/vultisig/vcli/local/vcli: No such file or directory
```

**Root cause:** The README doesn't mention that the `vcli` binary needs to be built before use. `make start` only starts infrastructure and services - it doesn't build the CLI tool.

**Fix needed:** Either:
- Add `make build` or `go build` step to README after `make start`
- Or have `make start` automatically build vcli
- Or have `vcli.sh` auto-build if binary is missing

---

## 2. Build Command Confusion

**What happened:** When I tried to build, I made multiple errors:

1. First attempt from repo root:
   ```bash
   go build -o local/vcli ./local/cmd/vcli
   # Error: go: cannot find main module
   ```

2. Had to discover that `go.mod` is inside `local/` directory

3. Then shell state got confused after `cd local`:
   ```bash
   cd local && go build -o vcli ./cmd/vcli
   # Error: (eval):cd:1: no such file or directory: local
   ```

**Root cause:**
- The Go module is in `local/`, not the repo root
- Shell working directory got reset during git clone operations
- README doesn't document how to build vcli

**Fix needed:** Add clear build instructions:
```bash
cd local && go build -o vcli ./cmd/vcli
```

---

## 3. Import Command Used Relative Path

**What happened:** First import attempt failed:
```bash
./local/vcli.sh vault import --file local/keyshares/FastPlugin1-a06a-share2of2.vult --password "Password123"
# Error: open local/keyshares/FastPlugin1-a06a-share2of2.vult: no such file or directory
```

Had to use absolute path instead:
```bash
./local/vcli.sh vault import --file /Users/eng/eng/vultisig/vcli/local/keyshares/FastPlugin1-a06a-share2of2.vult --password "Password123"
```

**Root cause:** The `vcli.sh` wrapper script changes directory before running vcli, so relative paths from the user's shell don't work.

**Fix needed:** Either:
- Document that absolute paths are required
- Fix `vcli.sh` to resolve relative paths before cd'ing
- Or use the default behavior (auto-find in `local/keyshares/`) which the README suggests but I didn't try first

---

## 4. Incomplete Repo Clone (Permission Error Recovery)

**What happened:** Initial `git clone` for verifier was interrupted by `/private/tmp/claude` permission error. This left verifier directory with only `.git/` folder (no source code).

Later check showed:
```bash
ls -la /Users/eng/eng/vultisig/verifier/
# Only .git directory, no source files
```

Had to `rm -rf` and re-clone.

**Root cause:** Interrupted git clone leaves partial state.

**Not a documentation issue** - this was a Claude Code environment problem.

---

## 5. Docker State Confusion

**What happened:** User said "kill * pid 87539: Docker Desktop" and I misinterpreted this as confirmation Docker was running. It was actually telling me Docker was killed.

**Not a documentation issue** - this was my interpretation error.

---

## 6. go-wrappers Library Auto-Download

**What happened:** First vcli command triggered automatic download of go-wrappers libraries:
```
Downloading go-wrappers libraries...
  Source: https://github.com/vultisig/go-wrappers/archive/refs/heads/master.tar.gz
  Target: /Users/eng/.vultisig/lib/darwin
```

This is actually good (auto-download works), but it happens on first run which might be unexpected.

**Not an issue** - just noting this behavior exists and works.

---

## 7. Authentication Warning (Expected)

**What happened:** Import showed warning:
```
Warning: Authentication failed: authentication failed (500): {"error":{"message":"failed to generate auth token"}}
```

But import still succeeded. The README mentions this can be done later with `vcli auth login`.

**Not an issue** - expected behavior, just noting it appears scary but is fine.

---

## Summary of Documentation Gaps

| Issue | Severity | Fix |
|-------|----------|-----|
| No build step documented | **High** | Add `cd local && go build -o vcli ./cmd/vcli` to Quick Start |
| Relative paths don't work with vcli.sh | Medium | Document or fix in vcli.sh |
| go.mod location unclear | Low | Note that module is in `local/` not repo root |

---

## Recommended README Changes

### After "Quick Start" section, add:

```markdown
## Build vcli (First Time)

Before using vcli commands, build the CLI tool:

\`\`\`bash
cd local && go build -o vcli ./cmd/vcli
\`\`\`

This only needs to be done once (or after code changes to vcli).
```

### In "Step 2: IMPORT" section, clarify:

```markdown
# Recommended: let vcli auto-find vault in local/keyshares/
./local/vcli.sh vault import --password "password"

# Or use ABSOLUTE path if specifying file:
./local/vcli.sh vault import --file /full/path/to/vault.vult --password "password"
```

---

## 8. go-wrappers Library Path Not Set for Services (CRITICAL)

**What happened:** After `make start`, the verifier service crashed with:
```
dyld[61639]: Library not loaded: /Users/johnny/project/wallet/dkls23-rs/target/release/deps/libgodkls.dylib
  Referenced from: .../verifier
  Reason: tried: '/Users/johnny/project/wallet/dkls23-rs/...' (no such file)
signal: abort trap
```

**Root cause:** The `run-services.sh` script didn't set `DYLD_LIBRARY_PATH` to include the go-wrappers libraries that vcli auto-downloads to `~/.vultisig/lib/darwin/`.

**Fix applied:** Added to `run-services.sh`:
```bash
LIB_DIR="$HOME/.vultisig/lib"
if [[ "$(uname)" == "Darwin" ]]; then
    LIB_DIR="$LIB_DIR/darwin"
    export DYLD_LIBRARY_PATH="$LIB_DIR:$DYLD_LIBRARY_PATH"
else
    LIB_DIR="$LIB_DIR/linux"
    export LD_LIBRARY_PATH="$LIB_DIR:$LD_LIBRARY_PATH"
fi
```

---

## 9. Plugin Install Requires Authentication First

**What happened:** After importing vault, running `plugin install` failed:
```
Error: authentication required: not authenticated. Run 'vcli auth login' first
```

**Root cause:** The vault import showed an authentication warning but continued. The README implies import is sufficient, but plugin operations require explicit authentication.

**Documentation gap:** README Step 3 (INSTALL) doesn't mention that authentication may be required between IMPORT and INSTALL if the auth warning appeared during import.

---

## 10. Authentication Fails with 404 from Fast Vault Server

**What happened:** Running `vcli auth login --password "Password123"` failed:
```
Error: TSS keysign failed: request fast vault keysign: fast vault server returned 404
```

**Root cause:** Unknown - may be a server-side issue or vault configuration problem.

**Status:** Blocked investigation.

---

## 11. Database Migration Error on Restart

**What happened:** After stopping and restarting, the verifier failed with:
```
ERROR: duplicate key value violates unique constraint "pg_type_typname_nsp_index"
```

**Root cause:** Partial state from previous run. The types already exist in the database.

**Note:** `make stop` DOES clean up Docker volumes, so this may have been a race condition where services started before Docker was fully cleaned.

---

## 12. Port Conflicts from Lingering Processes (CRITICAL)

**What happened:** Verifier consistently failed with:
```
panic: listen tcp :8080: bind: address already in use
```

**Root cause:** Multiple issues:
1. `pkill -f "go run.*cmd/verifier"` pattern doesn't match compiled binaries
2. `go run` compiles to `/Library/Caches/go-build/.../<binary>` and runs that
3. Previous processes weren't being killed properly

**Fix applied:** Updated both `run-services.sh` and `Makefile` to also kill compiled binaries:
```bash
pkill -9 -f "go-build.*/verifier$" 2>/dev/null || true
pkill -9 -f "go-build.*/worker$" 2>/dev/null || true
# etc.
```

---

## 13. Shared Machine Problem - Cannot Kill Other User's Processes

**What happened:** User `dev` has a verifier process (PID 31249) running from Saturday that holds port 8080. User `eng` cannot kill it without sudo.

**Root cause:** Shared development machine with multiple users running the same services.

**Impact:** BLOCKER - cannot proceed until either:
1. User `dev` kills their processes
2. Services are configured to use different ports
3. Admin kills the process

**Recommendation:** Add port conflict detection in `run-services.sh` that fails fast with a helpful message instead of starting services that will crash.

---

## Updated Summary of Issues

| Issue | Severity | Status |
|-------|----------|--------|
| No build step documented | **High** | Documentation fix needed |
| go-wrappers DYLD_LIBRARY_PATH not set | **Critical** | Fixed in run-services.sh |
| pkill patterns don't catch compiled binaries | **High** | Fixed in run-services.sh and Makefile |
| Authentication required after import | Medium | Documentation unclear |
| Relative paths don't work with vcli.sh | Medium | Document or fix |
| Shared machine port conflicts | **Blocker** | Need port conflict detection |

---

## Code Changes Made During This Session

### 1. local/scripts/run-services.sh

Added library path setup:
```bash
# Setup go-wrappers library path
LIB_DIR="$HOME/.vultisig/lib"
if [[ "$(uname)" == "Darwin" ]]; then
    LIB_DIR="$LIB_DIR/darwin"
    export DYLD_LIBRARY_PATH="$LIB_DIR:$DYLD_LIBRARY_PATH"
else
    LIB_DIR="$LIB_DIR/linux"
    export LD_LIBRARY_PATH="$LIB_DIR:$LD_LIBRARY_PATH"
fi
```

Added compiled binary cleanup:
```bash
pkill -9 -f "go-build.*/verifier$" 2>/dev/null || true
pkill -9 -f "go-build.*/worker$" 2>/dev/null || true
pkill -9 -f "go-build.*/server$" 2>/dev/null || true
pkill -9 -f "go-build.*/scheduler$" 2>/dev/null || true
pkill -9 -f "go-build.*/tx_indexer$" 2>/dev/null || true
```

### 2. Makefile

Added same compiled binary cleanup to `stop` target.

---

## 14. Plugins Not Seeded in Database

**What happened:** After `make start`, running `plugin install` failed:
```
Error: plugin not found: {"error":{"message":"an internal error occurred"}}
```

**Root cause:** The `seed-plugins.sql` file exists but is never run automatically. The README doesn't mention needing to seed plugins.

**Fix applied:** Ran manually:
```bash
docker exec -i vultisig-postgres psql -U vultisig -d vultisig-verifier < local/seed-plugins.sql
```

**Additional issue:** The seed file uses Docker hostnames (`http://vultisig-dca:8082`) but local dev uses `localhost`. Had to also run:
```bash
docker exec -i vultisig-postgres psql -U vultisig -d vultisig-verifier -c \
  "UPDATE plugins SET server_endpoint = 'http://localhost:8082' WHERE id = 'vultisig-dca-0000';"
```

**Recommendation:** Either:
1. Add seeding to `make start` automatically
2. Create a `local/seed-plugins-local.sql` with localhost endpoints
3. Document the seeding step in README

---

## 15. DCA Worker Crashes - Missing RPC Configurations (CRITICAL)

**What happened:** DCA worker crashed multiple times with missing RPC errors:
```
FATA[0000] failed to create rpc client: Zksync dial unix: missing address
FATA[0000] failed to create rpc client: CronosChain dial unix: missing address
```

**Root cause:** The `run-services.sh` script was missing multiple RPC URL environment variables.

**Fix applied:** Added to run-services.sh:
```bash
export RPC_ZKSYNC_URL="https://mainnet.era.zksync.io"
export RPC_CRONOS_URL="https://evm.cronos.org"
```

**Impact:** Policy execution cannot proceed without the DCA worker running.

**Note:** There may be more missing RPCs for other chains. The app-recurring worker requires ALL chain RPCs to be configured, even if you're only using Ethereum.

---

## 16. Policy Output Path - Cannot Write to /tmp

**What happened:** Running `policy generate --output /tmp/my-policy.json` failed:
```
Error: write file: open /tmp/my-policy.json: permission denied
```

**Root cause:** Sandbox environment blocks writes outside the project directory.

**Fix:** Use a path within the project:
```bash
./local/vcli.sh policy generate --from eth --to usdc --amount 0.01 --output local/policies/my-policy.json
```

**Documentation gap:** README Step 4 (GENERATE) shows `--output my-policy.json` without specifying where. Should clarify to use `local/policies/` directory or current working directory.

---

## Final Status: E2E Test Results

**Infrastructure:** Working after fixes applied.

**E2E Flow Test:**
1. ✅ `make start` - services started (after library path and process cleanup fixes)
2. ✅ `vault import` - vault imported and authenticated
3. ✅ `plugin install` - 4-party TSS reshare completed (after seeding plugins)
4. ✅ `policy generate` - policy JSON created
5. ✅ `policy add` - policy submitted and signed
6. ✅ Scheduler picks up policy
7. ✅ Worker fetches keyshare from MinIO
8. ✅ Worker generates swap route via 1inch
9. ❌ Policy rule validation failed (address mismatch - business logic issue, not infra)

**Conclusion:** Local development environment works. The final failure is at the policy rule level (expected swap router address doesn't match actual), which is a configuration/business logic issue, not an infrastructure problem.

---

## Summary: All Issues Found

| # | Issue | Severity | Fixed? |
|---|-------|----------|--------|
| 1 | No build step documented | High | No - needs README update |
| 2 | Build from wrong directory | Low | No - needs README update |
| 3 | Relative paths don't work | Medium | No - needs vcli.sh fix or docs |
| 8 | DYLD_LIBRARY_PATH not set | Critical | Yes - run-services.sh |
| 9 | Auth required after import | Medium | No - needs docs clarification |
| 12 | pkill patterns miss compiled binaries | High | Yes - run-services.sh + Makefile |
| 13 | Shared machine port conflicts | Blocker | No - needs port conflict detection |
| 14 | Plugins not seeded | High | No - needs make start to seed |
| 15 | Missing RPC configs (Zksync, Cronos) | Critical | Yes - run-services.sh |
| 16 | /tmp write blocked | Low | No - needs docs |

**Total issues found: 16**
**Critical issues fixed: 3**
**Documentation issues remaining: 6**
