# Swap Checks Implementation Plan

This document outlines the plan to extend vcli vault details and test bidirectional swaps across THORChain (TC) and MayaChain (MP).

---

## Status Tracking

| Part | Task | Status |
|------|------|--------|
| 1.1 | Add DASH, ZEC to UTXO chains | DONE |
| 1.2 | Add XRP balance fetch | DONE |
| 1.3 | Add TRON balance fetch | DONE |
| 1.4 | Add TRON USDT (TRC20) | DONE |
| 1.5 | Extend ERC20 to all EVM chains | DONE |
| 1.6 | Add missing asset aliases | DONE |
| 2.1 | Phase 1: ETH → Tokens (15 swaps) | DONE |
| 2.2 | Phase 2: ETH → Gas (18 swaps) | DONE |
| 2.3 | Phase 3: Tokens → ETH (15 swaps) | DONE |
| 2.4 | Phase 4: Gas → ETH (18 swaps) | DONE |

---

## Unified Asset List (31 Assets)

| # | Chain | Gas Asset | Token 1 | Token 2 | Token 3 | Protocol |
|---|-------|-----------|---------|---------|---------|----------|
| 1 | Ethereum | ETH | USDC | USDT | DAI | TC + MP |
| 2 | Bitcoin | BTC | - | - | - | TC + MP |
| 3 | Avalanche | AVAX | USDC | USDT | - | TC |
| 4 | BSC | BNB | USDC | USDT | BTCB | TC |
| 5 | Base | ETH | USDC | - | - | TC |
| 6 | Arbitrum | ETH | USDC | USDT | WBTC | MP |
| 7 | Litecoin | LTC | - | - | - | TC |
| 8 | Bitcoin Cash | BCH | - | - | - | TC |
| 9 | Dogecoin | DOGE | - | - | - | TC |
| 10 | Cosmos | ATOM | - | - | - | TC |
| 11 | THORChain | RUNE | - | - | - | TC + MP |
| 12 | MayaChain | CACAO | - | - | - | MP |
| 13 | TRON | TRX | USDT | - | - | TC |
| 14 | Ripple | XRP | - | - | - | TC |
| 15 | Dash | DASH | - | - | - | MP |
| 16 | Zcash | ZEC | - | - | - | MP |
| 17 | Kujira | KUJI | - | - | - | MP |

**Totals:** 17 gas assets + 14 non-gas tokens = 31 assets

---

## Part 1: vcli Vault Details Update

**File:** `local/cmd/vcli/cmd/vault.go`

### 1.1 Add Missing Chain Support

| Task | Chain | Implementation | API |
|------|-------|----------------|-----|
| 1.1.1 | DASH | Add to UTXO chains array | Blockchair |
| 1.1.2 | ZEC | Add to UTXO chains array | Blockchair |
| 1.1.3 | XRP | New balance fetch function | Ripple RPC |
| 1.1.4 | TRON | New balance fetch function | Tronscan API |

### 1.2 Add Missing Token Support

| Task | Chain | Tokens | Implementation |
|------|-------|--------|----------------|
| 1.2.1 | TRON | USDT | TRC20 balance via Tronscan |
| 1.2.2 | Avalanche | USDC, USDT | Extend ERC20 query |
| 1.2.3 | BSC | USDC, USDT, BTCB | Extend ERC20 query |
| 1.2.4 | Base | USDC | Extend ERC20 query |
| 1.2.5 | Arbitrum | USDC, USDT, WBTC | Extend ERC20 query |

### 1.3 Token Contract Addresses

```go
// Ethereum (already exists, verify)
ethereumTokens := []TokenInfo{
    {Symbol: "USDC", Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6},
    {Symbol: "USDT", Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6},
    {Symbol: "DAI",  Address: "0x6B175474E89094C44Da98b954EesD5C4BB76F7Ed", Decimals: 18},
}

// Avalanche
avalancheTokens := []TokenInfo{
    {Symbol: "USDC", Address: "0xB97EF9Ef8734C71904D8002F8B6Bc66Dd9c48a6E", Decimals: 6},
    {Symbol: "USDT", Address: "0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7", Decimals: 6},
}

// BSC
bscTokens := []TokenInfo{
    {Symbol: "USDC", Address: "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d", Decimals: 18},
    {Symbol: "USDT", Address: "0x55d398326f99059fF775485246999027B3197955", Decimals: 18},
    {Symbol: "BTCB", Address: "0x7130d2A12B9BCbFAe4f2634d864A1Ee1Ce3Ead9c", Decimals: 18},
}

// Base
baseTokens := []TokenInfo{
    {Symbol: "USDC", Address: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", Decimals: 6},
}

// Arbitrum
arbitrumTokens := []TokenInfo{
    {Symbol: "USDC", Address: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", Decimals: 6},
    {Symbol: "USDT", Address: "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9", Decimals: 6},
    {Symbol: "WBTC", Address: "0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f", Decimals: 8},
}

// TRON
tronTokens := []TokenInfo{
    {Symbol: "USDT", Address: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", Decimals: 6},
}
```

