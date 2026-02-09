# Hersh

**Reactive framework for Go with managed execution and monitoring**

Hersh is a lightweight reactive framework that provides deterministic state management through a managed execution model. It features reactive variables (polling and channel-based), session-scoped caching, persistent context storage, and HTTP API control.

## Features

- 🎯 **Managed Execution**: Single managed function triggered by messages or reactive changes
- 🔄 **WatchCall**: Polling-based reactive variables that trigger re-execution on changes
- 📡 **WatchFlow**: Channel-based reactive variables for event-driven patterns
- 💾 **Memo**: Session-scoped caching for expensive computations
- 📦 **HershContext**: Persistent key-value storage across executions
- 🌐 **WatcherAPI**: HTTP server for external control and state inspection
- 🛡️ **Fault Tolerance**: Built-in recovery policies and graceful error handling
- 📊 **Execution Logging**: Track execution count, errors, and performance metrics

## Installation

```bash
go get github.com/HershyOrg/hersh@v0.2.0
```

## Quick Start

### 1. Basic Example: Managed Function

```go
package main

import (
    "fmt"
    "time"
    "github.com/HershyOrg/hersh"
)

func main() {
    // Create Watcher with default config
    config := hersh.DefaultWatcherConfig()
    watcher := hersh.NewWatcher(config, nil, nil)

    // Define managed function (executes on Start + each message/reactive trigger)
    counter := 0
    managedFunc := func(msg *hersh.Message, ctx hersh.HershContext) error {
        counter++
        fmt.Printf("[Execution %d]\n", counter)

        // Handle messages
        if msg != nil {
            fmt.Printf("Received: %s\n", msg.Content)
            if msg.Content == "stop" {
                return hersh.NewStopErr("user requested stop")
            }
        }
        return nil
    }

    // Register managed function with cleanup
    watcher.Manage(managedFunc, "myApp").Cleanup(func(ctx hersh.HershContext) {
        fmt.Println("Cleanup executed")
    })

    // Start watcher (triggers first execution)
    watcher.Start()

    // Send messages to trigger additional executions
    watcher.SendMessage("hello")
    time.Sleep(100 * time.Millisecond)

    watcher.SendMessage("stop")
    time.Sleep(100 * time.Millisecond)

    // Stop watcher
    watcher.Stop()
}
```

### 2. WatchCall: Polling-Based Reactive

Poll external values and trigger re-execution on changes:

```go
import (
    "github.com/HershyOrg/hersh"
    "github.com/HershyOrg/hersh/manager"
)

externalCounter := 0

managedFunc := func(msg *hersh.Message, ctx hersh.HershContext) error {
    // Poll external value every 300ms, re-execute when changed
    watchedValue := hersh.WatchCall(
        func() (manager.VarUpdateFunc, error) {
            return func(prev any) (any, bool, error) {
                currentValue := externalCounter
                externalCounter++

                // Detect change
                if prev == nil {
                    return currentValue, true, nil // First call
                }

                changed := prev.(int) != currentValue
                return currentValue, changed, nil
            }, nil
        },
        "externalCounter", // Variable name
        300*time.Millisecond, // Poll interval
        ctx,
    )

    if watchedValue != nil {
        fmt.Printf("Value changed to: %d\n", watchedValue.(int))
    }
    return nil
}
```

**Key Points:**
- `getComputationFunc` returns a `VarUpdateFunc` on each tick
- `VarUpdateFunc` receives `prev` value and returns `(next, changed, error)`
- Re-execution only triggers when `changed = true`
- Returns `nil` on first call (not initialized yet)

### 3. WatchFlow: Channel-Based Reactive

Monitor channels and trigger re-execution on new values:

```go
// Create channel (can be external)
var eventChan = make(chan any, 10)
    // Send values to channel (from goroutine or external source)
go func() {
    eventChan <- "event1"
    time.Sleep(100 * time.Millisecond)
    eventChan <- "event2"
}()

managedFunc := func(msg *hersh.Message, ctx hersh.HershContext) error {

    // Watch channel for new values
    latestEvent := hersh.WatchFlow(eventChan, "eventStream", ctx)

    if latestEvent != nil {
        fmt.Printf("New event: %v\n", latestEvent)
    }


    return nil
}
```

**Key Points:**
- Monitors a channel and emits signals on new values
- Re-execution triggers automatically when channel receives data
- Returns `nil` until first value received
- Automatically stops when channel closed or Watcher stopped

### 4. Memo: Session-Scoped Caching

Cache expensive computations within a Watcher session:

```go
managedFunc := func(msg *hersh.Message, ctx hersh.HershContext) error {
    // Expensive computation (cached for entire session)
    client := hersh.Memo(func() any {
        fmt.Println("Creating expensive client...")
        time.Sleep(100 * time.Millisecond)
        return &ExpensiveClient{}
    }, "apiClient", ctx).(*ExpensiveClient)

    // First call computes, subsequent calls return cached value
    fmt.Printf("Using client: %v\n", client)

    // Clear cache if needed
    if msg != nil && msg.Content == "refresh" {
        hersh.ClearMemo("apiClient", ctx)
    }

    return nil
}
```

**Key Points:**
- Computed once per session (until `ClearMemo` or Watcher restart)
- Thread-safe with `LoadOrStore` semantics
- Does NOT trigger re-execution
- Useful for initialization (DB connections, HTTP clients, etc.)

### 5. HershContext: Persistent State Storage

Store values that persist across executions:

```go
managedFunc := func(msg *hersh.Message, ctx hersh.HershContext) error {
    // Get persistent value (survives across executions)
    totalRuns := ctx.GetValue("totalRuns")
    if totalRuns == nil {
        totalRuns = 0
    }

    // Update persistent state
    newTotal := totalRuns.(int) + 1
    ctx.SetValue("totalRuns", newTotal)
    fmt.Printf("Total runs: %d\n", newTotal)

    // UpdateValue with function (thread-safe with deep copy)
    ctx.UpdateValue("counter", func(current any) any {
        if current == nil {
            return 1
        }
        return current.(int) + 1
    })

    // Access environment variables (immutable, set at creation)
    if apiKey, ok := ctx.GetEnv("API_KEY"); ok {
        fmt.Printf("Using API key: %s\n", apiKey)
    }

    return nil
}

// Set environment variables at creation
watcher := hersh.NewWatcher(config, map[string]string{
    "API_KEY": "secret",
    "ENV": "production",
}, nil)
```

**Key Points:**
- **GetValue/SetValue**: Simple get/set for any type
- **UpdateValue**: Thread-safe update with deep copy
- **GetEnv**: Immutable environment variables (set at Watcher creation)
- Values persist across all executions within a session
- Separate from Memo (Memo is for cached computations, HershContext for state)

### 6. WatcherAPI: HTTP Control

Enable HTTP API for external control:

```go
watcher.StartAPIServer() // Defaults to port 8080
// or
watcher.StartAPIServer() // Port configured in WatcherConfig
```

**Available Endpoints:**

```bash
# Get Watcher state (Ready, Running, Stopped, etc)
curl http://localhost:8080/watcher/status

# Get detailed state (execution count, error count, uptime)
curl http://localhost:8080/watcher/state

# Send message to trigger managed function
curl -X POST http://localhost:8080/watcher/message \
  -H "Content-Type: application/json" \
  -d '{"content":"your-command"}'

# Get environment variables
curl http://localhost:8080/watcher/vars

# Get watched variables state (WatchCall, WatchFlow values)
curl http://localhost:8080/watcher/watching

# Get memo cache contents
curl http://localhost:8080/watcher/memoCache

# Get HershContext variable state (GetValue/SetValue)
curl http://localhost:8080/watcher/varState

# Get Watcher configuration
curl http://localhost:8080/watcher/config
```

## Core Concepts

### Execution Model

Hersh uses a **single managed function** that executes:

1. **On Start**: Initial execution when `watcher.Start()` is called
2. **On SendMessage**: When `watcher.SendMessage(content)` or WatcherAPI `/message` is called
3. **On WatchCall Change**: When polled values change (every `tick` interval)
4. **On WatchFlow Event**: When channel receives new values

```
Start() → InitRun (first execution) → Ready → [trigger] → Running → Ready → ...
```

### Error Handling & Lifecycle Control

Return special errors to control Watcher lifecycle:

```go
// Graceful stop (cleanup executes)
return hersh.NewStopErr("user requested stop")

// Force kill (immediate shutdown, no cleanup)
return hersh.NewKillErr("critical error")

// Crash (triggers recovery if configured)
return hersh.NewCrashErr("unexpected error")

// Normal error (logs but continues)
return fmt.Errorf("non-fatal error")
```

### State Lifecycle

```
NotRun → InitRun → Ready → [trigger] → Running → Ready
                          ↓
                      Stopped/Killed
                          ↓
                      Crashed → WaitRecover (if recovery enabled)
```

- **NotRun**: Before Start()
- **InitRun**: First execution (initialization)
- **Ready**: Idle, waiting for triggers
- **Running**: Executing managed function
- **Stopped**: Graceful shutdown (via StopErr or Stop())
- **Killed**: Force shutdown (via KillErr)
- **Crashed**: Error occurred, may recover
- **WaitRecover**: Waiting to retry after crash

### Signal Priority

Internal signal processing order:

```
PriorityManagerInner > PriorityUser > PriorityVar
```

