# Queue Isolation Fix for 4-Party TSS Reshare

## Status: COMPLETE

All tests passed on 2026-01-14.

## Problem Statement

When a user installs a plugin (e.g., DCA), a 4-party TSS reshare occurs to grant the plugin access to their vault. The 4 parties are:

1. **User's CLI** (vcli)
2. **Fast Vault Server** (production)
3. **Verifier Worker**
4. **DCA Plugin Worker**

**Bug**: Both Verifier Worker and DCA Worker were listening on `default_queue`. When the reshare was initiated, both workers raced to pick up tasks. One worker type picked up both reshare tasks, resulting in only 3 unique parties participating. The reshare completed but with wrong keyshares.

**Evidence from logs (before fix)**:
```
Parties: [dev's MacBook Air-AC8 Server-08017 verifier-dev-8530 verifier-dev-8225]
```
Both party 3 and 4 have `verifier-dev-*` prefix — no `dca-worker-*` party participated.

## Solution

Separate task queues for each service:

| Component | Queue Name | Env Var |
|-----------|------------|---------|
| Verifier Worker | `default_queue` | (default) |
| DCA Server | `dca_plugin_queue` | `SERVER_TASKQUEUENAME` |
| DCA Worker | `dca_plugin_queue` | `TASK_QUEUE_NAME` |

## Changes Made

### 1. verifier (branch: `jp/queue-fix`)

**`plugin/server/config.go`** - Added configurable queue name:
```go
type Config struct {
    Host             string `mapstructure:"host" json:"host,omitempty"`
    Port             int64  `mapstructure:"port" json:"port,omitempty"`
    EncryptionSecret string `mapstructure:"encryption_secret" json:"encryption_secret,omitempty"`
    TaskQueueName    string `mapstructure:"task_queue_name" json:"task_queue_name,omitempty"`  // NEW
}
```

**`plugin/server/server.go`** - Use configurable queue:
```go
func (s *Server) taskQueueName() string {
    if s.cfg.TaskQueueName != "" {
        return s.cfg.TaskQueueName
    }
    return tasks.QUEUE_NAME  // "default_queue" fallback
}

// Enqueue uses configurable queue
asynq.Queue(s.taskQueueName())
```

### 2. app-recurring (branch: `jp/queue-fix`)

**`cmd/worker/main.go`** - Added configurable queue for worker:
```go
type config struct {
    LogFormat     logging.LogFormat
    TaskQueueName string `envconfig:"TASK_QUEUE_NAME" default:"default_queue"`  // NEW
    // ... rest of config
}

// Worker listens on configurable queue
consumer := asynq.NewServer(
    redisConnOpt,
    asynq.Config{
        Queues: map[string]int{
            cfg.TaskQueueName: 10,  // Changed from tasks.QUEUE_NAME
        },
    },
)
```

**`go.mod`** - Added local replace for verifier:
```go
replace github.com/vultisig/verifier => ../verifier
```

### 3. vultisig-cluster (branch: `jp/queue-fix`)

**`local/configs/dca-server.env`**:
```bash
SERVER_TASKQUEUENAME=dca_plugin_queue
```

**`local/configs/dca-worker.env`**:
```bash
TASK_QUEUE_NAME=dca_plugin_queue
```

## Test Results

```
=== Vault Import ===
Duration: 5.5 seconds
Status: SUCCESS

=== Plugin Install (4-Party TSS Reshare) ===
Parties: [dev's MacBook Air-AC8 Server-08017 verifier-dev-7643 dca-worker-9891]
Duration: 18 seconds
Status: SUCCESS (4 distinct parties!)

=== Keyshares ===
Verifier (MinIO): 458KB ✓
DCA (MinIO): 458KB ✓

=== Policy Create ===
Duration: 6 seconds
Status: SUCCESS
```

## KPIs (All Met)

| KPI | Target | Result |
|-----|--------|--------|
| Reshare parties | 4 distinct | ✓ CLI, Server, verifier-dev-*, dca-worker-* |
| Verifier keyshare | Stored in MinIO | ✓ 458KB |
| DCA keyshare | Stored in MinIO | ✓ 458KB |
| Policy creation | HTTP 200 | ✓ Success |
| Total reshare time | < 30 seconds | ✓ ~18 seconds |

## Files Modified

| File | Repo | Change |
|------|------|--------|
| `plugin/server/config.go` | verifier | Added `TaskQueueName` field |
| `plugin/server/server.go` | verifier | Added `taskQueueName()` helper, use in enqueue |
| `cmd/worker/main.go` | app-recurring | Added `TaskQueueName` config, use in consumer |
| `go.mod` | app-recurring | Added replace directive for local verifier |
| `local/configs/dca-server.env` | vultisig-cluster | Set `SERVER_TASKQUEUENAME` |
| `local/configs/dca-worker.env` | vultisig-cluster | Set `TASK_QUEUE_NAME` |
| `local/VCLI.md` | vultisig-cluster | Updated queue isolation docs |

## Next Steps

1. Commit changes to all three repos on `jp/queue-fix` branches
2. Create PRs for review
3. After merge, remove the replace directive from app-recurring go.mod