### 1.4 Missing Asset Aliases (config.go)

These aliases need to be added to `local/cmd/vcli/cmd/config.go` for swap commands:

```go
// Avalanche tokens
"usdc:avalanche" → Chain: Avalanche, Address: 0xB97EF9Ef8734C71904D8002F8B6Bc66Dd9c48a6E
"usdt:avalanche" → Chain: Avalanche, Address: 0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7

// BSC tokens
"usdc:bsc"       → Chain: BSC, Address: 0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d
"usdt:bsc"       → Chain: BSC, Address: 0x55d398326f99059fF775485246999027B3197955
"btcb"           → Chain: BSC, Address: 0x7130d2A12B9BCbFAe4f2634d864A1Ee1Ce3Ead9c

// Base tokens
"usdc:base"      → Chain: Base, Address: 0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913

// Gas assets
"base"           → Chain: Base, native ETH
"cacao"          → Chain: MayaChain, native
"dash"           → Chain: Dash, native
"zec"            → Chain: Zcash, native
"kuji"           → Chain: Kujira, native
```

### 1.5 Implementation Order

1. Add DASH, ZEC to existing UTXO array (easiest)
2. Extend ERC20 queries to all EVM chains
3. Add TRON native + TRC20 support
4. Add XRP support
5. Add missing asset aliases to config.go

---

## Part 2: Swap Route Test Plan

### 2.1 Dual-Route Assets (Test Both TC and MP)

| Asset | TC | MP | Notes |
|-------|----|----|-------|
| BTC | ✓ | ✓ | Test both routes |
| ETH.USDC | ✓ | ✓ | Test both routes |
| ETH.USDT | ✓ | ✓ | Test both routes |
| RUNE | ✓ | ✓ | TC native, MP has pool |

### 2.2 Swap Execution Order

Swaps must be executed in this order to preserve gas:

```
Phase 1: ETH → Tokens     (get tokens while we have ETH)
Phase 2: ETH → Gas Assets (get gas on other chains)
Phase 3: Tokens → ETH     (return tokens, uses chain gas)
Phase 4: Gas Assets → ETH (return gas assets last)
```

---

### Phase 1: ETH → Tokens (15 swaps)

| # | From | To | Route | Amount | Command |
|---|------|-----|-------|--------|---------|
| 1 | ETH | USDC | TC | 0.01 | `--from eth --to usdc` |
| 2 | ETH | USDC | MP | 0.01 | `--from eth --to usdc --route mayachain` |
| 3 | ETH | USDT | TC | 0.01 | `--from eth --to usdt` |
| 4 | ETH | USDT | MP | 0.01 | `--from eth --to usdt --route mayachain` |
| 5 | ETH | DAI | TC | 0.01 | `--from eth --to dai` |
| 6 | ETH | AVAX.USDC | TC | 0.01 | `--from eth --to usdc:avalanche` |
| 7 | ETH | AVAX.USDT | TC | 0.01 | `--from eth --to usdt:avalanche` |
| 8 | ETH | BSC.USDC | TC | 0.01 | `--from eth --to usdc:bsc` |
| 9 | ETH | BSC.USDT | TC | 0.01 | `--from eth --to usdt:bsc` |
| 10 | ETH | BSC.BTCB | TC | 0.01 | `--from eth --to btcb` |
| 11 | ETH | BASE.USDC | TC | 0.01 | `--from eth --to usdc:base` |
| 12 | ETH | ARB.USDC | MP | 0.01 | `--from eth --to arb-usdc --route mayachain` |
| 13 | ETH | ARB.USDT | MP | 0.01 | `--from eth --to arb-usdt --route mayachain` |
| 14 | ETH | ARB.WBTC | MP | 0.01 | `--from eth --to arb-wbtc --route mayachain` |
| 15 | ETH | TRON.USDT | TC | 0.01 | `--from eth --to usdt:tron` |

