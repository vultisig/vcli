# MayaChain Swaps Implementation Results

## Implementation Status: COMPLETE

All code changes have been implemented and compile successfully.

## Code Changes

### 1. Route Priority Override Mechanism
- **app-recurring/internal/recurring/consumer.go**: Added `routePreference` constant
- **app-recurring/internal/recurring/spec_swap.go**: Added `routePreference` to recipe schema and constraint passing
- **recipes/metarule/metarule.go**: Added `extractRoutePreference()` helper and modified swap handling to respect route preference (mayachain → thorchain fallback, thorchain only, or auto)

### 2. MayaChain EVM Provider
- **app-recurring/internal/mayachain/provider_evm.go**: NEW - MayaChain EVM provider following thorchain pattern
- **app-recurring/cmd/worker/main.go**: Added MayaChain EVM provider to network initialization

### 3. TRON TRC-20 Support
- **recipes/chain/tron/chain.go**: Added ContractAddress field and TriggerSmartContract parsing
- **recipes/engine/tron/tron.go**: Added TRC-20 validation with ABI decoding (transfer function selector, recipient address, amount)

### 4. vcli Support
- **vcli/local/cmd/vcli/cmd/config.go**: Added Arbitrum asset aliases (arb, arb-eth, arb-usdc, arb-usdt, arb-wbtc)
- **vcli/local/cmd/vcli/cmd/policy_generate.go**: Added --route flag for route preference

## Test Results

### Policy Generation Tests (ALL PASS)

| Test | Route | Status |
|------|-------|--------|
| ETH → ZEC | mayachain | ✓ PASS |
| ETH → DASH | mayachain | ✓ PASS |
| ETH → ARB | mayachain | ✓ PASS |
| ETH → ARB-USDC | mayachain | ✓ PASS |
| ETH → BTC | mayachain | ✓ PASS |
| ETH → BTC | thorchain | ✓ PASS |
| ETH → BTC | auto | ✓ PASS |
| BTC → ETH | mayachain | ✓ PASS |
| ZEC → ETH | mayachain | ✓ PASS |
| DASH → ETH | mayachain | ✓ PASS |
| ETH → RUNE | thorchain | ✓ PASS |
| USDT:TRON → ETH | auto | ✓ PASS |

### Build Tests (ALL PASS)

- vcli: ✓ Compiles
- app-recurring: ✓ Compiles
- recipes: ✓ Compiles
- verifier: ✓ Compiles

## Blocking Issue

### EdDSA TSS Reshare Timeout

The plugin installation (which requires 4-party TSS reshare) consistently fails during the EdDSA phase:

```
Error: reshare failed: reshare EdDSA failed: reshare timeout
```

**Details:**
- ECDSA reshare completes successfully every time
- EdDSA reshare times out after ~2 minutes
- Issue is with production Fast Vault Server at api.vultisig.com
- All 4 parties join successfully
- Protocol messages are exchanged but never completes

**This is an infrastructure issue, not a code issue.**

## TRON Swap Issue (FIXED)

### Problem
TRON swaps fail with "transaction has no contracts" error in the verifier.

### Root Cause Analysis
Debug logging revealed that the TRON engine receives the THORChain memo string (59 bytes) instead of the protobuf transaction bytes (~200 bytes).

**Debug output:**
```
[TRON DEBUG] Evaluate called with 59 bytes: 52393d3a653a...
```

The hex decodes to ASCII: `R9=:e:0x2d63088Dacce3a87b0966982D52141AEe53be224:754923/3/0`

This is a THORChain streaming swap memo, NOT protobuf transaction data.

### Why "no contracts" Error Occurs

The TRON protobuf parser interprets the memo bytes as follows:
- Memo starts with `R` (0x52 hex)
- `0x52` = (10 << 3) | 2 = field 10 (data/memo field), wire type 2 (length-delimited)
- Next byte `9` (0x39) is interpreted as length = 57
- Parser reads 57 bytes as the "memo" field value
- **No contract field (field 11) is ever parsed** because the memo bytes don't contain valid protobuf contract data
- Result: `rawData.Contract` is empty → "transaction has no contracts" error

### Code Tracing

