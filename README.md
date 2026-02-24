# Hersh

**Reactive framework for Go with managed execution and monitoring**

[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.24-blue)](https://go.dev/)

Hersh is a lightweight reactive framework that provides deterministic state management through a managed execution model with comprehensive error handling and monitoring capabilities.

## Features

- 🎯 **Managed Execution**: Single function triggered by messages or reactive changes
- 🔄 **WatchCall**: Polling-based reactive variables with explicit signal control
- 📡 **WatchFlow**: Channel-based reactive variables with signal skipping support
- ⏰ **WatchTick**: Time-based ticker helper (automatic count tracking)
- 💾 **Memo**: Session-scoped caching for expensive computations
- 📦 **HershContext**: Persistent key-value storage with atomic updates
- 🎬 **TriggeredSignal**: Inspect which signals triggered current execution
- 🌐 **WatcherAPI**: HTTP server for external control and monitoring
- 🛡️ **Fault Tolerance**: Built-in recovery policies with exponential backoff
- 📊 **Execution Logging**: Track state transitions, errors, and context changes
- ✅ **Explicit Error Handling**: HershValue type with built-in error information

## Quick Start

### Installation

```bash
go get github.com/HershyOrg/hersh@v0.3.1
```

### Demo

!! COMPLETE DEMO AT /demo/long/main.go !!

```go
package main

import (
 "bufio"
 "context"
 "fmt"
 "os"
 "os/signal"
 "strings"
 "syscall"
 "time"

 "github.com/HershyOrg/hersh"
 "github.com/HershyOrg/hersh/hutil"
)

func main() {

 //... omit initiate logic...///

 // Create Watcher config
 config := hersh.DefaultWatcherConfig()
// Create EnvVars
 envVars := map[string]string{
  "DEMO_NAME":    DemoName,
  "DEMO_VERSION": DemoVersion,
 }
 // Create context with 10-minute timeout for graceful shutdown
 ctx, cancel := context.WithTimeout(context.Background(), TargetDuration)
 defer cancel()


 // Create Watcher with timeout context - it will auto-stop when context expires
 watcher := hersh.NewWatcher(config, envVars, ctx)
 // Register managed function with closure
 watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
  return mainReducer(
   msg, ctx,
   stream,
   simulator,
   statsCollector,
   commandHandler,
  )
 }, "TradingSimulator").Cleanup(func(ctx hersh.HershContext) {
  cleanup(ctx, stream, simulator, statsCollector)
 })



 select {
 case <-ctx.Done():
  time.Sleep(200 * time.Millisecond)
 }
}

// mainReducer is the main managed function for the Watcher
func mainReducer(
 msg *hersh.Message,
 ctx hersh.HershContext,
 stream *BinanceStream,
 simulator *TradingSimulator,
 statsCollector *StatsCollector,
 commandHandler *CommandHandler,
) error {
 // WatchFlow: BTC price (real-time from WebSocket)
 btcHV := hersh.WatchFlow(stream.GetBTCPriceStream(), "btc_price", ctx)
 if btcHV.IsValid() {
  simulator.UpdatePrice("BTC", btcHV.Value.(float64))
 }

 // WatchFlow: ETH price (real-time from WebSocket)
 ethHV := hersh.WatchFlow(stream.GetETHPriceStream(), "eth_price", ctx)
 if ethHV.IsValid() {
  simulator.UpdatePrice("ETH", ethHV.Value.(float64))
 }

 // WatchTick: Stats ticker (1 minute interval)
 statsTick := hutil.WatchTick("stats_ticker", StatsInterval, ctx)
 if !statsTick.IsZero() && statsTick.IsTriggered(ctx) {
  statsCollector.PrintStats(stream, simulator)
  hersh.PrintWithLog(fmt.Sprintf("   (Stats tick #%d at %s)", statsTick.TickCount, statsTick.Time.Format("15:04:05")), ctx)
 }

 // WatchTick: Rebalance ticker (1 hour interval)
 rebalanceTick := hutil.WatchTick("rebalance_ticker", RebalanceInterval, ctx)
 if !rebalanceTick.IsZero() && rebalanceTick.IsTriggered(ctx) {
  hersh.PrintWithLog(fmt.Sprintf("\n⏰ Hourly rebalance triggered (tick #%d at %s)...",
   rebalanceTick.TickCount, rebalanceTick.Time.Format("15:04:05")), ctx)
  trades := simulator.Rebalance()

  if len(trades) > 0 {
   hersh.PrintWithLog(fmt.Sprintf("   Executed %d rebalancing trades", len(trades)), ctx)
   for _, t := range trades {
    hersh.PrintWithLog(fmt.Sprintf("      %s %s: %.6f @ $%.2f",
     t.Action, t.Symbol, t.Amount, t.Price), ctx)
   }
  } else {
   hersh.PrintWithLog("   No rebalancing needed", ctx)
  }
 }

 // Execute trading strategy (unless paused)
 if !simulator.IsPaused() {
  trades := simulator.ExecuteStrategy()

  if len(trades) > 0 {
   hersh.PrintWithLog(fmt.Sprintf("\n💹 Strategy executed %d trades:", len(trades)), ctx)
   for _, t := range trades {
    hersh.PrintWithLog(fmt.Sprintf("   %s %s %s: %.6f @ $%.2f (%s)",
     t.Time.Format("15:04:05"),
     t.Action, t.Symbol, t.Amount, t.Price, t.Reason), ctx)
   }

   portfolio := simulator.GetPortfolio()
   hersh.PrintWithLog(fmt.Sprintf("   Portfolio Value: $%.2f (%.2f%%)",
    portfolio.CurrentValue, portfolio.ProfitLossPercent), ctx)
  }
 }

 // Handle user messages (commands)
 if msg != nil && msg.Content != "" {
  commandHandler.HandleCommand(msg.Content, ctx)
 }

 return nil
}

// cleanup is called when the Watcher stops
func cleanup(
 ctx hersh.HershContext,
 stream *BinanceStream,
 simulator *TradingSimulator,
 statsCollector *StatsCollector,
) {
 fmt.Println("\n🔧 Cleanup started...")

 // Close WebSocket
 fmt.Println("   Closing WebSocket...")
 stream.Close()

 // Print final statistics
 fmt.Println("\n📊 Final Statistics:")
 statsCollector.PrintDetailedStats(stream, simulator)

 fmt.Println("\n✅ Cleanup complete")
}

```

### Complete Example

This example demonstrates all core features in one place:

```go
package main

import (
    "context"
    "fmt"
    "time"
    "github.com/HershyOrg/hersh"
    "github.com/HershyOrg/hersh/hutil"
    "github.com/HershyOrg/hersh/manager"
    "github.com/HershyOrg/hersh/shared"
)

func main() {
    // 1. Create Watcher with environment variables
    config := hersh.DefaultWatcherConfig()
    watcher := hersh.NewWatcher(config, map[string]string{
        "API_KEY": "secret",
    }, nil)

    // External data sources
    externalCounter := 0

    // Managed function with all features
    watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
        // 2. WatchCall: Polling-based reactive (checks every 500ms) - Returns HershValue
        counter := hersh.WatchCall(
            func() (manager.VarUpdateFunc, error) {
                return func(prev shared.HershValue) (shared.HershValue, bool, error) {
                    current := externalCounter
                    externalCounter++

                    if prev.IsZero() {
                        return shared.HershValue{Value: current}, true, nil
                    }

                    prevVal := prev.Value.(int)
                    return shared.HershValue{Value: current}, prevVal != current, nil
                }, nil
            },
            "counter", 500*time.Millisecond, ctx,
        )

        // 3. Handle errors from reactive variables
        if counter.IsError() {
            fmt.Printf("Counter error: %v\n", counter.Error)
        } else if counter.IsValid() {
            fmt.Printf("Counter value: %d\n", counter.Value.(int))
        }

        // 4. WatchFlow: Channel-based reactive - Returns HershValue
        event := hersh.WatchFlow(
            func(flowCtx context.Context) (<-chan shared.FlowValue, error) {
                ch := make(chan shared.FlowValue, 10)
                go func() {
                    defer close(ch)
                    // Producer logic here
                }()
                return ch, nil
            },
            "events", ctx,
        )

        // 5. WatchTick: Time-based ticker helper (hutil package) - Returns HershTick
        tick := hutil.WatchTick("stats_ticker", 1*time.Minute, ctx)
        if !tick.IsZero() {
            fmt.Printf("Tick #%d at %s\n", tick.TickCount, tick.Time.Format("15:04:05"))
        }

        // 6. Memo: Cached computation (runs once per session)
        apiClient := hersh.Memo(func() any {
            fmt.Println("Initializing API client...")
            return &struct{ name string }{name: "client"}
        }, "apiClient", ctx)

        // 7. HershContext: Persistent state with atomic updates
        ctx.UpdateValue("totalRuns", func(current any) any {
            if current == nil {
                return 1
            }
            return current.(int) + 1
        })
        totalRuns := ctx.GetValue("totalRuns")

        // 8. Environment variables (immutable)
        apiKey, _ := ctx.GetEnv("API_KEY")

        // 9. TriggeredSignal: Check what triggered this execution
        trigger := ctx.GetTriggeredSignal()
        if trigger != nil {
            if trigger.IsUserSig {
                fmt.Println("Triggered by user message:", msg.Content)
            }
            if counter.IsTriggered(ctx) {
                fmt.Println("Counter changed!")
            }
        }

        // 10. Logging
        hersh.Log(fmt.Sprintf("Execution complete: runs=%v", totalRuns), ctx)

        fmt.Printf("Summary: counter=%v, event=%v, client=%v, runs=%v, key=%s\n",
            counter.Value, event.Value, apiClient, totalRuns, apiKey)

        // 11. Message handling with error control
        if msg != nil && msg.Content == "stop" {
            return hersh.NewStopErr("user requested stop")
        }

        return nil
    }, "app").Cleanup(func(ctx hersh.HershContext) {
        fmt.Println("Cleanup executed")
    })

    // Start watcher (blocks until Ready)
    watcher.Start()

    // 12. Send messages to trigger execution
    watcher.SendMessage("hello")
    time.Sleep(100 * time.Millisecond)

    // 13. Logger: Inspect execution history
    watcher.GetLogger().PrintSummary()

    // 14. Stop gracefully
    watcher.SendMessage("stop")
    time.Sleep(100 * time.Millisecond)
    watcher.Stop()
}
```

## Core Concepts

### Managed Function

Hersh uses a **single managed function** that executes:

1. **On Start**: Initial execution (`InitRun` state)
2. **On SendMessage**: When `SendMessage(content)` or API `/message` is called
3. **On WatchCall**: When polled value changes (every `tick` interval)
4. **On WatchFlow**: When channel receives new values
5. **On WatchTick**: When time interval elapses

State flow: `NotRun → InitRun → Ready → Running → Ready → ...`

### Reactive Variables

#### WatchCall (Polling with Signal Control)

Polls external values at fixed intervals with explicit signal control:

```go
counter := hersh.WatchCall(
    func() (manager.VarUpdateFunc, bool, error) {
        // This outer function is called every tick
        // Can perform network calls, file reads, etc.

        // The inner VarUpdateFunc computes next state from previous
        updateFunc := func(prev shared.HershValue) (shared.HershValue, error) {
            // Should be pure (no side effects)
            value := fetchExternalValue()
            return shared.HershValue{Value: value}, nil
        }

        // Signal control: false = send signal, true = skip signal
        skipSignal := false  // Default: send signal to trigger execution

        return updateFunc, skipSignal, nil
    },
    "varName", 500*time.Millisecond, ctx,
)

// Handle the result
if counter.IsError() {
    fmt.Printf("Error: %v\n", counter.Error)
} else if counter.IsValid() {
    fmt.Printf("Value: %v\n", counter.Value)
}
```

**Characteristics**:

- Returns `HershValue` (not `any`)
- **Signal Control**: `skipSignal = false` sends signal (triggers execution), `true` skips
- **No change detection**: Execution is controlled by signals, not value changes
- Returns empty `HershValue` until first value is ready
- State-dependent: processes updates sequentially

#### WatchFlow (Channel with Signal Control)

Monitors channels for new values with explicit signal control:

```go
event := hersh.WatchFlow(
    func(flowCtx context.Context) (<-chan shared.FlowValue, error) {
        ch := make(chan shared.FlowValue, 10)

        go func() {
            defer close(ch)

            for {
                select {
                case <-flowCtx.Done():
                    return
                case data := <-externalSource:
                    // SkipSignal: false = send signal (default), true = skip signal
                    ch <- shared.FlowValue{
                        V:          data,
                        E:          nil,
                        SkipSignal: false,  // Send signal to trigger execution
                    }
                }
            }
        }()

        return ch, nil
    },
    "events", ctx,
)

// Handle the result
if event.IsValid() {
    fmt.Printf("Event: %v\n", event.Value)
}
```

**Characteristics**:

- Returns `HershValue` (not `any`)
- **Signal Control**: `SkipSignal = false` sends signal (triggers execution), `true` skips
- **No change detection**: Execution is controlled by signals, not value changes
- Channel lifecycle managed by `flowCtx`
- Errors can be sent via `FlowValue{V: nil, E: err}`
- Returns empty `HershValue` until first value received
- State-independent: uses last value only (deduplication)

#### WatchTick (Time-based Helper)

Convenience wrapper around `WatchFlow` for time-based operations:

```go
import "github.com/HershyOrg/hersh/hutil"

tick := hutil.WatchTick("stats_ticker", 1*time.Minute, ctx)

if !tick.IsZero() {
    fmt.Printf("Tick #%d at %s\n", tick.TickCount, tick.Time.Format("15:04:05"))

    // Check if this tick triggered current execution
    if tick.IsTriggered(ctx) {
        // Perform periodic task
    }
}
```

**Characteristics**:

- Returns `HershTick` (not `HershValue`)
- Automatic tick counting (starts from 1)
- Sends initial tick immediately
- Built on `WatchFlow` (state-independent)

### HershValue & HershTick

#### HershValue (Watch Variable Return Type)

All `WatchCall` and `WatchFlow` functions return `HershValue`:

```go
type HershValue struct {
    Value   any    // The actual value (may be nil)
    Error   error  // Error that occurred during computation (nil if no error)
    VarName string // Name of the watched variable
}
```

**Methods**:

| Method | Return | Description |
|--------|--------|-------------|
| `IsError()` | `bool` | Returns true if Error != nil |
| `IsZero()` | `bool` | Returns true if Value == nil |
| `IsValid()` | `bool` | Returns true if !IsError() && !IsZero() |
| `Unwrap()` | `(any, error)` | Returns (Value, Error) tuple (Go idiom) |
| `MustValue()` | `any` | Returns Value or panics if Error != nil |
| `ValueOr(default)` | `any` | Returns Value if no error, else default |
| `IsTriggered(ctx)` | `bool` | Check if this variable triggered current execution |

**Usage Examples**:

```go
// Method 1: Check validity
value := hersh.WatchCall(...)
if value.IsValid() {
    fmt.Println(value.Value)
}

// Method 2: Unwrap (Go idiom)
val, err := value.Unwrap()
if err != nil {
    return err
}
fmt.Println(val)

// Method 3: MustValue (panic on error)
val := value.MustValue().(int) // Panics if error

// Method 4: ValueOr (default fallback)
val := value.ValueOr(0).(int)

// Method 5: Check trigger
if value.IsTriggered(ctx) {
    fmt.Println("This value triggered current execution!")
}
```

#### HershTick (WatchTick Return Type)

Time-based tick event with count tracking:

```go
type HershTick struct {
    Time      time.Time // Current tick timestamp
    TickCount int       // Total number of ticks occurred (starts from 1)
    VarName   string    // Name of the watched variable
}
```

**Methods**:

| Method | Return | Description |
|--------|--------|-------------|
| `IsZero()` | `bool` | Returns true if uninitialized (zero value) |
| `IsTriggered(ctx)` | `bool` | Check if this ticker triggered current execution |

**Usage Example**:

```go
tick := hutil.WatchTick("timer", 5*time.Second, ctx)

if !tick.IsZero() {
    fmt.Printf("Tick #%d at %s\n", tick.TickCount, tick.Time.Format("15:04:05"))

    if tick.IsTriggered(ctx) {
        // Only execute when this ticker actually triggered
        performPeriodicTask()
    }
}
```

### TriggeredSignal

Inspect which signals triggered the current execution:

```go
type TriggeredSignal struct {
    IsUserSig    bool     // Triggered by user message (SendMessage)
    IsWatcherSig bool     // Triggered by lifecycle event (InitRun, Stop, etc.)
    VarSigNames  []string // Names of Watch variables that triggered
}
```

**Methods**:

| Method | Return | Description |
|--------|--------|-------------|
| `HasTrigger()` | `bool` | Returns true if any signal was triggered |
| `HasVarTrigger(varName)` | `bool` | Check if specific variable triggered |

**Usage Example**:

```go
trigger := ctx.GetTriggeredSignal()

if trigger.IsUserSig {
    fmt.Println("User sent:", msg.Content)
}

if trigger.HasVarTrigger("price") {
    fmt.Println("Price changed!")
}

// Or use HershValue.IsTriggered()
price := hersh.WatchCall(...)
if price.IsTriggered(ctx) {
    fmt.Println("Price triggered this execution")
}
```

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
return hersh.NewCrashErr("recoverable error", cause)

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

| Method | Signature | Description |
|--------|-----------|-------------|
| `NewWatcher` | `(config, envVars, parentCtx) *Watcher` | Create Watcher with configuration and environment variables |
| `Manage` | `(fn, name) *CleanupBuilder` | Register managed function, returns `CleanupBuilder` |
| `.Cleanup` | `(cleanupFn) *CleanupBuilder` | Register cleanup function (called on Stop/Kill/Crash) |
| `Start` | `() error` | Start Watcher (blocks until `Ready` or error) |
| `Stop` | `() error` | Graceful stop with cleanup (blocks until complete) |
| `SendMessage` | `(content string) error` | Send message to trigger managed function |
| `GetState` | `() ManagerInnerState` | Get current state (`Ready`, `Running`, `Stopped`, etc.) |
| `GetLogger` | `() *Logger` | Access execution logs for inspection |
| `StartAPIServer` | `() (*WatcherAPIServer, error)` | Start HTTP API server (default port 8080) |

**NewWatcher Parameters**:

```go
func NewWatcher(
    config WatcherConfig,       // Configuration (use DefaultWatcherConfig())
    envVars map[string]string,  // Environment variables (immutable, can be nil)
    parentCtx context.Context,  // Parent context for auto-shutdown (can be nil)
) *Watcher
```

**Manage Function Signature**:

```go
type ManagedFunc func(msg *Message, ctx HershContext) error

func (w *Watcher) Manage(fn ManagedFunc, name string) *CleanupBuilder
```

**Cleanup Function Signature**:

```go
type CleanupFunc func(ctx HershContext)

func (cb *CleanupBuilder) Cleanup(fn CleanupFunc) *CleanupBuilder
```

### Logger Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `PrintSummary()` | - | Print execution summary to stdout |
| `GetReduceLog()` | `[]ReduceLogEntry` | State transitions (signals processed) |
| `GetEffectLog()` | `[]EffectLogEntry` | Effect execution logs (user messages) |
| `GetEffectResults()` | `[]EffectResult` | Effect execution results (NEW in v0.2.0) |
| `GetWatchErrorLog()` | `[]WatchErrorLogEntry` | Watch variable errors (computation failures) |
| `GetContextLog()` | `[]ContextValueLogEntry` | Context value changes (`SetValue`, `UpdateValue`) |
| `GetStateTransitionFaultLog()` | `[]StateTransitionFaultLogEntry` | Invalid state transitions (errors) |
| `GetRecentResults(count)` | `[]*EffectResult` | N most recent effect results |

**Log Entry Types**:

```go
type ReduceLogEntry struct {
    LogID     uint64
    Timestamp time.Time
    Action    ReduceAction // Signal processing action
}

type EffectLogEntry struct {
    LogID     uint64
    Timestamp time.Time
    Message   string // User log message from Log() function
}

type EffectResult struct {
    EffectType string
    Status     string // "success" or "error"
    Timestamp  time.Time
}

type WatchErrorLogEntry struct {
    LogID      uint64
    Timestamp  time.Time
    VarName    string
    ErrorPhase WatchErrorPhase // "get_compute_func" or "execute_compute_func"
    Error      error
}

type ContextValueLogEntry struct {
    LogID     uint64
    Timestamp time.Time
    Key       string
    OldValue  any
    NewValue  any
    Operation string // "initialized" or "updated"
}

type StateTransitionFaultLogEntry struct {
    LogID     uint64
    Timestamp time.Time
    FromState ManagerInnerState
    ToState   ManagerInnerState
    Reason    string
    Error     error
}
```

**Example**:

```go
logger := watcher.GetLogger()
logger.PrintSummary()

// Get specific logs
contextChanges := logger.GetContextLog()
watchErrors := logger.GetWatchErrorLog()
effectResults := logger.GetEffectResults()
```

### Reactive Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `WatchCall(getComputeFunc, varName, tick, ctx)` | `HershValue` | Polling-based reactive |
| `WatchFlow(getChannelFunc, varName, ctx)` | `HershValue` | Channel-based reactive |
| `WatchTick(varName, tickInterval, ctx)` | `HershTick` | Time-based ticker (hutil package) |
| `Memo(computeValue, memoName, ctx)` | `any` | Session-scoped cache |
| `ClearMemo(memoName, ctx)` | - | Clear cached value |

**Function Signatures**:

```go
// WatchCall - Polling-based reactive
func WatchCall(
    getComputationFunc func() (manager.VarUpdateFunc, error),
    varName string,
    tick time.Duration,
    ctx HershContext,
) HershValue

// VarUpdateFunc signature
type VarUpdateFunc func(prev HershValue) (next HershValue, changed bool, err error)

// WatchFlow - Channel-based reactive
func WatchFlow(
    getChannelFunc func(ctx context.Context) (<-chan FlowValue, error),
    varName string,
    ctx HershContext,
) HershValue

// FlowValue (internal type)
type FlowValue struct {
    V          any   // Value (passed to user if E == nil)
    E          error // Error (logged internally, converted to HershValue.Error)
    SkipSignal bool  // If true, skip sending VarSig (default false = send signal)
}

// WatchTick - Time-based ticker (hutil package)
func WatchTick(
    varName string,
    tickInterval time.Duration,
    ctx HershContext,
) HershTick

// Memo - Session-scoped cache
func Memo(
    computeValue func() any,
    memoName string,
    ctx HershContext,
) any

// ClearMemo - Clear cached value
func ClearMemo(memoName string, ctx HershContext)
```

### Logging Functions

| Function | Description |
|----------|-------------|
| `Log(message, ctx)` | Write message to effect log |
| `PrintWithLog(message, ctx)` | Print to stdout + log |

**Function Signatures**:

```go
// Log - Write message to effect log
func Log(s string, ctx HershContext)

// PrintWithLog - Print to stdout and log
func PrintWithLog(s string, ctx HershContext)
```

**Usage Example**:

```go
watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
    hersh.Log("Processing started", ctx)

    result := doWork()

    hersh.PrintWithLog(fmt.Sprintf("Result: %v", result), ctx)

    return nil
}, "worker")
```

### HershContext Interface

| Method | Signature | Description |
|--------|-----------|-------------|
| `WatcherID()` | `string` | Watcher's unique identifier |
| `Message()` | `*Message` | Current user message (`nil` if none) |
| `GetTriggeredSignal()` | `*TriggeredSignal` | Information about which signals triggered execution (NEW in v0.2.0) |
| `GetValue(key)` | `any` | Get stored value (returns actual value, not copy) |
| `SetValue(key, value)` | - | Set stored value (simple assignment) |
| `UpdateValue(key, updateFn)` | `any` | Atomic update with **deep copy** (thread-safe) |
| `GetEnv(key)` | `(string, bool)` | Get immutable environment variable |
| `GetWatcher()` | `any` | Watcher reference (as `any`) |

**Message Type**:

```go
type Message struct {
    Content    string    // Message content
    IsConsumed bool      // Internal flag
    ReceivedAt time.Time // When message was received
}

func (m *Message) String() string // Returns Content (or "" if nil)
```

**UpdateValue Example** (atomic, thread-safe):

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

### Types Reference

#### HershValue

```go
type HershValue struct {
    Value   any    // The actual value (may be nil)
    Error   error  // Error that occurred during computation (nil if no error)
    VarName string // Name of the watched variable
}

// Methods
func (hv HershValue) IsError() bool
func (hv HershValue) IsZero() bool
func (hv HershValue) IsValid() bool
func (hv HershValue) Unwrap() (any, error)
func (hv HershValue) MustValue() any
func (hv HershValue) ValueOr(defaultVal any) any
func (hv HershValue) IsTriggered(ctx HershContext) bool
```

#### HershTick

```go
type HershTick struct {
    Time      time.Time // Current tick timestamp
    TickCount int       // Total number of ticks occurred (starts from 1)
    VarName   string    // Name of the watched variable
}

// Methods
func (ht HershTick) IsZero() bool
func (ht HershTick) IsTriggered(ctx HershContext) bool
```

#### TriggeredSignal

```go
type TriggeredSignal struct {
    IsUserSig    bool     // Triggered by user message
    IsWatcherSig bool     // Triggered by lifecycle event
    VarSigNames  []string // Names of Watch variables that triggered
}

// Methods
func (ts *TriggeredSignal) HasTrigger() bool
func (ts *TriggeredSignal) HasVarTrigger(varName string) bool
```

#### FlowValue

```go
type FlowValue struct {
    V          any   // Value (passed to user if E == nil)
    E          error // Error (logged internally, never exposed to user)
    SkipSignal bool  // If true, skip sending VarSig (default false = send signal)
}
```

### Error Constructors

| Constructor | Signature | Lifecycle | Cleanup? | Recovery? |
|-------------|-----------|-----------|----------|-----------|
| `NewStopErr` | `(reason string) *StopError` | Stopped (permanent) | ✅ Yes | ❌ No |
| `NewKillErr` | `(reason string) *KillError` | Killed (permanent) | ❌ No | ❌ No |
| `NewCrashErr` | `(reason string, cause error) *CrashError` | Crashed → WaitRecover | ✅ Yes | ✅ Yes (if configured) |

**Error Types**:

```go
type StopError struct {
    Reason string
}

type KillError struct {
    Reason string
}

type CrashError struct {
    Reason string
    Cause  error // Underlying error that caused the crash
}
```

### Configuration Types

#### WatcherConfig

```go
type WatcherConfig struct {
    DefaultTimeout     time.Duration  // Managed function timeout (default: 1min)
    RecoveryPolicy     RecoveryPolicy // Fault tolerance policy
    ServerPort         int            // API server port (default: 8080, 0 to disable)
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
    LightweightRetryDelays []time.Duration // Delays for failures <MinConsecutiveFailures
}

func DefaultRecoveryPolicy() RecoveryPolicy
```

**Behavior**:

- **Failures < MinConsecutiveFailures**: Lightweight retry with delays `[15s, 30s, 60s]` → `Ready`
- **Failures ≥ MinConsecutiveFailures**: Heavy retry with exponential backoff (`5s → 10s → 20s → ...`) → `WaitRecover`
- **Failures ≥ MaxConsecutiveFailures**: Permanent `Crashed` state (no more retries)

**Example**:

```go
config := hersh.WatcherConfig{
    DefaultTimeout: 30 * time.Second,
    RecoveryPolicy: hersh.RecoveryPolicy{
        MinConsecutiveFailures: 2,
        MaxConsecutiveFailures: 5,
        BaseRetryDelay:         3 * time.Second,
        MaxRetryDelay:          1 * time.Minute,
        LightweightRetryDelays: []time.Duration{
            10 * time.Second,
            20 * time.Second,
        },
    },
    ServerPort: 8080,
}
watcher := hersh.NewWatcher(config, nil, nil)
```

### State Machine Constants

```go
const (
    StateReady       ManagerInnerState = iota // Idle, waiting for triggers
    StateRunning                              // Executing managed function
    StateInitRun                              // First execution (initialization)
    StateStopped                              // Gracefully stopped (permanent)
    StateKilled                               // Force killed (permanent)
    StateCrashed                              // Crashed after max retries (permanent)
    StateWaitRecover                          // Waiting to retry after crash
)

const (
    PriorityManagerInner SignalPriority = 0 // WatcherSig (highest priority)
    PriorityUser         SignalPriority = 1 // UserSig (medium priority)
    PriorityVar          SignalPriority = 2 // VarSig (lowest priority)
)
```

## WatcherAPI (HTTP Endpoints)

### Control & State

```bash
# Get current state and basic information
GET /watcher/status
Response: {
  "state": "Ready",
  "isRunning": true,
  "watcherID": "watcher-123",
  "uptime": "5m30s",
  "lastUpdate": "2024-01-15T10:30:00Z"
}

# Send message to trigger managed function
POST /watcher/message
Content-Type: application/json
Body: {"content": "your-command"}
Response: {"status": "message sent"}

# Get Watcher configuration
GET /watcher/config
Response: {
  "config": {
    "serverPort": 8080,
    "signalChanCapacity": 50000,
    "maxLogEntries": 50000,
    "maxMemoEntries": 1000
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Monitoring

```bash
# Get watched variables (WatchCall/WatchFlow current values)
GET /watcher/watching
Response: {
  "watchedVars": ["counter", "events", "price"],
  "count": 3,
  "timestamp": "2024-01-15T10:30:00Z"
}

# Get memo cache contents
GET /watcher/memoCache
Response: {
  "entries": {
    "apiClient": {"name": "client"},
    "dbConnection": {...}
  },
  "count": 2,
  "timestamp": "2024-01-15T10:30:00Z"
}

# Get HershContext variable state (GetValue/SetValue)
GET /watcher/varState
Response: {
  "variables": {
    "totalRuns": 42,
    "stats": {"count": 100}
  },
  "count": 2,
  "timestamp": "2024-01-15T10:30:00Z"
}

# Get signal queue status
GET /watcher/signals
Response: {
  "varSigCount": 5,
  "userSigCount": 2,
  "watcherSigCount": 0,
  "totalPending": 7,
  "recentSignals": [
    {"type": "var", "content": "VarSig{varName=counter}", "createdAt": "..."}
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Logs

```bash
# Get logs by type with optional limit
GET /watcher/logs?type={TYPE}&limit={N}

# Available types:
# - reduce: State transition logs
# - effect: Effect execution logs (user messages)
# - effect_result: Effect execution results (NEW in v0.2.0)
# - watch_error: Watch errors
# - context: Context value changes
# - state_fault: State transition faults
# - all: All logs combined

# Examples:
GET /watcher/logs?type=reduce&limit=100
GET /watcher/logs?type=effect_result&limit=50
GET /watcher/logs?type=all&limit=200

Response: {
  "reduceLogs": [...],
  "effectLogs": [...],
  "effectResults": [...],  // NEW in v0.2.0
  "watchErrorLogs": [...],
  "contextLogs": [...],
  "stateFaultLogs": [...]
}
```

**Starting API Server**:

```go
// Option 1: Start during Watcher creation (auto-starts)
config := hersh.DefaultWatcherConfig()
config.ServerPort = 8080 // Set to 0 to disable
watcher := hersh.NewWatcher(config, nil, nil)

// Option 2: Start manually (non-blocking)
apiServer, err := watcher.StartAPIServer()
if err != nil {
    log.Fatal(err)
}

// Query from external process
// curl http://localhost:8080/watcher/status
// curl http://localhost:8080/watcher/logs?type=effect_result&limit=100
```

## Migration Guide (v0.2.x → v0.3.0)

### Breaking Changes

This release focuses on making the API more declarative and improving signal tracking capabilities.

#### 1. WatchFlow Now Accepts Channel Factory Function

**Before (v0.2.x)**:

```go
eventChan := make(chan any, 10)
event := hersh.WatchFlow(eventChan, "events", ctx)
// Manual channel cleanup required
```

**After (v0.3.0)**:

```go
event := hersh.WatchFlow(
    func(flowCtx context.Context) (<-chan shared.FlowValue, error) {
        ch := make(chan shared.FlowValue, 10)
        go func() {
            defer close(ch)
            // Framework handles cleanup via flowCtx
            for {
                select {
                case <-flowCtx.Done():
                    return
                case data := <-source:
                    ch <- shared.FlowValue{V: data, E: nil}
                }
            }
        }()
        return ch, nil
    },
    "events", ctx,
)
```

**Benefits**:
- Automatic channel cleanup through context
- More declarative usage pattern
- Framework-managed goroutine lifecycle

#### 2. Watch Functions Now Return (any, error) Tuple

**Before (v0.2.x)**:

```go
counter := hersh.WatchCall(...) // returns HershValue
if counter.IsValid() {
    val := counter.Value.(int)
}
```

**After (v0.3.0)**:

```go
counter, err := hersh.WatchCall(...) // returns (any, error)
if err != nil {
    log.Printf("Error: %v", err)
} else if counter != nil {
    val := counter.(int)
}
```

**Benefits**:
- Explicit error handling
- Better error propagation
- More idiomatic Go pattern

#### 3. Added TriggeredSignal to HershContext

**New Feature (v0.3.0)**:

```go
// Check which signal triggered current execution
trigger := ctx.TriggeredSignal()

if trigger.IsUserSig {
    fmt.Println("User sent:", msg.Content)
}

if trigger.HasVarTrigger("price") {
    fmt.Println("Price variable triggered this execution")
}
```

**Benefits**:
- Precise control flow based on trigger source
- Enhanced logging with trigger information
- Better debugging and monitoring capabilities

### Internal Changes

#### Manager VarState Now Stores (any, error)

The internal state management now properly stores both values and errors:

```go
// Internal: VarState structure
type VarState struct {
    Value any
    Error error
    // ...
}
```

This enables better error tracking and more reliable reactive behavior.

#### Manager Tracks lastTriggeredSignal

The Manager now maintains `lastTriggeredSignal` state for:
- Monitoring via API endpoints
- Enhanced logging
- Better debugging support

### New Convenience Functions

#### Tick() Function for Easy Timer Integration

```go
import "github.com/HershyOrg/hersh/hutil"

tick, err := hutil.Tick("timer", 5*time.Second, ctx)
if err != nil {
    log.Printf("Error: %v", err)
} else if tick != nil {
    // tick contains {time.Time, tickCount int}
    fmt.Printf("Tick #%d at %s\n", tick.TickCount, tick.Time.Format("15:04:05"))
}
```

**Benefits**:
- Simplifies periodic task scheduling
- Automatic tick counting
- Built on new WatchFlow pattern

### Documentation Updates

- Updated README.md with new API patterns
- Enhanced examples in demo/long/main.go
- Added migration guide for v0.2.x users

### Recommended Migration Steps

1. **Update WatchFlow calls** to use channel factory functions
2. **Update error handling** to use (any, error) tuple pattern
3. **Leverage TriggeredSignal** for better control flow
4. **Test thoroughly** as these are breaking changes
5. **Review demo/long/main.go** for updated usage patterns

## Migration Guide (v0.3.0 → v0.3.1)

### Major Paradigm Shift

**v0.3.0**: Change detection paradigm - signals triggered when values changed
**v0.3.1**: Signal control paradigm - explicit control over signal triggering

The concept of "change" has been entirely removed from Hersh. The only trigger criterion is now "was a signal sent or not."

### Breaking Changes

#### 1. WatchCall Signature Changed

**Before (v0.3.0):**

```go
hersh.WatchCall(
    func() (manager.VarUpdateFunc, error) {
        return func(prev shared.HershValue) (shared.HershValue, bool, error) {
            value := fetchValue()
            changed := prev.Value != value
            return shared.HershValue{Value: value}, changed, nil
        }, nil
    },
    "varName", tick, ctx,
)
```

**After (v0.3.1):**

```go
hersh.WatchCall(
    func() (manager.VarUpdateFunc, bool, error) {
        return func(prev shared.HershValue) (shared.HershValue, error) {
            value := fetchValue()
            return shared.HershValue{Value: value}, nil
        }, false, nil  // false = don't skip signal (send it)
    },
    "varName", tick, ctx,
)
```

#### 2. FlowValue Structure Updated

**Before (v0.3.0):**

```go
ch <- shared.FlowValue{
    V: value,
    E: nil,
}
```

**After (v0.3.1):**

```go
ch <- shared.FlowValue{
    V:          value,
    E:          nil,
    SkipSignal: false,  // false = send signal (default behavior)
}
```

#### 3. VarUpdateFunc Signature Simplified

**Before (v0.3.0):**

```go
type VarUpdateFunc func(prev shared.HershValue) (next shared.HershValue, hasChanged bool, err error)
```

**After (v0.3.1):**

```go
type VarUpdateFunc func(prev shared.HershValue) (next shared.HershValue, err error)
```

### Other Changes

#### 4. Start() No Longer Blocks

- **v0.3.0**: Start() blocked until InitRun → Ready transition completed
- **v0.3.1**: Start() returns immediately after launching the reactive engine
- Impact: If you relied on Start() blocking to ensure Ready state, adjust your code

#### 5. Field Rename

- **ComputedTime** → **ReceivedTime** in VarSig structure for semantic accuracy

### Migration Steps

1. **Update WatchCall implementations:**
   - Change function signature from `func() (VarUpdateFunc, error)` to `func() (VarUpdateFunc, bool, error)`
   - Remove `hasChanged` boolean from VarUpdateFunc returns
   - Add `skipSignal` boolean (use `false` for normal behavior)

2. **Update WatchFlow implementations:**
   - Add `SkipSignal: false` to FlowValue when you want to trigger execution
   - Set `SkipSignal: true` only when you want to skip triggering

3. **Update custom VarUpdateFunc implementations:**
   - Remove the `hasChanged bool` return value
   - Signal control is now in the outer function

4. **Review Start() usage:**
   - If you depended on Start() blocking, add appropriate synchronization

5. **Update field references:**
   - Replace `ComputedTime` with `ReceivedTime` where used

### Key Concept: SkipSignal

The `SkipSignal` field uses inverted logic for better ergonomics:
- **Default (`false`)**: Send signal, trigger execution (common case)
- **Set to `true`**: Skip signal, don't trigger execution (special case)

This design makes the default behavior (sending signals) require no extra code.

## Examples

### Example 1: Complete Feature Demo

See [Quick Start](#quick-start) section above for a comprehensive example using all features.

### Example 2: Recovery Policy Demo

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
        return hersh.NewCrashErr(fmt.Sprintf("simulated failure %d", count+1), nil)
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

### Example 3: Real-time Event Pipeline

Demonstrates WatchFlow with processing pipeline:

```go
watcher := hersh.NewWatcher(hersh.DefaultWatcherConfig(), nil, nil)

watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
    // Watch incoming events
    event := hersh.WatchFlow(
        func(flowCtx context.Context) (<-chan shared.FlowValue, error) {
            ch := make(chan shared.FlowValue, 100)

            go func() {
                defer close(ch)

                for i := 1; i <= 5; i++ {
                    select {
                    case <-flowCtx.Done():
                        return
                    case <-time.After(100 * time.Millisecond):
                        ch <- shared.FlowValue{
                            V: fmt.Sprintf("event%d", i),
                            E: nil,
                        }
                    }
                }
            }()

            return ch, nil
        },
        "eventStream", ctx,
    )

    if event.IsValid() {
        // Process event
        processed := fmt.Sprintf("processed_%v", event.Value)

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
        fmt.Printf("Event: %v → %s (total: %d)\n", event.Value, processed, stats["total"])
    }

    // Handle control messages
    if msg != nil && msg.Content == "status" {
        stats := ctx.GetValue("stats")
        fmt.Printf("Pipeline stats: %+v\n", stats)
    }

    return nil
}, "pipeline")

watcher.Start()

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

### Example 4: WatchTick Usage

Demonstrates time-based periodic tasks:

```go
import "github.com/HershyOrg/hersh/hutil"

watcher := hersh.NewWatcher(hersh.DefaultWatcherConfig(), nil, nil)

watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
    // Periodic stats collection every 5 seconds
    tick := hutil.WatchTick("stats_ticker", 5*time.Second, ctx)

    if !tick.IsZero() {
        fmt.Printf("📊 Tick #%d at %s\n", tick.TickCount, tick.Time.Format("15:04:05"))

        // Check if this tick triggered current execution
        if tick.IsTriggered(ctx) {
            // Perform periodic task
            stats := collectStats()
            ctx.SetValue("lastStats", stats)
            hersh.Log(fmt.Sprintf("Stats collected: %+v", stats), ctx)
        }
    }

    return nil
}, "ticker-app")

watcher.Start()
time.Sleep(16 * time.Second) // Wait for 3 ticks
watcher.Stop()
```

**Output**:

```
📊 Tick #1 at 10:00:00
📊 Tick #2 at 10:00:05
📊 Tick #3 at 10:00:10
```

### Example 5: TriggeredSignal Usage

Demonstrates conditional execution based on trigger source:

```go
watcher := hersh.NewWatcher(hersh.DefaultWatcherConfig(), nil, nil)

watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
    // Monitor multiple variables
    price := hersh.WatchCall(..., "price", 1*time.Second, ctx)
    volume := hersh.WatchCall(..., "volume", 1*time.Second, ctx)

    // Check what triggered this execution
    trigger := ctx.GetTriggeredSignal()

    if trigger.IsUserSig {
        fmt.Println("🔔 User command:", msg.Content)
        // Handle user commands
    }

    if trigger.HasVarTrigger("price") {
        fmt.Printf("💰 Price changed: %v\n", price.Value)
        // React to price changes
    }

    if trigger.HasVarTrigger("volume") {
        fmt.Printf("📊 Volume changed: %v\n", volume.Value)
        // React to volume changes
    }

    // Or use convenience method
    if price.IsTriggered(ctx) {
        fmt.Println("Price triggered this execution")
    }

    return nil
}, "trigger-demo")