---

### Phase 2: ETH → Gas Assets (18 swaps)

| # | From | To | Route | Amount | Command |
|---|------|-----|-------|--------|---------|
| 16 | ETH | BTC | TC | 0.01 | `--from eth --to btc` |
| 17 | ETH | BTC | MP | 0.01 | `--from eth --to btc --route mayachain` |
| 18 | ETH | AVAX | TC | 0.01 | `--from eth --to avax` |
| 19 | ETH | BNB | TC | 0.01 | `--from eth --to bnb` |
| 20 | ETH | BASE.ETH | TC | 0.01 | `--from eth --to base` |
| 21 | ETH | ARB.ETH | MP | 0.01 | `--from eth --to arb-eth --route mayachain` |
| 22 | ETH | LTC | TC | 0.01 | `--from eth --to ltc` |
| 23 | ETH | BCH | TC | 0.01 | `--from eth --to bch` |
| 24 | ETH | DOGE | TC | 0.01 | `--from eth --to doge` |
| 25 | ETH | ATOM | TC | 0.01 | `--from eth --to atom` |
| 26 | ETH | RUNE | TC | 0.01 | `--from eth --to rune` |
| 27 | ETH | RUNE | MP | 0.01 | `--from eth --to rune --route mayachain` |
| 28 | ETH | CACAO | MP | 0.01 | `--from eth --to cacao --route mayachain` |
| 29 | ETH | TRX | TC | 0.01 | `--from eth --to trx` |
| 30 | ETH | XRP | TC | 0.01 | `--from eth --to xrp` |
| 31 | ETH | DASH | MP | 0.01 | `--from eth --to dash --route mayachain` |
| 32 | ETH | ZEC | MP | 0.01 | `--from eth --to zec --route mayachain` |
| 33 | ETH | KUJI | MP | 0.01 | `--from eth --to kuji --route mayachain` |

---

### Phase 3: Tokens → ETH (15 swaps)

| # | From | To | Route | Amount | Command |
|---|------|-----|-------|--------|---------|
| 34 | USDC | ETH | TC | 30 | `--from usdc --to eth` |
| 35 | USDC | ETH | MP | 30 | `--from usdc --to eth --route mayachain` |
| 36 | USDT | ETH | TC | 30 | `--from usdt --to eth` |
| 37 | USDT | ETH | MP | 30 | `--from usdt --to eth --route mayachain` |
| 38 | DAI | ETH | TC | 30 | `--from dai --to eth` |
| 39 | AVAX.USDC | ETH | TC | 30 | `--from usdc:avalanche --to eth` |
| 40 | AVAX.USDT | ETH | TC | 30 | `--from usdt:avalanche --to eth` |
| 41 | BSC.USDC | ETH | TC | 30 | `--from usdc:bsc --to eth` |
| 42 | BSC.USDT | ETH | TC | 30 | `--from usdt:bsc --to eth` |
| 43 | BSC.BTCB | ETH | TC | 0.0003 | `--from btcb --to eth` |
| 44 | BASE.USDC | ETH | TC | 30 | `--from usdc:base --to eth` |
| 45 | ARB.USDC | ETH | MP | 30 | `--from arb-usdc --to eth --route mayachain` |
| 46 | ARB.USDT | ETH | MP | 30 | `--from arb-usdt --to eth --route mayachain` |
| 47 | ARB.WBTC | ETH | MP | 0.0003 | `--from arb-wbtc --to eth --route mayachain` |
| 48 | TRON.USDT | ETH | TC | 30 | `--from usdt:tron --to eth` |

---

### Phase 4: Gas Assets → ETH (18 swaps)