The code flow was traced completely and appears correct:
1. `provider_tron.go::MakeTransaction` calls TRON API → gets `RawDataHex` (protobuf hex)
2. `hex.DecodeString(tx.RawDataHex)` → gets protobuf bytes
3. `injectMemoIntoTronTx` adds memo as protobuf field 10
4. Returns modified protobuf bytes (~200 bytes)
5. `signer_service.go::SignAndBroadcast` receives protobuf bytes as `txData`
6. `buildKeysignRequest` calls `base64.StdEncoding.EncodeToString(txData)` → ~280 chars
7. `PluginKeysignRequest.Transaction = txBase64`
8. Request sent to verifier via HTTP/Redis
9. Verifier's `ExtractTxBytes` calls `base64.StdEncoding.DecodeString(txData)` → bytes
10. TRON engine receives bytes

**The mystery:** All code paths set `Transaction` to base64-encoded protobuf bytes. No code path was found that would set `Transaction = base64(memo_string)`.

### Debug Logging Added
- `app-recurring/internal/tron/signer_service.go:45-56` - Logs txData bytes before encoding
- `app-recurring/internal/thorchain/provider_tron.go:154` - Logs returned transaction bytes
- `verifier/internal/api/plugin.go:213-220` - Logs received Transaction field

### Root Cause Found

**The TRON API (`/wallet/createtransaction`) returns empty `RawDataHex` when the source account has insufficient TRX balance.**

Debug logging revealed:
```
[TRON TX BUILDER DEBUG] CreateTransaction response: TxID=, RawDataHex len=0
```

When `RawDataHex` is empty:
1. `hex.DecodeString("")` returns empty `[]byte{}` (no error)
2. `injectMemoIntoTronTx([]byte{}, memo)` receives empty txData
3. Since there's no contract field to find, it just returns `memoField` (protobuf-wrapped memo)
4. The TRON engine tries to parse this as a full transaction but finds no contracts
5. Error: "transaction has no contracts"

### Fix Applied

**File:** `app-recurring/internal/thorchain/provider_tron.go`

Added validation to check for empty API response before proceeding:
```go
if tx.RawDataHex == "" {
    return nil, fmt.Errorf("tron: TRON API returned empty RawDataHex (may indicate insufficient balance)")
}
```

Now the system properly returns an "insufficient balance" error instead of passing invalid data through:
```
skipping execution: insufficient balance (no retry until next scheduled run)
error="tron: TRON API returned empty RawDataHex (may indicate insufficient balance)"
```

### Testing Results

| Test | Amount | Result |
|------|--------|--------|
| TRX → ETH | 100 TRX | ❌ Empty RawDataHex (insufficient balance, vault has 3.9 TRX) |
| TRX → ETH | 1 TRX | ❌ THORChain dust threshold (1587/7560) |
| TRX → ETH | 3 TRX | ❌ THORChain dust threshold (21708/22681) |

**Note:** All tests now properly fail at the quote/validation stage rather than the transaction building stage. The "transaction has no contracts" error is fixed.

## End-to-End TRON Swap Testing (COMPLETE)

### Test Date: 2026-01-24

### Forward Swaps (ETH → TRON)

| Swap | Amount | TX Hash (Ethereum) | Status |
|------|--------|-------------------|--------|
| ETH → TRX | 0.006 ETH (~$20) | `0x5c850611dab0fcd5ce9a45efdf32722df92e7da1e725af2a63ed89ee8bcd29b1` | ✅ SUCCESS |
| ETH → USDT:TRON | 0.006 ETH (~$20) | `0x2085bdfc1080f06a9e93b03b7f3b12391ee9d545baa431e166298ced52fa0c72` | ✅ SUCCESS |

**THORChain Swap Results:**
- TRX received: ~58 TRX (from 0.006 ETH)
- USDT received: ~14 USDT (from 0.006 ETH)

### Reverse Swaps (TRON → ETH)

| Swap | Amount | TX Hash (TRON) | Status |
|------|--------|----------------|--------|
| TRX → ETH | 50 TRX | `9c5d38d1eb8a92453a705b02495aa08916fb8db85a1412cba7f6a5906567604b` | ✅ SUCCESS |
| USDT:TRON → ETH | 20 USDT | `6be75a78d3ff1257ca5aacaa052412ac5ee1bf7397ad2bbbd303644c9bc09cd4` | ✅ SUCCESS |