watcher.Start()
watcher.SendMessage("check")
time.Sleep(2 * time.Second)
watcher.Stop()
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
├── log_and_print.go     # Log, PrintWithLog functions
├── types.go             # Public types (re-exports from shared/)
├── manager/             # Internal Manager (Reducer-Effect pattern)
│   ├── manager.go       # Manager orchestrator
│   ├── reducer.go       # Pure state transitions
│   ├── effect_handler.go # Effect execution
│   ├── logger.go        # Execution logging
│   ├── signal.go        # Signal priority queue
│   ├── state.go         # State management (VarState, UserState, ManagerState)
│   └── effect.go        # Effect definitions
├── hctx/                # HershContext implementation
│   └── context.go
├── hutil/               # Helper utilities (NEW in v0.2.0)
│   └── watchtick.go     # WatchTick helper
├── shared/              # Shared types and errors
│   ├── types.go         # Core types (HershValue, HershTick, TriggeredSignal, etc.)
│   ├── errors.go        # Control flow errors
│   └── copy.go          # Deep copy utilities
├── api/                 # WatcherAPI HTTP handlers
│   ├── handlers.go      # HTTP request handlers
│   └── types.go         # API response types
└── demo/                # Usage examples
    ├── example_simple.go
    ├── example_watchcall.go
    └── example_trading.go