- **ManagerInner**: Lifecycle signals (stop, kill, crash)
- **User**: `SendMessage()` and WatcherAPI `/message`
- **Var**: WatchCall/WatchFlow reactive triggers

This ensures lifecycle commands always take precedence over user messages and reactive triggers.

## Configuration

### WatcherConfig

```go
config := hersh.WatcherConfig{
    DefaultTimeout:     5 * time.Second,  // Timeout for managed function
    RecoveryPolicy:     hersh.DefaultRecoveryPolicy(), // or custom
    SignalChanCapacity: 100,              // Internal signal queue size
    MaxLogEntries:      1000,             // Max log history
    MaxMemoEntries:     100,              // Max memo cache size
}

watcher := hersh.NewWatcher(config, envVars, parentCtx)
```

### Recovery Policy

Configure automatic recovery on crashes:

```go
config.RecoveryPolicy = &hersh.RecoveryPolicy{
    MaxRetries:    3,               // Max retry attempts
    RetryDelay:    1 * time.Second, // Initial delay
    BackoffFactor: 2.0,             // Exponential backoff (1s, 2s, 4s)
}
```

**Behavior:**
- On `CrashErr`: Waits `RetryDelay`, then retries managed function
- On `StopErr`/`KillErr`: No recovery, immediate stop
- After `MaxRetries`: Enters permanent `Crashed` state

### Parent Context (Auto-Shutdown)

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

watcher := hersh.NewWatcher(config, nil, ctx)
// Watcher automatically stops when ctx is cancelled
```

## Complete API Reference

### Watcher Methods

```go
// Lifecycle
func NewWatcher(config WatcherConfig, envVars map[string]string, parentCtx context.Context) *Watcher
func (w *Watcher) Manage(fn ManagedFunc, name string) *CleanupBuilder
func (cb *CleanupBuilder) Cleanup(cleanupFn func(ctx HershContext)) *Watcher
func (w *Watcher) Start() error
func (w *Watcher) Stop() error

// Control
func (w *Watcher) SendMessage(content string) error
func (w *Watcher) GetState() ManagerInnerState
func (w *Watcher) GetLogger() *Logger

// API Server
func (w *Watcher) StartAPIServer() (*WatcherAPIServer, error)
```

### Reactive Functions

```go
// Polling-based reactive
func WatchCall(
    getComputationFunc func() (manager.VarUpdateFunc, error),
    varName string,
    tick time.Duration,
    ctx HershContext,
) any

// Channel-based reactive
func WatchFlow(
    sourceChan <-chan any,
    varName string,
    ctx HershContext,
) any
```

### Caching

```go
func Memo(computeValue func() any, memoName string, ctx HershContext) any
func ClearMemo(memoName string, ctx HershContext)
```

### HershContext Interface

```go
// Persistent storage (across executions)
GetValue(key string) any
SetValue(key string, value any)
UpdateValue(key string, updateFn func(current any) any) any

// Environment variables (immutable)
GetEnv(key string) (string, bool)

// Context info
WatcherID() string
Message() *Message
GetWatcher() any
```

### Error Constructors

```go
func NewStopErr(reason string) error   // Graceful stop
func NewKillErr(reason string) error   // Force kill
func NewCrashErr(reason string) error  // Crash with recovery
```

### Helper Functions

```go
func DefaultWatcherConfig() WatcherConfig
func DefaultRecoveryPolicy() *RecoveryPolicy
```

## Examples

See the [demo/](./demo/) directory for complete examples:

- **[example_simple.go](./demo/example_simple.go)**: Basic managed function with Memo and HershContext
- **[example_watchcall.go](./demo/example_watchcall.go)**: Polling-based reactive with WatchCall
- **[example_trading.go](./demo/example_trading.go)**: Real-time trading simulator with Binance WebSocket

### Running Examples

```bash
# Simple example (Memo, HershContext, SendMessage)
go run demo/example_simple.go

# WatchCall reactive polling example
go run demo/example_watchcall.go

# Trading simulation (requires Binance WebSocket connection)
go run demo/example_trading.go demo/market_client.go
```

## Testing

Hersh includes 80+ tests covering all major features:

```bash
# Run all tests
go test ./... -v

# Run with race detector
go test ./... -race

# Run with coverage
go test ./... -cover