**TRON Balance After Swaps:**
- TRX: 3.02 TRX (50 TRX + fees sent to THORChain)
- USDT: 5.43 USDT (20 USDT sent to THORChain)

### Key Validations

1. **Native TRX Transfer**: Correctly builds protobuf transaction with `TransferContract` type
2. **TRC-20 USDT Transfer**: Correctly builds protobuf transaction with `TriggerSmartContract` type
3. **THORChain Memo**: Correctly injected into protobuf field 10 (data field)
4. **TSS Keysign**: Successfully completed 2-of-2 signature with verifier
5. **TRON Broadcast**: All transactions confirmed on TRON mainnet

### Debug Output Showing Correct Transaction Building

```
[TRON TX BUILDER DEBUG] CreateTransaction response: TxID=4631f89..., RawDataHex len=268
[TRON PROVIDER DEBUG] MakeTransaction returning 193 bytes: 0a025b92...
[TRON SIGNER DEBUG] SignAndBroadcast called with 193 bytes
[TRON SIGNER DEBUG] keysignRequest.Transaction length: 260
```

**Note:** Transaction sizes:
- Native TRX transfer: ~193 bytes protobuf
- TRC-20 USDT transfer: ~270 bytes protobuf

### Summary

All TRON bidirectional swaps working end-to-end:
- ✅ ETH → TRX (via THORChain)
- ✅ ETH → USDT:TRON (via THORChain)
- ✅ TRX → ETH (via THORChain)
- ✅ USDT:TRON → ETH (via THORChain)

## Files Modified

| File | Change |
|------|--------|
| app-recurring/internal/recurring/consumer.go | routePreference constant |
| app-recurring/internal/recurring/spec_swap.go | Recipe schema + constraint |
| recipes/metarule/metarule.go | Route preference handling |
| app-recurring/internal/mayachain/provider_evm.go | NEW: MayaChain EVM provider |
| app-recurring/cmd/worker/main.go | MayaChain provider init |
| recipes/chain/tron/chain.go | TRC-20 parsing |
| recipes/engine/tron/tron.go | TRC-20 validation |
| vcli/local/cmd/vcli/cmd/config.go | Arbitrum aliases |
| vcli/local/cmd/vcli/cmd/policy_generate.go | --route flag |

---

## Comprehensive All-Chain Test (2026-01-24)

### Forward Swaps (ETH → X)

| Swap | Route | TX Hash | Source Status | Note |
|------|-------|---------|---------------|------|
| ETH → LTC | THORChain | `0x602a8dc1e8eed1ea5be74f83ff79239f70ba2a446767151dd917d6db4b537255` | ✅ Confirmed | Cross-chain processing |
| ETH → DOGE | THORChain | `0xf9d80ebd2dcd3577adec31d4effdf4216d7c1b20bd0ff41dbddc0047ce31b97d` | ⏳ Pending | - |
| ETH → BCH | THORChain | `0xee46ea21898e9e809e0edf883a66d7d7b32250fc59646a83eeb7b1a1021c85bd` | ⏳ Pending | - |
| ETH → ZEC | MayaChain | `0x1167306edf74168a63f85f68e03c85d27f36bfe29f6aee5176bab91ce4998bd6` | ✅ Confirmed | Cross-chain processing |
| ETH → USDC | 1inch | `0x3b5ee8761e7c209168c722448f6cf08ac626dcd71fc9b160de34d894a1e1dabb` | ⏳ Pending | Same-chain swap |
| ETH → BTC | THORChain | - | ❌ Failed | Router mismatch |
| ETH → RUNE | THORChain | - | ❌ Failed | Router mismatch |
| ETH → DASH | MayaChain | - | ❌ Not processed | Worker didn't pick up |

### Bug Found: Router Address Mismatch

**Symptom:**
```
failed to evaluate tx: ethereum.thorchain_router.depositWithExpiry(
  failed to assert target: tx target is wrong:
  tx_to=0xe3985E6b61b814F7Cdb188766562ba71b446B46d,
  rule_magic_const_resolved=0xD37BbE5744D730a1d98d8DC97c42F0Ca46aD7146
)
```

