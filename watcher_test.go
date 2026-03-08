package hersh

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HershyOrg/hersh/manager"
	"github.com/HershyOrg/hersh/shared"
)

// TestWatchCall_BasicFunctionality tests basic WatchCall behavior
func TestWatchCall_BasicFunctionality(t *testing.T) {
	config := DefaultWatcherConfig()
	config.DefaultTimeout = 5 * time.Second

	watcher := NewWatcher(config, nil, nil)

	executeCount := int32(0)
	varValue := int32(0)

	managedFunc := func(msg *Message, ctx ManageContext) error {
		atomic.AddInt32(&executeCount, 1)

		// WatchCall with compute function
		val := WatchCall[int32](int32(0),
			func() (manager.VarUpdateFunc[int32], bool, error) {
				return func(prev int32) (int32, error) {
					newVal := atomic.AddInt32(&varValue, 1)
					return newVal, nil
				}, false, nil
			},
			"testVar",
			100*time.Millisecond,
			ctx,
		)

		t.Logf("WatchCall returned: %v", val.Value)

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Wait for several watch cycles
	time.Sleep(500 * time.Millisecond)

	// Verify that managed function was executed
	executions := atomic.LoadInt32(&executeCount)
	if executions < 1 {
		t.Errorf("Expected at least 1 execution, got %d", executions)
	}

	// Verify that varValue was incremented
	finalVarValue := atomic.LoadInt32(&varValue)
	if finalVarValue < 2 {
		t.Errorf("Expected varValue to be incremented at least twice, got %d", finalVarValue)
	}

	t.Logf("Test complete - executions: %d, varValue: %d", executions, finalVarValue)
}

// TestWatchCall_ValuePersistence tests that WatchCall values persist across executions
func TestWatchCall_ValuePersistence(t *testing.T) {
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	observedValues := make([]any, 0)
	executionCount := 0

	managedFunc := func(msg *Message, ctx ManageContext) error {
		executionCount++

		val := WatchCall[int](0,
			func() (manager.VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return executionCount, nil
				}, false, nil
			},
			"counter",
			50*time.Millisecond,
			ctx,
		)

		observedValues = append(observedValues, val.Value)
		t.Logf("Execution %d: observed value = %v", executionCount, val.Value)

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(400 * time.Millisecond)

	if len(observedValues) < 2 {
		t.Errorf("Expected at least 2 observed values, got %d", len(observedValues))
	}

	// Verify that values increase over time (persistence)
	for i := 1; i < len(observedValues); i++ {
		curr := observedValues[i].(int)
		prev := observedValues[i-1].(int)
		if curr <= prev {
			t.Errorf("Expected increasing values, got %v -> %v", prev, curr)
		}
	}

	t.Logf("Test complete - observed values: %v", observedValues)
}

// TestWatchFlow_ChannelBased tests WatchFlow with channel-based reactive programming
func TestWatchFlow_ChannelBased(t *testing.T) {
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	receivedValues := make([]any, 0)
	executeCount := int32(0)

	// Create channel function (new WatchFlow signature)
	getChannelFunc := func(ctx context.Context) (<-chan shared.FlowValue[int], error) {
		sourceChan := make(chan shared.FlowValue[int], 10)

		go func() {
			defer close(sourceChan)

			// Send initial value
			time.Sleep(50 * time.Millisecond)
			sourceChan <- shared.FlowValue[int]{V: 0, E: nil}

			// Send more values
			for i := 1; i <= 5; i++ {
				select {
				case <-ctx.Done():
					return
				case <-time.After(100 * time.Millisecond):
					sourceChan <- shared.FlowValue[int]{V: i, E: nil}
					t.Logf("Sent value: %d", i)
				}
			}
		}()

		return sourceChan, nil
	}

	managedFunc := func(msg *Message, ctx ManageContext) error {
		atomic.AddInt32(&executeCount, 1)

		val := WatchFlow[int](0, getChannelFunc, "flowVar", ctx)

		receivedValues = append(receivedValues, val.Value)
		t.Logf("Execution %d: received value = %v", atomic.LoadInt32(&executeCount), val.Value)

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(800 * time.Millisecond)

	executions := atomic.LoadInt32(&executeCount)
	if executions < 3 {
		t.Errorf("Expected at least 3 executions, got %d", executions)
	}

	if len(receivedValues) < 3 {
		t.Errorf("Expected at least 3 received values, got %d", len(receivedValues))
	}

	t.Logf("Test complete - executions: %d, received values: %v", executions, receivedValues)
}

// TestWatchFlow_ChannelClosed tests WatchFlow behavior when channel is closed
func TestWatchFlow_ChannelClosed(t *testing.T) {
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	receivedValues := make([]any, 0)

	// Create channel function that closes after sending values
	getChannelFunc := func(ctx context.Context) (<-chan shared.FlowValue[int], error) {
		sourceChan := make(chan shared.FlowValue[int], 5)

		go func() {
			defer close(sourceChan)

			time.Sleep(50 * time.Millisecond)
			sourceChan <- shared.FlowValue[int]{V: 1, E: nil}
			time.Sleep(100 * time.Millisecond)
			sourceChan <- shared.FlowValue[int]{V: 2, E: nil}
			time.Sleep(100 * time.Millisecond)
			t.Log("Channel closed")
		}()

		return sourceChan, nil
	}

	managedFunc := func(msg *Message, ctx ManageContext) error {
		val := WatchFlow[int](0, getChannelFunc, "flowVar", ctx)
		if !val.IsError() {
			receivedValues = append(receivedValues, val.Value)
		}
		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(500 * time.Millisecond)

	// Should have received values before channel closed
	if len(receivedValues) < 1 {
		t.Errorf("Expected at least 1 received value before channel close, got %d", len(receivedValues))
	}

	t.Logf("Test complete - received values: %v", receivedValues)
}

// TestMemo_BasicCaching tests basic Memo caching functionality
func TestMemo_BasicCaching(t *testing.T) {
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	computeCount := int32(0)
	executeCount := int32(0)

	managedFunc := func(msg *Message, ctx ManageContext) error {
		atomic.AddInt32(&executeCount, 1)

		// Memo should compute only once
		val := Memo(func() string {
			count := atomic.AddInt32(&computeCount, 1)
			t.Logf("Computing expensive value: call %d", count)
			return "expensive-result"
		}, "cachedValue", ctx)

		if val != "expensive-result" {
			t.Errorf("Expected 'expensive-result', got %v", val)
		}

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Trigger multiple executions
	time.Sleep(100 * time.Millisecond)
	watcher.SendMessage("msg1")
	time.Sleep(100 * time.Millisecond)
	watcher.SendMessage("msg2")
	time.Sleep(200 * time.Millisecond)

	executions := atomic.LoadInt32(&executeCount)
	computes := atomic.LoadInt32(&computeCount)

	// Managed function should execute multiple times
	if executions < 3 {
		t.Errorf("Expected at least 3 executions, got %d", executions)
	}

	// But Memo compute should only happen once
	if computes != 1 {
		t.Errorf("Expected exactly 1 compute call (cached), got %d", computes)
	}

	t.Logf("Test complete - executions: %d, compute calls: %d", executions, computes)
}

// TestMemo_ClearMemo tests ClearMemo functionality
func TestMemo_ClearMemo(t *testing.T) {
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	computeCount := int32(0)

	managedFunc := func(msg *Message, ctx ManageContext) error {
		if msg != nil && msg.Content == "clear" {
			ClearMemo("counter", ctx)
			t.Log("Memo cleared")
			return nil
		}

		val := Memo(func() int32 {
			count := atomic.AddInt32(&computeCount, 1)
			return count
		}, "counter", ctx)

		t.Logf("Memo value: %v", val)
		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(100 * time.Millisecond)

	// First access - should compute
	watcher.SendMessage("access1")
	time.Sleep(100 * time.Millisecond)

	// Second access - should use cache (no compute)
	watcher.SendMessage("access2")
	time.Sleep(100 * time.Millisecond)

	computes1 := atomic.LoadInt32(&computeCount)
	if computes1 != 1 {
		t.Errorf("Expected 1 compute before clear, got %d", computes1)
	}

	// Clear memo
	watcher.SendMessage("clear")
	time.Sleep(100 * time.Millisecond)

	// Third access - should recompute
	watcher.SendMessage("access3")
	time.Sleep(100 * time.Millisecond)

	computes2 := atomic.LoadInt32(&computeCount)
	if computes2 != 2 {
		t.Errorf("Expected 2 computes after clear, got %d", computes2)
	}

	t.Logf("Test complete - total compute calls: %d", computes2)
}

// TestWatcher_MultipleWatchVariables tests multiple Watch variables working together
func TestWatcher_MultipleWatchVariables(t *testing.T) {
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	counter1 := int32(0)
	counter2 := int32(0)
	executeCount := int32(0)

	managedFunc := func(msg *Message, ctx ManageContext) error {
		atomic.AddInt32(&executeCount, 1)

		val1 := WatchCall[int32](int32(0),
			func() (manager.VarUpdateFunc[int32], bool, error) {
				return func(prev int32) (int32, error) {
					return atomic.AddInt32(&counter1, 1), nil
				}, false, nil
			},
			"var1",
			80*time.Millisecond,
			ctx,
		)

		val2 := WatchCall[int32](int32(0),
			func() (manager.VarUpdateFunc[int32], bool, error) {
				return func(prev int32) (int32, error) {
					return atomic.AddInt32(&counter2, 2), nil
				}, false, nil
			},
			"var2",
			80*time.Millisecond,
			ctx,
		)

		t.Logf("Execution %d: var1=%v, var2=%v", atomic.LoadInt32(&executeCount), val1.Value, val2.Value)

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(500 * time.Millisecond)

	executions := atomic.LoadInt32(&executeCount)
	if executions < 3 {
		t.Errorf("Expected at least 3 executions, got %d", executions)
	}

	c1 := atomic.LoadInt32(&counter1)
	c2 := atomic.LoadInt32(&counter2)

	if c1 < 3 {
		t.Errorf("Expected counter1 >= 3, got %d", c1)
	}

	if c2 < 6 {
		t.Errorf("Expected counter2 >= 6, got %d", c2)
	}

	t.Logf("Test complete - executions: %d, counter1: %d, counter2: %d", executions, c1, c2)
}

// TestWatcher_WatchAndMemo tests Watch and Memo working together
func TestWatcher_WatchAndMemo(t *testing.T) {
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	watchCounter := int32(0)
	memoComputeCount := int32(0)

	managedFunc := func(msg *Message, ctx ManageContext) error {
		// Watch value changes frequently
		watchVal := WatchCall[int32](int32(0),
			func() (manager.VarUpdateFunc[int32], bool, error) {
				return func(prev int32) (int32, error) {
					return atomic.AddInt32(&watchCounter, 1), nil
				}, false, nil
			},
			"frequentVar",
			50*time.Millisecond,
			ctx,
		)

		// Memo computes once
		memoVal := Memo(func() string {
			atomic.AddInt32(&memoComputeCount, 1)
			return "cached-config"
		}, "config", ctx)

		if !watchVal.IsError() {
			t.Logf("Watch value: %v, Memo value: %v", watchVal.Value, memoVal)
		}

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(400 * time.Millisecond)

	watchCount := atomic.LoadInt32(&watchCounter)
	memoCount := atomic.LoadInt32(&memoComputeCount)

	// Watch should be called many times
	if watchCount < 5 {
		t.Errorf("Expected watchCounter >= 5, got %d", watchCount)
	}

	// Memo should be computed only once
	if memoCount != 1 {
		t.Errorf("Expected memoComputeCount = 1, got %d", memoCount)
	}

	t.Logf("Test complete - watch count: %d, memo compute count: %d", watchCount, memoCount)
}

// TestWatcher_HershContextAccess tests accessing Watcher through HershContext
func TestWatcher_HershContextAccess(t *testing.T) {
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	contextValid := false

	managedFunc := func(msg *Message, ctx ManageContext) error {
		// Verify we can access watcher from context
		watcherFromCtx := ctx.GetWatcher()
		if watcherFromCtx != nil {
			contextValid = true
			t.Log("Successfully accessed watcher from HershContext")
		}

		// Use Watch to verify context is working
		val := WatchCall[int](0,
			func() (manager.VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return 42, nil
				}, false, nil
			},
			"contextTest",
			100*time.Millisecond,
			ctx,
		)

		t.Logf("Watch value: %v", val.Value)

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(300 * time.Millisecond)

	if !contextValid {
		t.Error("Failed to access watcher from HershContext")
	}

	t.Log("Test complete - HershContext access verified")
}

// TestWatcher_StopCancelsWatches tests that Stop() stops the watcher gracefully
func TestWatcher_StopCancelsWatches(t *testing.T) {
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	watchCallCount := int32(0)

	managedFunc := func(msg *Message, ctx ManageContext) error {
		WatchCall[int64](0,
			func() (manager.VarUpdateFunc[int64], bool, error) {
				return func(prev int64) (int64, error) {
					atomic.AddInt32(&watchCallCount, 1)
					return time.Now().Unix(), nil
				}, false, nil
			},
			"activeCheck",
			50*time.Millisecond,
			ctx,
		)

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Let it run for a bit
	time.Sleep(200 * time.Millisecond)
	callsBeforeStop := atomic.LoadInt32(&watchCallCount)

	// Stop watcher
	err = watcher.Stop()
	if err != nil {
		t.Fatalf("Failed to stop watcher: %v", err)
	}

	// Wait and verify no more watch calls happen
	time.Sleep(300 * time.Millisecond)
	callsAfterStop := atomic.LoadInt32(&watchCallCount)

	// After stop, watch should stop running (calls should not increase significantly)
	// Allow some buffer for in-flight operations (up to 4 additional calls)
	if callsAfterStop > callsBeforeStop+4 {
		t.Errorf("Watch continued running after Stop: before=%d, after=%d", callsBeforeStop, callsAfterStop)
	}

	t.Logf("Test complete - calls before stop: %d, calls after stop: %d", callsBeforeStop, callsAfterStop)
}

// TestWatchCall_ErrorHandling tests error handling in WatchCall compute function
func TestWatchCall_ErrorHandling(t *testing.T) {
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	errorCount := int32(0)
	successCount := int32(0)

	managedFunc := func(msg *Message, ctx ManageContext) error {
		val := WatchCall[int32](int32(0),
			func() (manager.VarUpdateFunc[int32], bool, error) {
				return func(prev int32) (int32, error) {
					count := atomic.AddInt32(&errorCount, 1)
					if count%2 == 0 {
						// Return error on even calls
						return 0, context.DeadlineExceeded
					}
					atomic.AddInt32(&successCount, 1)
					return count, nil
				}, false, nil
			},
			"errorVar",
			100*time.Millisecond,
			ctx,
		)

		t.Logf("Received value despite errors: %v", val.Value)

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(600 * time.Millisecond)

	errors := atomic.LoadInt32(&errorCount)
	successes := atomic.LoadInt32(&successCount)

	if errors < 4 {
		t.Errorf("Expected at least 4 error attempts, got %d", errors)
	}

	if successes < 2 {
		t.Errorf("Expected at least 2 successful calls, got %d", successes)
	}

	t.Logf("Test complete - errors: %d, successes: %d", errors, successes)
}

// TestWatcher_ContextCancellation tests auto-stop when parent context is cancelled
func TestWatcher_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := DefaultWatcherConfig()
	config.ServerPort = 0 // Disable API server
	watcher := NewWatcher(config, nil, ctx)

	executionCount := int32(0)
	managedFunc := func(msg *Message, hctx ManageContext) error {
		atomic.AddInt32(&executionCount, 1)
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Let it run for a bit
	time.Sleep(100 * time.Millisecond)

	// Cancel context - should trigger auto-stop
	t.Log("Cancelling context...")
	cancel()

	// Wait for auto-stop with timeout
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for watcher to stop")
		case <-ticker.C:
			if !watcher.isRunning.Load() {
				t.Logf("Test complete - executions before stop: %d", atomic.LoadInt32(&executionCount))
				return
			}
		}
	}
}

// TestWatcher_ContextTimeout tests auto-stop with context timeout
func TestWatcher_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	config := DefaultWatcherConfig()
	config.ServerPort = 0
	watcher := NewWatcher(config, nil, ctx)

	managedFunc := func(msg *Message, hctx ManageContext) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for context timeout + auto-stop
	time.Sleep(400 * time.Millisecond)

	if watcher.isRunning.Load() {
		t.Error("Watcher still running after context timeout")
	}

	t.Log("Test complete - context timeout triggered auto-stop")
}

// TestWatcher_NilContext tests backward compatibility with nil context
func TestWatcher_NilContext(t *testing.T) {
	config := DefaultWatcherConfig()
	config.ServerPort = 0

	watcher := NewWatcher(config, nil, nil)

	executionCount := int32(0)
	managedFunc := func(msg *Message, hctx ManageContext) error {
		atomic.AddInt32(&executionCount, 1)
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher with nil context: %v", err)
	}

	// Let it run
	time.Sleep(100 * time.Millisecond)

	// Manual stop should work
	err = watcher.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	if executionCount == 0 {
		t.Error("Expected at least one execution")
	}

	t.Logf("Test complete - nil context backward compatibility verified (executions: %d)", executionCount)
}

// TestWatcher_ManualStopAfterContextCancel tests manual Stop after context cancel
func TestWatcher_ManualStopAfterContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := DefaultWatcherConfig()
	config.ServerPort = 0
	watcher := NewWatcher(config, nil, ctx)

	managedFunc := func(msg *Message, hctx ManageContext) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Cancel context first
	cancel()

	// Wait for auto-stop
	time.Sleep(200 * time.Millisecond)

	// Manual stop after auto-stop should return error (not running)
	err = watcher.Stop()
	if err == nil {
		t.Error("Expected error from Stop() after auto-stop")
	}

	t.Logf("Test complete - manual Stop after auto-stop behaves correctly: %v", err)
}