# Run specific test
go test -run TestWatchCall_BasicFunctionality -v
go test -run TestWatchFlow_ChannelBased -v
go test -run TestMemo_BasicCaching -v
```

## Architecture

### Package Structure

```
github.com/HershyOrg/hersh/
├── watcher.go              # Core Watcher (lifecycle, Start, Stop)
├── watcher_api.go          # HTTP API server
├── watch.go                # WatchCall, WatchFlow reactive functions
├── memo.go                 # Memo caching
├── types.go                # Public types (re-exports from shared/)
├── manager/                # Internal Manager (Reducer-Effect pattern)
│   ├── manager.go          # Manager orchestrator
│   ├── reducer.go          # Pure state transitions
│   ├── effect.go           # Side effects
│   ├── effect_handler.go   # Effect execution
│   ├── signal.go           # Signal priority queue
│   ├── state.go            # Internal state management
│   └── logger.go           # Execution logging
├── hctx/                   # HershContext implementation
│   └── context.go
├── api/                    # WatcherAPI HTTP handlers
│   ├── handlers.go
│   └── types.go
├── hutil/                  # Utilities (ticker)
│   └── ticker.go
├── shared/                 # Shared types and errors
│   ├── types.go
│   ├── errors.go
│   └── copy.go
├── demo/                   # Usage examples
│   ├── example_simple.go
│   ├── example_watchcall.go
│   ├── example_trading.go
│   └── market_client.go
└── test/                   # Integration tests
    ├── concurrent_watch_test.go
    ├── recovery_test.go
    └── manager_integration_test.go
```

### Internal: Reducer-Effect Pattern

Hersh's Manager internally uses the Reducer-Effect pattern for deterministic state management:

1. **Pure Reducer**: Computes next state from current state + signal (no side effects)
2. **Effect Generation**: Reducer returns effects to execute
3. **Effect Handler**: Executes effects (runs managed function, updates state)
4. **Supervisor Loop**: Processes signals with priority queue

This enables:
- ✅ Deterministic state transitions (testable, reproducible)
- ✅ Observable state changes (all transitions logged)
- ✅ Fault tolerance (recovery policies, graceful shutdown)
- ✅ Testable pure functions (reducer tests without IO)

## Common Patterns

### Pattern 1: API Client with Caching

```go
managedFunc := func(msg *hersh.Message, ctx hersh.HershContext) error {
    // Initialize once per session
    client := hersh.Memo(func() any {
        return api.NewClient(ctx.GetEnv("API_URL"))
    }, "apiClient", ctx).(*api.Client)

    // Handle commands
    if msg != nil {
        switch msg.Content {
        case "fetch":
            data := client.Fetch()
            ctx.SetValue("lastData", data)
        case "refresh":
            hersh.ClearMemo("apiClient", ctx)
        }
    }
    return nil
}
```

### Pattern 2: Reactive Data Pipeline

```go
managedFunc := func(msg *hersh.Message, ctx hersh.HershContext) error {
    // Poll external API every 5 seconds
    rawData := hersh.WatchCall(
        func() (manager.VarUpdateFunc, error) {
            return func(prev any) (any, bool, error) {
                data := fetchExternalData()
                return data, !reflect.DeepEqual(prev, data), nil
            }, nil
        },
        "rawData",
        5*time.Second,
        ctx,
    )

    if rawData != nil {
        // Process data
        processed := processData(rawData)
        ctx.SetValue("processed", processed)
    }
    return nil
}
```

### Pattern 3: Event-Driven Processing

```go
eventChan := make(chan any, 100)

managedFunc := func(msg *hersh.Message, ctx hersh.HershContext) error {
    // Watch channel for events
    event := hersh.WatchFlow(eventChan, "events", ctx)

    if event != nil {
        // Process event
        handleEvent(event)

        // Track statistics
        count := ctx.GetValue("eventCount")
        if count == nil {
            count = 0
        }
        ctx.SetValue("eventCount", count.(int)+1)
    }
    return nil
}

// External goroutine pushes events
go func() {
    for event := range source {
        eventChan <- event
    }
}()
```

## Real-World Usage

Hersh powers **[Hershy](https://github.com/HershyOrg/hershy)**, a container orchestration system that manages Docker containers with reactive state management.

**Production Examples:**
- **[simple-counter](https://github.com/HershyOrg/hershy/tree/main/examples/simple-counter)**: Basic counter with WatcherAPI control
- **[trading-long](https://github.com/HershyOrg/hershy/tree/main/examples/trading-long)**: Real-time trading simulator with command handling
- **[watcher-server](https://github.com/HershyOrg/hershy/tree/main/examples/watcher-server)**: HTTP server with persistent state

These examples demonstrate Hersh running inside Docker containers with:
- WatcherAPI exposed on localhost ports
- Persistent state in `/state` directory
- External control via HTTP API
- Real-time reactive updates

## License

MIT License

## Contributing

Contributions are welcome! Please submit a Pull Request or open an Issue.

## Links

- **Repository**: https://github.com/HershyOrg/hersh
- **Issues**: https://github.com/HershyOrg/hersh/issues
- **Documentation**: https://pkg.go.dev/github.com/HershyOrg/hersh
- **Hershy (Container Orchestration)**: https://github.com/HershyOrg/hershy