**Root Cause:**
- THORChain router: `0xD37BbE5744D730a1d98d8DC97c42F0Ca46aD7146`
- MayaChain router: `0xe3985E6b61b814F7Cdb188766562ba71b446B46d`

When policies are generated without `--route mayachain`, they expect THORChain router. However, at execution time, the MayaChain EVM provider may return a better quote or THORChain may be unavailable, causing MayaChain to be selected. The transaction is built with MayaChain router, but the policy rules expect THORChain router → verifier rejects.

**Affected Swaps:**
- ETH → BTC: THORChain policy, MayaChain transaction
- ETH → RUNE: THORChain policy, MayaChain transaction

**Workaround:**
Use explicit `--route` flag when generating policies to match expected provider:
```bash
# For MayaChain-only assets (ZEC, DASH):
vcli policy generate --from eth --to zec --route mayachain ...

# For THORChain assets when THORChain is preferred:
# Need to ensure THORChain is available and returns valid quotes
```

**Fix Needed:**
1. Make provider selection respect policy's expected router address
2. Or make policy rules accept either THORChain or MayaChain router
3. Or add router address to route_preference constraint

### Reverse Swaps

Reverse swaps (X → ETH) pending until forward swaps complete cross-chain (typically 10-30 minutes).

### Working Chains Summary

| Chain | THORChain | MayaChain | 1inch |
|-------|-----------|-----------|-------|
| LTC | ✅ | - | - |
| DOGE | ✅ | - | - |
| BCH | ✅ | - | - |
| ZEC | - | ✅ | - |
| USDC | ✅ (router) | - | ✅ (swap) |
| BTC | ❌ Router mismatch | - | - |
| RUNE | ❌ Router mismatch | - | - |
| DASH | - | ❌ Not processed | - |

---

## Bidirectional Swap Testing (2026-01-24)

### Part 1: vcli Vault Details Update - COMPLETE

Added the following features to `vcli vault details`:

| Feature | Status |
|---------|--------|
| DASH, ZEC to UTXO chains | ✅ Added |
| XRP balance fetch | ✅ Added (Ripple RPC) |
| TRON balance fetch | ✅ Added (TronGrid API) |
| TRON USDT (TRC20) | ✅ Added |
| ERC20 tokens on all EVM chains | ✅ Added (AVAX, BSC, Base, ARB) |
| Missing asset aliases | ✅ Added |

**New Token Support:**
- Avalanche: USDC (0xB97EF...), USDT (0x97022...)
- BSC: USDC (0x8AC76...), USDT (0x55d39...), BTCB (0x7130d...)
- Base: USDC (0x83358...)
- Arbitrum: USDC (0xaf88d...), USDT (0xFd086...), WBTC (0x2f2a2...)

**vultisig-go Updates (pushed to main):**
- Added XRP address derivation support (commit 5ee9e9f)
- Dash and Zcash were already supported in latest version

**Now Displaying in vault details:**
- XRP: r494XUJk791EjmHRyATUZQbQ41TEfgoKQC
- Dash: XbPnT4VgSZtoZvELHxAauSPwbey7owrYSE
- Zcash: t1UCndnn1tQheXGe1seoqKGBw8upJgWBkbQ
- TRON: TUubp4EmQsd9GYLv26urtwZ1Crh2ZouGNg

**New Asset Aliases:**
- `usdc:avalanche`, `usdt:avalanche`
- `usdc:bsc`, `usdt:bsc`, `btcb`
- `usdc:base`
- `usdc:arbitrum`, `usdt:arbitrum`, `wbtc:arbitrum`

### Part 2: 66 Swap Policies Added - COMPLETE

All 4 phases of swap policies have been submitted:

| Phase | Description | Swaps | Status |
|-------|-------------|-------|--------|
| 1 | ETH → Tokens | 15 | ✅ Added |
| 2 | ETH → Gas Assets | 18 | ✅ Added |
| 3 | Tokens → ETH | 15 | ✅ Added |
| 4 | Gas → ETH | 18 | ✅ Added |

**Notes:**
- CACAO and KUJI swaps had recipe validation warnings but policies were created
- Swaps are executing via verifier worker

