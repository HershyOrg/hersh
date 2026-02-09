# Hersh

**Reactive framework for Go with managed execution and monitoring**

[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.21-blue)](https://go.dev/)

Hersh is a lightweight reactive framework that provides deterministic state management through a managed execution model.

## Features

- 🎯 **Managed Execution**: Single function triggered by messages or reactive changes
- 🔄 **WatchCall**: Polling-based reactive variables (tick-based with change detection)
- 📡 **WatchFlow**: Channel-based reactive variables (event streams)
- 💾 **Memo**: Session-scoped caching for expensive computations
- 📦 **HershContext**: Persistent key-value storage with atomic updates
- 🌐 **WatcherAPI**: HTTP server for external control and monitoring
- 🛡️ **Fault Tolerance**: Built-in recovery policies with exponential backoff
- 📊 **Execution Logging**: Track state transitions, errors, and context changes

## Quick Start

### Installation

```bash
go get github.com/HershyOrg/hersh@v0.2.0
```

### Complete Example

This example demonstrates all core features in one place:

```go
package main

import (
    "fmt"
    "time"
    "github.com/HershyOrg/hersh"
    "github.com/HershyOrg/hersh/manager"
)

func main() {
    // 1. Create Watcher with environment variables
    config := hersh.DefaultWatcherConfig()
    watcher := hersh.NewWatcher(config, map[string]string{
        "API_KEY": "secret",
    }, nil)

    // External data sources
    externalCounter := 0
    eventChan := make(chan any, 10)

    // Managed function with all features
    watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
        // 2. WatchCall: Polling-based reactive (checks every 500ms)
        counter := hersh.WatchCall(
            func() (manager.VarUpdateFunc, error) {
                // 함수 본문에선 네트워크 호출 등 가능.
                time.Sleep(20*time.Millisecond) 
                //return 함수 내부엔 가능한 외부효과 없는 계산만 이용
                return func(prev any) (any, bool, error) {
                    current := externalCounter
                    externalCounter++
                    if prev == nil {
                        return current, true, nil
                    }
                    return current, prev.(int) != current, nil
                }, nil
            },
            "counter", 500*time.Millisecond, ctx,
        )

        // 3. WatchFlow: Channel-based reactive
        event := hersh.WatchFlow(eventChan, "events", ctx)

        // 4. Memo: Cached computation (runs once per session)
        apiClient := hersh.Memo(func() any {
            fmt.Println("Initializing API client...")
            return &struct{ name string }{name: "client"}
        }, "apiClient", ctx)

        // 5. HershContext: Persistent state with atomic updates
        ctx.UpdateValue("totalRuns", func(current any) any {
            if current == nil {
                return 1
            }
            return current.(int) + 1
        })
        totalRuns := ctx.GetValue("totalRuns")

        // 6. Environment variables (immutable)
        apiKey, _ := ctx.GetEnv("API_KEY")

        fmt.Printf("Execution: counter=%v, event=%v, client=%v, runs=%v, key=%s\n",
            counter, event, apiClient, totalRuns, apiKey)

        // 7. Message handling with error control
        if msg != nil && msg.Content == "stop" {
            return hersh.NewStopErr("user requested stop")
        }

        return nil
    }, "app").Cleanup(func(ctx hersh.HershContext) {
        fmt.Println("Cleanup executed")
    })

    // Start watcher (blocks until Ready)
    watcher.Start()

    // 8. Send messages to trigger execution
    watcher.SendMessage("hello")
    time.Sleep(100 * time.Millisecond)

    // Send events to trigger WatchFlow
    eventChan <- "event1"
    time.Sleep(100 * time.Millisecond)

    // 9. Logger: Inspect execution history
    watcher.GetLogger().PrintSummary()

    // 10. Stop gracefully
    watcher.SendMessage("stop")
    time.Sleep(100 * time.Millisecond)
    watcher.Stop()
}
```

**Output**:

``` txt
Initializing API client...
Execution: counter=0, event=<nil>, client=&{client}, runs=1, key=secret
Execution: counter=1, event=<nil>, client=&{client}, runs=2, key=secret
Execution: counter=1, event=event1, client=&{client}, runs=3, key=secret

=== Logger Summary ===
Reduce Log Entries: 12
Effect Log Entries: 0
Effect Results: 8
Watch Error Log Entries: 0
Context Value Changes: 5

Cleanup executed
```

## Core Concepts

### Managed Function

Hersh uses a **single managed function** that executes:

1. **On Start**: Initial execution (`InitRun` state)
2. **On SendMessage**: When `SendMessage(content)` or API `/message` is called
3. **On WatchCall**: When polled value changes (every `tick` interval)
4. **On WatchFlow**: When channel receives new values

State flow: `NotRun → InitRun → Ready → Running → Ready → ...`

### Reactive Variables

#### WatchCall (Polling)

- Polls external values at fixed intervals (`tick`)
- Returns `VarUpdateFunc` that computes `(next, changed, error)` from `prev`
- Re-executes managed function only when `changed = true`
- Returns `nil` until first value is ready

#### WatchFlow (Channel)

- Monitors channels for new values
- Pushes values directly without computation
- Re-executes managed function on each channel event
- Returns `nil` until first value received

### Memo vs HershContext

| Feature | Memo | HershContext |
|---------|------|--------------|
| **Purpose** | Cache expensive computations | Store persistent state |
| **Lifetime** | Session (until `ClearMemo` or restart) | Session (across all executions) |
| **Triggers** | Does NOT trigger re-execution | Does NOT trigger re-execution |
| **Thread-safe** | ✅ `LoadOrStore` semantics | ✅ Mutex-protected |
| **Use case** | DB connections, HTTP clients | Counters, statistics, flags |

### Error Handling

Control Watcher lifecycle by returning special errors:

```go
// Graceful stop (cleanup executes, can't recover)
return hersh.NewStopErr("user stop")

// Force kill (no cleanup, immediate shutdown)
return hersh.NewKillErr("critical error")

// Crash with recovery (cleanup executes, may recover)
return hersh.NewCrashErr("recoverable error")

// Normal error (logs but continues execution)
return fmt.Errorf("non-fatal error")
```

### State Lifecycle

```
NotRun → InitRun → Ready ⇄ Running
                    ↓
                Stopped/Killed (permanent)
                    ↓
                Crashed → WaitRecover → Ready (or Crashed permanently)
```

Terminal states: `Stopped`, `Killed`, `Crashed` (after max retries)

## Complete API Reference

### Watcher Methods

| Method | Description |
|--------|-------------|
| `NewWatcher(config, envVars, parentCtx)` | Create Watcher with configuration and environment variables |
| `Manage(fn, name)` | Register managed function, returns `CleanupBuilder` |
| `.Cleanup(cleanupFn)` | Register cleanup function (called on Stop/Kill/Crash) |
| `Start()` | Start Watcher (blocks until `Ready` or error) |
| `Stop()` | Graceful stop with cleanup (blocks until complete) |
| `SendMessage(content)` | Send message to trigger managed function |
| `GetState()` | Get current state (`Ready`, `Running`, `Stopped`, etc.) |
| `GetLogger()` | Access execution logs for inspection |
| `StartAPIServer()` | Start HTTP API server (default port 8080) |

### Logger Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `PrintSummary()` | - | Print execution summary to stdout |
| `GetReduceLog()` | `[]ReduceLogEntry` | State transitions (signals processed) |
| `GetEffectLog()` | `[]EffectLogEntry` | Effect execution logs (user messages) |
| `GetWatchErrorLog()` | `[]WatchErrorLogEntry` | Watch variable errors (computation failures) |
| `GetContextLog()` | `[]ContextValueLogEntry` | Context value changes (`SetValue`, `UpdateValue`) |
| `GetStateTransitionFaultLog()` | `[]StateTransitionFaultLogEntry` | Invalid state transitions (errors) |
| `GetRecentResults(count)` | `[]*EffectResult` | N most recent effect results |

**Example**:
```go
logger := watcher.GetLogger()
logger.PrintSummary()

// Get specific logs
contextChanges := logger.GetContextLog()
watchErrors := logger.GetWatchErrorLog()
```

### Reactive Functions

| Function | Description |
|----------|-------------|
| `WatchCall(getComputationFunc, varName, tick, ctx)` | Polling-based reactive (returns current value or `nil`) |
| `WatchFlow(sourceChan, varName, ctx)` | Channel-based reactive (returns latest value or `nil`) |
| `Memo(computeValue, memoName, ctx)` | Session-scoped cache (returns computed value) |
| `ClearMemo(memoName, ctx)` | Clear cached value (forces recomputation) |

**WatchCall signature**:
```go
func WatchCall(
    getComputationFunc func() (manager.VarUpdateFunc, error),
    varName string,
    tick time.Duration,
    ctx HershContext,
) any
```

**VarUpdateFunc signature**:
```go
type VarUpdateFunc func(prev any) (next any, changed bool, err error)
```

### HershContext Interface

| Method | Description |
|--------|-------------|
| `WatcherID()` | Watcher's unique identifier |
| `Message()` | Current user message (`nil` if none) |
| `GetValue(key)` | Get stored value (returns actual value, not copy) |
| `SetValue(key, value)` | Set stored value (simple assignment) |
| `UpdateValue(key, updateFn)` | Atomic update with **deep copy** (thread-safe) |
| `GetEnv(key)` | Get immutable environment variable (set at Watcher creation) |
| `GetWatcher()` | Watcher reference (as `any`) |

**UpdateValue example** (atomic, thread-safe):
```go
// updateFn receives a deep copy of current value
ctx.UpdateValue("stats", func(current any) any {
    if current == nil {
        return map[string]int{"count": 1}
    }
    stats := current.(map[string]int)
    stats["count"]++
    return stats
})
```

### Error Constructors

| Constructor | Lifecycle | Cleanup? | Recovery? |
|-------------|-----------|----------|-----------|
| `NewStopErr(reason)` | Stopped (permanent) | ✅ Yes | ❌ No |
| `NewKillErr(reason)` | Killed (permanent) | ❌ No | ❌ No |
| `NewCrashErr(reason)` | Crashed → WaitRecover | ✅ Yes | ✅ Yes (if configured) |

### Configuration Types

#### WatcherConfig

```go
type WatcherConfig struct {
    DefaultTimeout     time.Duration  // Managed function timeout (default: 1min)
    RecoveryPolicy     RecoveryPolicy // Fault tolerance policy
    ServerPort         int            // API server port (default: 8080)
    MaxLogEntries      int            // Log buffer size (default: 50,000)
    MaxWatches         int            // Max concurrent watches (default: 1,000)
    MaxMemoEntries     int            // Max memo cache size (default: 1,000)
    SignalChanCapacity int            // Signal queue size (default: 50,000)
}

func DefaultWatcherConfig() WatcherConfig
```

#### RecoveryPolicy

```go
type RecoveryPolicy struct {
    MinConsecutiveFailures int           // Failures before WaitRecover (default: 3)
    MaxConsecutiveFailures int           // Failures before Crashed (default: 6)
    BaseRetryDelay         time.Duration // Initial retry delay (default: 5s)
    MaxRetryDelay          time.Duration // Max retry delay cap (default: 5min)
    LightweightRetryDelays []time.Duration // Delays for failures <3 (default: [15s, 30s, 60s])
}

func DefaultRecoveryPolicy() RecoveryPolicy
```

**Behavior**:
- **Failures < 3**: Lightweight retry with delays `[15s, 30s, 60s]` → `Ready`
- **Failures ≥ 3**: Heavy retry with exponential backoff (`5s → 10s → 20s → ...`) → `WaitRecover`
- **Failures ≥ 6**: Permanent `Crashed` state (no more retries)

**Example**:
```go
config := hersh.WatcherConfig{
    DefaultTimeout: 30 * time.Second,
    RecoveryPolicy: hersh.RecoveryPolicy{
        MinConsecutiveFailures: 2,
        MaxConsecutiveFailures: 5,
        BaseRetryDelay:         3 * time.Second,
        MaxRetryDelay:          1 * time.Minute,
    },
}
```

## WatcherAPI (HTTP Endpoints)

### Control & State

```bash
# Get current state (Ready, Running, Stopped, Killed, Crashed, WaitRecover)
GET /watcher/status

# Get detailed state (execution count, error count, uptime)
GET /watcher/state

# Send message to trigger managed function
POST /watcher/message
Content-Type: application/json
{"content": "your-command"}

# Get Watcher configuration
GET /watcher/config
```

### Monitoring

```bash
# Environment variables
GET /watcher/vars

# Watch variables (WatchCall/WatchFlow current values)
GET /watcher/watching

# Memo cache contents
GET /watcher/memoCache

# HershContext variable state (GetValue/SetValue)
GET /watcher/varState
```

### Logs

```bash
# State transition logs (Reducer actions)
GET /watcher/logs/reduce

# Effect execution logs (managed function runs)
GET /watcher/logs/effect

# Watch errors (computation failures)
GET /watcher/logs/watch-error

# Context value changes (SetValue/UpdateValue)
GET /watcher/logs/context

# State transition faults (invalid transitions)
GET /watcher/logs/state-fault
```

**Example**:
```bash
# Start API server (in managed function or before Start)
watcher.StartAPIServer()

# Query from external process
curl http://localhost:8080/watcher/status
# {"status": "Ready"}

curl http://localhost:8080/watcher/logs/context
# [{"logID": 1, "key": "totalRuns", "newValue": 5, ...}]
```

## Examples

### Example 1: Recovery Policy Demo

Demonstrates automatic recovery with exponential backoff:

```go
config := hersh.DefaultWatcherConfig()
config.RecoveryPolicy = hersh.RecoveryPolicy{
    MinConsecutiveFailures: 2,
    MaxConsecutiveFailures: 4,
    BaseRetryDelay:         1 * time.Second,
    LightweightRetryDelays: []time.Duration{500 * time.Millisecond, 1 * time.Second},
}

watcher := hersh.NewWatcher(config, nil, nil)

watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
    failCount := ctx.GetValue("failCount")
    if failCount == nil {
        failCount = 0
    }
    count := failCount.(int)

    fmt.Printf("Execution attempt %d (state: %s)\n", count+1, watcher.GetState())

    if count < 5 {
        ctx.SetValue("failCount", count+1)
        return hersh.NewCrashErr(fmt.Sprintf("simulated failure %d", count+1))
    }

    fmt.Println("Success after recovery!")
    return nil
}, "recovery-demo")

watcher.Start()
time.Sleep(15 * time.Second) // Wait for retries
watcher.GetLogger().PrintSummary()
watcher.Stop()
```

**Output**:
```
Execution attempt 1 (state: InitRun)
Execution attempt 2 (state: Ready)      # 500ms delay (lightweight)
Execution attempt 3 (state: Ready)      # 1s delay (lightweight)
Execution attempt 4 (state: WaitRecover) # 1s delay (heavy)
Execution attempt 5 (state: WaitRecover) # 2s delay (exponential)
Execution attempt 6 (state: WaitRecover) # 4s delay (exponential)
Success after recovery!

=== Logger Summary ===
State Transition Fault Entries: 5
```

### Example 2: Real-time Event Pipeline

Demonstrates WatchFlow with processing pipeline:

```go
eventChan := make(chan any, 100)
watcher := hersh.NewWatcher(hersh.DefaultWatcherConfig(), nil, nil)

watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
    // Watch incoming events
    event := hersh.WatchFlow(eventChan, "eventStream", ctx)

    if event != nil {
        // Process event
        processed := fmt.Sprintf("processed_%v", event)

        // Store in context
        ctx.SetValue("lastProcessed", processed)

        // Update statistics atomically
        ctx.UpdateValue("stats", func(current any) any {
            if current == nil {
                return map[string]int{"total": 1}
            }
            stats := current.(map[string]int)
            stats["total"]++
            return stats
        })

        stats := ctx.GetValue("stats").(map[string]int)
        fmt.Printf("Event: %v → %s (total: %d)\n", event, processed, stats["total"])
    }

    // Handle control messages
    if msg != nil && msg.Content == "status" {
        stats := ctx.GetValue("stats")
        fmt.Printf("Pipeline stats: %+v\n", stats)
    }

    return nil
}, "pipeline")

watcher.Start()

// Producer goroutine
go func() {
    for i := 1; i <= 5; i++ {
        eventChan <- fmt.Sprintf("event%d", i)
        time.Sleep(100 * time.Millisecond)
    }
}()

time.Sleep(600 * time.Millisecond)
watcher.SendMessage("status")
time.Sleep(100 * time.Millisecond)
watcher.Stop()
```

**Output**:
```
Event: event1 → processed_event1 (total: 1)
Event: event2 → processed_event2 (total: 2)
Event: event3 → processed_event3 (total: 3)
Event: event4 → processed_event4 (total: 4)
Event: event5 → processed_event5 (total: 5)
Pipeline stats: map[total:5]
```

## Architecture

### State Machine

```
NotRun → InitRun → Ready ⇄ Running
                    ↓
                Stopped/Killed (permanent)
                    ↓
                Crashed → WaitRecover → Ready
                                  ↓
                                Crashed (permanent, after MaxConsecutiveFailures)
```

**State descriptions**:
- **NotRun**: Before `Start()`
- **InitRun**: First execution (initialization phase)
- **Ready**: Idle, waiting for triggers (messages, reactive changes)
- **Running**: Executing managed function
- **Stopped**: Graceful shutdown via `StopErr` or `Stop()` (permanent)
- **Killed**: Force shutdown via `KillErr` (permanent)
- **Crashed**: Unrecoverable error after max retries (permanent)
- **WaitRecover**: Waiting to retry after crash (temporary)

### Signal Priority

Internal signal processing order (lower = higher priority):

```
Priority 0: WatcherSig (lifecycle: InitRun, Stop, Kill, Recover)
Priority 1: UserSig     (SendMessage, API /message)
Priority 2: VarSig      (WatchCall/WatchFlow reactive triggers)
```

This ensures lifecycle commands always take precedence over user messages and reactive triggers.

### Package Structure

```
github.com/HershyOrg/hersh/
├── watcher.go           # Core Watcher API
├── watcher_api.go       # HTTP API server
├── watch.go             # WatchCall, WatchFlow
├── memo.go              # Memo caching
├── types.go             # Public types (re-exports from shared/)
├── manager/             # Internal Manager (Reducer-Effect pattern)
│   ├── manager.go       # Manager orchestrator
│   ├── reducer.go       # Pure state transitions
│   ├── effect_handler.go # Effect execution
│   ├── logger.go        # Execution logging
│   └── signal.go        # Signal priority queue
├── hctx/                # HershContext implementation
│   └── context.go
├── shared/              # Shared types and errors
│   ├── types.go
│   └── errors.go
├── api/                 # WatcherAPI HTTP handlers
└── demo/                # Usage examples
    ├── example_simple.go
    ├── example_watchcall.go
    └── example_trading.go
```

## Real-World Usage

Hersh powers **[Hershy](https://github.com/HershyOrg/hershy)**, a container orchestration system that manages Docker containers with reactive state management.

**Production examples**:
- [simple-counter](https://github.com/HershyOrg/hershy/tree/main/examples/simple-counter): Basic counter with WatcherAPI control
- [trading-long](https://github.com/HershyOrg/hershy/tree/main/examples/trading-long): Real-time trading simulator
- [watcher-server](https://github.com/HershyOrg/hershy/tree/main/examples/watcher-server): HTTP server with persistent state

## Links

- **Repository**: https://github.com/HershyOrg/hersh
- **Documentation**: https://pkg.go.dev/github.com/HershyOrg/hersh
- **Issues**: https://github.com/HershyOrg/hersh/issues
- **Hershy (Container Orchestration)**: https://github.com/HershyOrg/hershy