```

## Changelog

### v0.3.1 (2025-02-25)

**Breaking Changes - Signal Control Paradigm**

- **Removed change detection**: The concept of "change" has been entirely removed from Hersh
- **Explicit signal control**: Replaced implicit change detection with explicit signal control via `SkipSignal`
- **WatchCall signature changed**: Added `bool` parameter for signal control
- **VarUpdateFunc simplified**: Removed `hasChanged bool` return value
- **FlowValue enhanced**: Added `SkipSignal` field for channel-based signal control

**Other Changes**

- **Start() behavior**: No longer blocks until Ready state is reached
- **Field rename**: `ComputedTime` → `ReceivedTime` in VarSig for semantic accuracy

See the [Migration Guide](#migration-guide-v030--v031) for detailed upgrade instructions.

### v0.3.0 (2025-02-22)

**Major Breaking Changes - Better Declarative API**

- Renamed `VarSig.SignalTime` → `VarSig.ReceivedTime`
- Updated `WatchFlow` to use channel factory functions
- Enhanced `TriggeredSignal` for better control flow
- Added `IsTriggered()` method to `HershValue` and `HershTick`

## Real-World Usage

Hersh powers **[Hershy](https://github.com/HershyOrg/hershy)**, a container orchestration system that manages Docker containers with reactive state management.

**Production examples**:

- [simple-counter](https://github.com/HershyOrg/hershy/tree/main/examples/simple-counter): Basic counter with WatcherAPI control
- [trading-long](https://github.com/HershyOrg/hershy/tree/main/examples/trading-long): Real-time trading simulator
- [watcher-server](https://github.com/HershyOrg/hershy/tree/main/examples/watcher-server): HTTP server with persistent state

## Links

- **Repository**: <https://github.com/HershyOrg/hersh>
- **Documentation**: <https://pkg.go.dev/github.com/HershyOrg/hersh>
- **Issues**: <https://github.com/HershyOrg/hersh/issues>
- **Hershy (Container Orchestration)**: <https://github.com/HershyOrg/hershy>