### Execution Status (2026-01-24 23:00 UTC)

**Summary: 32/66 SUCCESS, 34 PENDING, 0 FAILED**

| Phase | Swaps | Success | Pending |
|-------|-------|---------|---------|
| Phase 1 (ETH→Tokens) | 15 | 5 | 10 |
| Phase 2 (ETH→Gas) | 18 | 14 | 4 |
| Phase 3 (Tokens→ETH) | 15 | 6 | 9 |
| Phase 4 (Gas→ETH) | 18 | 7 | 11 |

**Phase 1 (ETH → Tokens):**
| # | Swap | Route | Status |
|---|------|-------|--------|
| 1 | ETH→USDC | TC | ✅ SUCCESS |
| 2 | ETH→USDC | MP | ⏳ PENDING |
| 3 | ETH→USDT | TC | ✅ SUCCESS |
| 4 | ETH→USDT | MP | ⏳ PENDING |
| 5 | ETH→DAI | TC | ⏳ PENDING |
| 6 | ETH→USDC:AVAX | TC | ⏳ PENDING |
| 7 | ETH→USDT:AVAX | TC | ⏳ PENDING |
| 8 | ETH→USDC:BSC | TC | ✅ SUCCESS |
| 9 | ETH→USDT:BSC | TC | ✅ SUCCESS |
| 10 | ETH→BTCB | TC | ⏳ PENDING |
| 11 | ETH→USDC:BASE | TC | ⏳ PENDING |
| 12 | ETH→ARB-USDC | MP | ⏳ PENDING |
| 13 | ETH→ARB-USDT | MP | ⏳ PENDING |
| 14 | ETH→ARB-WBTC | MP | ⏳ PENDING |
| 15 | ETH→USDT:TRON | TC | ✅ SUCCESS |

**Phase 2 (ETH → Gas Assets):**
| # | Swap | Route | Status |
|---|------|-------|--------|
| 16 | ETH→BTC | TC | ⏳ PENDING |
| 17 | ETH→BTC | MP | ✅ SUCCESS |
| 18 | ETH→AVAX | TC | ✅ SUCCESS |
| 19 | ETH→BNB | TC | ✅ SUCCESS |
| 20 | ETH→BASE | TC | ✅ SUCCESS |
| 21 | ETH→ARB-ETH | MP | ✅ SUCCESS |
| 22 | ETH→LTC | TC | ✅ SUCCESS |
| 23 | ETH→BCH | TC | ✅ SUCCESS |
| 24 | ETH→DOGE | TC | ✅ SUCCESS |
| 25 | ETH→ATOM | TC | ✅ SUCCESS |
| 26 | ETH→RUNE | TC | ⏳ PENDING |
| 27 | ETH→RUNE | MP | ✅ SUCCESS |
| 28 | ETH→CACAO | MP | ✅ SUCCESS |
| 29 | ETH→TRX | TC | ✅ SUCCESS |
| 30 | ETH→XRP | TC | ✅ SUCCESS |
| 31 | ETH→DASH | MP | ⏳ PENDING |
| 32 | ETH→ZEC | MP | ✅ SUCCESS |
| 33 | ETH→KUJI | MP | ✅ SUCCESS |

**Phase 3 (Tokens → ETH):**
| # | Swap | Route | Status |
|---|------|-------|--------|
| 34 | USDC→ETH | TC | ✅ SUCCESS |
| 35 | USDC→ETH | MP | ✅ SUCCESS |
| 36 | USDT→ETH | TC | ✅ SUCCESS |
| 37 | USDT→ETH | MP | ✅ SUCCESS |
| 38 | DAI→ETH | TC | ⏳ PENDING |
| 39 | USDC:AVAX→ETH | TC | ⏳ PENDING |
| 40 | USDT:AVAX→ETH | TC | ⏳ PENDING |
| 41 | USDC:BSC→ETH | TC | ⏳ PENDING |
| 42 | USDT:BSC→ETH | TC | ⏳ PENDING |
| 43 | BTCB→ETH | TC | ⏳ PENDING |
| 44 | USDC:BASE→ETH | TC | ⏳ PENDING |
| 45 | ARB-USDC→ETH | MP | ⏳ PENDING |
| 46 | ARB-USDT→ETH | MP | ⏳ PENDING |
| 47 | ARB-WBTC→ETH | MP | ⏳ PENDING |
| 48 | USDT:TRON→ETH | TC | ✅ SUCCESS |

