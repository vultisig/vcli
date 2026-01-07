# Comprehensive Swap Route Testing Plan

## Current Vault Balances
- ETH: 0.236259 ETH (~$800)
- USDT: 118,365 USDT
- USDC: 20.33 USDC
- BTC: 0.00155 BTC (~$150)
- BNB: 0.001213 (dust, skip)

## Constraints
- **Gas Reserve**: Keep ~0.05 ETH for gas (~$170)
- **THORChain Minimum**: ~$10-50 per swap
- **1inch Minimum**: Very low, usually $1-5
- **Time**: Each policy takes ~30s to execute (scheduler poll + keysign)

## Phase 1: Same-Chain Swaps (Ethereum via 1inch)
Test all ERC20 ↔ ETH and ERC20 ↔ ERC20 combinations

| # | From | To | Amount | Status |
|---|------|-----|--------|--------|
| 1 | ETH | USDC | 0.01 ETH (~$34) | pending |
| 2 | ETH | USDT | 0.01 ETH (~$34) | pending |
| 3 | USDC | USDT | 20 USDC | pending |
| 4 | USDT | USDC | 50 USDT | pending |
| 5 | USDC | ETH | 50 USDC | pending |
| 6 | USDT | ETH | 100 USDT | pending |

## Phase 2: Cross-Chain Swaps (via THORChain)
Test cross-chain routes

| # | From | To | Amount | Status |
|---|------|-----|--------|--------|
| 7 | ETH | BTC | 0.01 ETH | pending |
| 8 | USDT → BTC | - | 10 USDT | DONE ✓ |
| 9 | USDC | BTC | 20 USDC | pending |
| 10 | BTC | ETH | 0.0005 BTC (~$48) | pending |
| 11 | ETH | RUNE | 0.005 ETH (~$17) | pending |
| 12 | USDT | RUNE | 20 USDT | pending |

## Phase 3: Consolidate to ETH
Swap all remaining assets back to ETH

| # | From | To | Amount | Status |
|---|------|-----|--------|--------|
| 13 | BTC | ETH | ALL BTC | pending |
| 14 | USDT | ETH | ALL USDT | pending |
| 15 | USDC | ETH | ALL USDC | pending |
| 16 | RUNE | ETH | ALL RUNE | pending |

## Token Addresses
- ETH: native (empty string)
- USDT: 0xdAC17F958D2ee523a2206206994597C13D831ec7
- USDC: 0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48
- BTC: native (empty string)
- RUNE: native (empty string)

## Vault Address
- EVM: 0x65261c9d3b49367e6a49902B1e735b2e734F8ee7
- BTC: bc1q4hw77advuu4lk7pm88trhtufaza357p6r0hjdf
- THOR: thor1jq0huwma0gvujn7a4mlygw8kutdmja5kza4e5p

## Execution Notes
- Wait 30-60s between policies for scheduler to pick up
- Monitor /tmp/dca-worker.log for errors
- Check tx status with: vcli policy transactions <policy-id>