| # | From | To | Route | Amount | Command |
|---|------|-----|-------|--------|---------|
| 49 | BTC | ETH | TC | 0.0003 | `--from btc --to eth` |
| 50 | BTC | ETH | MP | 0.0003 | `--from btc --to eth --route mayachain` |
| 51 | AVAX | ETH | TC | 1 | `--from avax --to eth` |
| 52 | BNB | ETH | TC | 0.05 | `--from bnb --to eth` |
| 53 | BASE.ETH | ETH | TC | 0.005 | `--from base --to eth` |
| 54 | ARB.ETH | ETH | MP | 0.005 | `--from arb-eth --to eth --route mayachain` |
| 55 | LTC | ETH | TC | 0.3 | `--from ltc --to eth` |
| 56 | BCH | ETH | TC | 0.05 | `--from bch --to eth` |
| 57 | DOGE | ETH | TC | 100 | `--from doge --to eth` |
| 58 | ATOM | ETH | TC | 3 | `--from atom --to eth` |
| 59 | RUNE | ETH | TC | 10 | `--from rune --to eth` |
| 60 | RUNE | ETH | MP | 10 | `--from rune --to eth --route mayachain` |
| 61 | CACAO | ETH | MP | 10 | `--from cacao --to eth --route mayachain` |
| 62 | TRX | ETH | TC | 100 | `--from trx --to eth` |
| 63 | XRP | ETH | TC | 50 | `--from xrp --to eth` |
| 64 | DASH | ETH | MP | 0.5 | `--from dash --to eth --route mayachain` |
| 65 | ZEC | ETH | MP | 0.5 | `--from zec --to eth --route mayachain` |
| 66 | KUJI | ETH | MP | 20 | `--from kuji --to eth --route mayachain` |

---

## Summary

| Component | Count |
|-----------|-------|
| Assets in unified list | 31 |
| Phase 1: ETH → Tokens | 15 swaps |
| Phase 2: ETH → Gas | 18 swaps |
| Phase 3: Tokens → ETH | 15 swaps |
| Phase 4: Gas → ETH | 18 swaps |
| **Total swaps** | **66 swaps** |

### Dual-Route Coverage

| Asset | TC Swaps | MP Swaps |
|-------|----------|----------|
| BTC | 2 | 2 |
| ETH.USDC | 2 | 2 |
| ETH.USDT | 2 | 2 |
| RUNE | 2 | 2 |

---

## Part 3: Bug Fixing Requirements

If any swap fails to complete the full flow (generate → add → monitor with on-chain validation), the bug must be fixed before proceeding.

### Validation Criteria

Each swap must successfully:
1. **Generate** - `policy generate` creates valid policy JSON
2. **Add** - `policy add` submits policy without error
3. **Execute** - Policy executes and transaction is broadcast
4. **Validate** - Transaction confirmed on-chain

### Bug Fix Scope

| Repo | When to Fix |
|------|-------------|
| `vcli` | Policy generation fails, asset aliases missing, address derivation issues |
| `recipes` | Swap route not found, chain not supported, amount conversion errors |
| `verifier` | Policy validation fails, signature errors, TSS issues |
| `app-recurring` | Scheduler issues, worker execution fails, transaction broadcast errors |

### Bug Fix Process

1. Identify which component failed (check logs)
2. Fix the bug in the appropriate repo
3. Rebuild affected services (`make stop && make start`)
4. Retry the failed swap
5. Document the fix in the test results

### Log Locations

```bash
tail -f local/logs/verifier.log      # Policy validation, TSS
tail -f local/logs/worker.log        # Verifier worker
tail -f local/logs/dca-server.log    # DCA API
tail -f local/logs/dca-worker.log    # Swap execution
tail -f local/logs/dca-scheduler.log # Policy scheduling
```

---

## Prerequisites

1. Vault `FastPlugin1` imported with sufficient ETH balance (~0.7 ETH for all swaps)
2. Plugin `vultisig-dca-0000` installed
3. All services running (`make start`)

## Sibling Repos (for bug fixes)

```
vultisig/
├── vcli/           # This repo - CLI and policy generation
├── verifier/       # Policy verification + TSS signing
├── app-recurring/  # DCA plugin (scheduler, worker, server)
├── recipes/        # Chain abstraction, swap routing
└── go-wrappers/    # Crypto primitives (rarely needs changes)
```

## Execution

```bash
# Generate all policies
./scripts/generate-swap-policies.sh

# Add policies in phases
./scripts/add-phase1-policies.sh  # ETH → Tokens
./scripts/add-phase2-policies.sh  # ETH → Gas
# ... wait for execution ...
./scripts/add-phase3-policies.sh  # Tokens → ETH
./scripts/add-phase4-policies.sh  # Gas → ETH

# Monitor
./local/vcli.sh policy list --plugin vultisig-dca-0000
tail -f local/logs/dca-worker.log
```