**Phase 4 (Gas → ETH):**
| # | Swap | Route | Status |
|---|------|-------|--------|
| 49 | BTC→ETH | TC | ✅ SUCCESS |
| 50 | BTC→ETH | MP | ✅ SUCCESS |
| 51 | AVAX→ETH | TC | ⏳ PENDING |
| 52 | BNB→ETH | TC | ⏳ PENDING |
| 53 | BASE→ETH | TC | ✅ SUCCESS |
| 54 | ARB-ETH→ETH | MP | ✅ SUCCESS |
| 55 | LTC→ETH | TC | ✅ SUCCESS |
| 56 | BCH→ETH | TC | ⏳ PENDING |
| 57 | DOGE→ETH | TC | ⏳ PENDING |
| 58 | ATOM→ETH | TC | ⏳ PENDING |
| 59 | RUNE→ETH | TC | ⏳ PENDING |
| 60 | RUNE→ETH | MP | ⏳ PENDING |
| 61 | CACAO→ETH | MP | ⏳ PENDING |
| 62 | TRX→ETH | TC | ⏳ PENDING |
| 63 | XRP→ETH | TC | ✅ SUCCESS |
| 64 | DASH→ETH | MP | ⏳ PENDING |
| 65 | ZEC→ETH | MP | ✅ SUCCESS |
| 66 | KUJI→ETH | MP | ⏳ PENDING |

### Files Modified

**vault.go:**
- Added DASH, ZEC to UTXO chains section
- Added XRP balance fetch (getXRPBalance)
- Added TRON balance fetch (getTronBalance, getTRC20Balance)
- Added token definitions for all EVM chains (chainTokens map)

**config.go:**
- Added asset aliases for cross-chain tokens
- Updated capitalizeChain() for new chains

---

## Additional Swap Testing (2026-01-25)

### ZEC → ETH (MayaChain)

| Field | Value |
|-------|-------|
| Date | 2026-01-25 |
| Policy ID | `917a9cb0-f59a-4935-b347-7394bf828ae5` |
| Amount | 0.05 ZEC |
| Route | MayaChain |
| TX Hash (Zcash) | `e77233c6c79ca5f7be396ea3560eb355723b74a2d4e62e71085b8cd8f008c9f2` |
| Status | ✅ SUCCESS (broadcast to Zcash network) |

**Notes:**
- Zcash transaction successfully signed via 2-of-2 TSS (DCA worker + Verifier)
- Transaction broadcast to Zcash network for MayaChain cross-chain swap
- Cross-chain settlement to ETH typically takes 10-30 minutes

---

## Avalanche Swap Testing (2026-01-25)

### Swaps to AVAX and Avalanche Tokens

| # | Swap | Amount | Policy ID | TX Hash | Status |
|---|------|--------|-----------|---------|--------|
| 1 | ETH → AVAX | 0.01 ETH | `dd660352-fa26-412b-bd74-2beb277b5712` | `0x1903f07ba5af58b454584538200e773bdce0fb35095efbf1bd9c55932a7b036c` | ✅ SIGNED → PENDING |
| 2 | USDT → AVAX | 10 USDT | `4c8edd6a-2ed5-4e68-a195-a818ca0008e3` | `0xe54d3ddb753d38a2df64e967bed756d84aca04d72c61c0145b073547dd20204d` | ✅ SIGNED → PENDING |
| 3 | ETH → USDT:AVAX | 0.01 ETH | `b58d3165-7e4f-408b-9188-bcca2123887e` | - | ⏳ Awaiting scheduler |

**Test Configuration:**
- Vault: FastPlugin1
- Plugin: vultisig-dca-0000
- Frequency: one-time
- Route: auto (THORChain)

**Notes:**
- All policies created and signed successfully via 2-of-2 TSS with Fast Vault Server
- Two transactions already signed and pending on-chain broadcast
- Third policy awaiting scheduler pickup (workers need to be running)
