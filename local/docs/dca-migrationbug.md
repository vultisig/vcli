# DCA Migration Bug

## Issue
DCA worker fails to start with migration error:
```
FATA[0000] failed to initialize Postgres pool: failed to run migrations: failed to run plugin migrations: ERROR 20260102000000_add_amount.sql: failed to execute SQL query "ALTER TABLE tx_indexer ADD COLUMN amount TEXT;": ERROR: column "amount" of relation "tx_indexer" already exists (SQLSTATE 42701)
```

## Root Cause
Migration file `/Users/dev/dev/vultisig/verifier/plugin/tx_indexer/pkg/storage/migrations/20260102000000_add_amount.sql` is not idempotent:
```sql
-- Original (breaks on re-run)
ALTER TABLE tx_indexer ADD COLUMN amount TEXT;
```

The migration is being re-run despite being marked as applied in `goose_db_version` table. This suggests either:
1. Multiple goose_db_version tables (one per migration path)
2. The `plugin.WithMigrations()` function runs migrations from a different path than expected
3. Database state inconsistency from previous failed runs

## Observations
- `goose_db_version` table shows migration `20260102000000` as applied
- `tx_indexer` table already has `amount` column
- Yet the migration still attempts to run and fails

## Temporary Fix Applied
Changed migration to be idempotent:
```sql
ALTER TABLE tx_indexer ADD COLUMN IF NOT EXISTS amount TEXT;
```

## Investigation Needed
1. Check why goose is re-running applied migrations
2. Look at `plugin.WithMigrations()` implementation in verifier repo
3. Check if there are multiple migration directories being processed
4. Consider if DCA worker and other services share the same database but use different migration paths

## Files
- Migration: `/Users/dev/dev/vultisig/verifier/plugin/tx_indexer/pkg/storage/migrations/20260102000000_add_amount.sql`
- DCA Worker main: `/Users/dev/dev/vultisig/app-recurring/cmd/worker/main.go`
- Migration helper: `/Users/dev/dev/vultisig/verifier/plugin/migrations.go` (WithMigrations function)
