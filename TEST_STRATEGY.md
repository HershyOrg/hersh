# Hersh Framework Test Strategy

## Overview
This document outlines the testing strategy for the Hersh reactive framework, with special focus on lifecycle management and timing considerations.

## Critical Timing Considerations

### Stop Operation Duration
The `Watcher.Stop()` operation can take significant time to complete:
- **Average case**: ~1 minute for normal cleanup
- **Worst case**: Up to 5 minutes for complex cleanup operations
- **Reason**: Polling-based synchronization mechanism ensures deterministic cleanup completion

### Why Stop Takes Time

1. **Polling-based Synchronization**:
   - The framework uses 500ms polling intervals to check cleanup completion
   - This ensures deterministic state transitions without race conditions
   - Trade-off: Reliability over speed

2. **Graceful Shutdown Sequence**:
   - Manager sends Stop signal
   - EffectHandler executes cleanup script
   - All Watch goroutines must terminate
   - API server shutdown (5-second timeout)
   - State verification at each step

3. **Timeout Hierarchy**:
   - Cleanup execution: 60 seconds
   - API server shutdown: 5 seconds
   - Total Stop timeout: 5 minutes
   - Test waiting timeout: 6 minutes (with buffer)

## Test Patterns

### 1. Lifecycle Tests

#### Pattern: Waiting for Stop Completion
```go
// Wait for Watcher to stop with extended timeout
deadline := time.Now().Add(6 * time.Minute)
for time.Now().Before(deadline) {
    if !watcher.isRunning.Load() {
        break
    }
    time.Sleep(100 * time.Millisecond)
}
```

**Why 6 minutes?**
- Stop operation max: 5 minutes
- Buffer time: 1 minute
- Ensures test doesn't fail due to timing

#### Pattern: Verifying Complete Cleanup
```go
// 1. Check Watcher stopped
if w.isRunning.Load() {
    t.Fatal("Watcher should be stopped")
}

// 2. Check Manager's rootCtx cancelled
select {
case <-w.manager.GetEffectHandler().GetRootContext().Done():
    // Good - context cancelled
default:
    t.Fatal("Manager's rootCtx should be cancelled")
}

// 3. Check all Watch goroutines terminated
for i, ch := range channels {
    select {
    case <-ch:
        // Channel closed = goroutine terminated
    default:
        t.Fatalf("Watch goroutine %d still running", i)
    }
}
```

### 2. Parent Context Cancellation Tests

#### Pattern: Auto-shutdown on Parent Context Cancel
```go
// Create Watcher with parent context
parentCtx, parentCancel := context.WithCancel(context.Background())
w := NewWatcher(config, envVars, parentCtx)

// Start operations...
w.Start()

// Cancel parent - triggers auto-shutdown
parentCancel()

// Wait with extended timeout (same 6-minute pattern)
deadline := time.Now().Add(6 * time.Minute)
// ... waiting loop ...
```

**Important**: Parent context cancellation triggers the same Stop flow, so it needs the same timeout.

### 3. Watch Registration Tests

#### Pattern: Concurrent Watch Operations
```go
// Create multiple WatchFlows concurrently
for i := 0; i < 3; i++ {
    ch := make(chan int, 10)
    channels[i] = ch

    WatchFlow(fmt.Sprintf("flow%d", i), ch,
        func(value int) { /* process */ }, ctx)
}

// Always cleanup channels on test completion
defer func() {
    for _, ch := range channels {
        close(ch)
    }
}()
```

## Test Categories

### Unit Tests
- **Focus**: Individual component behavior
- **Timeout**: Standard (30 seconds)
- **Examples**: Reducer logic, Signal priority, State transitions

### Integration Tests
- **Focus**: Component interaction
- **Timeout**: Extended (2 minutes)
- **Examples**: Manager-Watcher coordination, Watch-Signal flow

### Lifecycle Tests
- **Focus**: Complete startup/shutdown sequences
- **Timeout**: Maximum (6+ minutes)
- **Examples**: Stop operations, Context cancellation, Cleanup verification
- **Special Consideration**: Must account for polling-based synchronization delays

## Best Practices

1. **Always Use Extended Timeouts for Stop Operations**
   - Never assume Stop will complete quickly
   - Always wait at least 6 minutes in tests
   - Log intermediate states for debugging

2. **Verify Complete Cleanup**
   - Check Watcher state
   - Check Manager state
   - Check all goroutine termination
   - Check resource cleanup (API server, channels)

3. **Handle Already-Stopped States**
   - Stop() should be idempotent
   - Check for terminal states before operations
   - Clean up resources even in early-exit paths

4. **Use Deterministic Polling**
   - Prefer polling over channel operations for state checks
   - Use consistent polling intervals (100ms for tests)
   - Always have timeout boundaries

5. **Document Timing Dependencies**
   - Comment why specific timeouts are chosen
   - Explain any timing-sensitive operations
   - Note any race condition mitigations

## Common Pitfalls

1. **Insufficient Timeout**: Tests fail intermittently due to Stop taking longer than expected
   - **Solution**: Always use 6-minute timeout for Stop operations

2. **Resource Leaks**: Goroutines or channels not properly cleaned up
   - **Solution**: Always defer cleanup, verify termination in tests

3. **Race Conditions**: Concurrent access to shared state
   - **Solution**: Use polling-based checks, avoid direct channel operations

4. **API Server Conflicts**: Port already in use errors
   - **Solution**: Ensure API server shutdown in all paths, including early-stop

## Test Execution Guidelines

### Running Lifecycle Tests
```bash
# Run with extended timeout
go test -v -timeout 10m -run TestManagerLifecycle

# Run all tests with proper timeout
go test ./... -v -timeout 10m
```

### Debugging Slow Tests
1. Check logs for "[Watcher] Waiting for cleanup completion..."
2. Verify cleanup function isn't hanging
3. Check for deadlocks in Watch loops
4. Ensure all contexts are properly cancelled

## Polling-Based Synchronization Details

The framework uses polling instead of channels for critical synchronization:

```go
// Example from clearRunScript
for i := 0; i < 120; i++ { // 60 seconds total
    eh.mu.RLock()
    if eh.cleanupCompleted {
        eh.mu.RUnlock()
        break
    }
    eh.mu.RUnlock()
    time.Sleep(500 * time.Millisecond)
}
```

**Benefits**:
- Deterministic behavior
- No channel deadlocks
- Clear timeout boundaries
- Easy to debug and reason about

**Trade-offs**:
- Slower than channel-based coordination
- Fixed polling intervals add latency
- Requires patience in tests

## Summary

The Hersh framework prioritizes **correctness and determinism** over speed in lifecycle operations. This means:

1. Stop operations can take up to 5 minutes
2. Tests must wait up to 6 minutes for cleanup
3. Polling-based synchronization ensures reliability
4. Complete cleanup verification is essential

When writing tests, always:
- Use extended timeouts for lifecycle operations
- Verify complete cleanup at each stage
- Document timing dependencies clearly
- Handle edge cases (already-stopped, early-exit)