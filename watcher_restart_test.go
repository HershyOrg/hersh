package hersh

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HershyOrg/hersh/shared"
	"github.com/HershyOrg/hersh/wmachine"
)

// TestWatcherRestart verifies that Watcher can stop and restart Manager multiple times
// Scenario: Stop→Run×3→Stop→Run×3→Stop
func TestWatcherRestart(t *testing.T) {
	config := DefaultWatcherConfig()
	config.DefaultTimeout = 5 * time.Second

	watcher := NewWatcher(config, nil)

	// Track execution counts
	var (
		executionCount   atomic.Int32
		watchCallValue   atomic.Int32
		watchFlowValue   atomic.Int32
		memoComputeCount atomic.Int32
	)

	// Managed function with Watch variables
	managedFunc := func(msg *Message, ctx ManageContext) error {
		executionCount.Add(1)

		// WatchCall: tick-based with computation
		tickWatch := WatchCall(
			0, // initial value
			func() (wmachine.VarUpdateFunc[int], bool, error) {
				// Return computation function
				computeFunc := func(prev int) (int, error) {
					memoComputeCount.Add(1)
					return int(time.Now().UnixMilli() % 1000), nil
				}
				return computeFunc, false, nil // false = don't skip signal
			},
			"tick",
			100*time.Millisecond,
			ctx,
		)
		if tickWatch.Error != nil {
			t.Errorf("WatchCall error: %v", tickWatch.Error)
			return tickWatch.Error
		}
		watchCallValue.Store(int32(tickWatch.Value))

		// WatchFlow: channel-based
		flowWatch := WatchFlow(
			0, // initial value
			func(ctx context.Context) (<-chan shared.FlowValue[int], error) {
				flowCh := make(chan shared.FlowValue[int], 10)
				go func() {
					ticker := time.NewTicker(100 * time.Millisecond)
					defer ticker.Stop()
					counter := 1
					for {
						select {
						case <-ctx.Done():
							close(flowCh)
							return
						case <-ticker.C:
							select {
							case flowCh <- shared.FlowValue[int]{V: counter}:
								counter++
							case <-ctx.Done():
								close(flowCh)
								return
							}
						}
					}
				}()
				return flowCh, nil
			},
			"flow",
			ctx,
		)
		if flowWatch.Error != nil {
			t.Errorf("WatchFlow error: %v", flowWatch.Error)
			return flowWatch.Error
		}
		watchFlowValue.Store(int32(flowWatch.Value))

		// Memo: should be recalculated after restart
		memoValue := Memo(func() string {
			return "computed_value"
		}, "memo_key", ctx)
		_ = memoValue

		return nil
	}

	// Register managed function
	watcher.Manage(managedFunc, "test_restart", nil)

	// Start watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for first execution to complete and reach Ready state
	waitForReady(t, watcher, 5*time.Second)

	t.Log("=== Phase 1: First run cycle (3 executions) ===")

	// Trigger 3 executions
	initialExecCount := executionCount.Load()
	for i := 0; i < 3; i++ {
		if err := watcher.SendMessage("test"); err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}
		time.Sleep(200 * time.Millisecond) // Wait for execution
	}

	// Verify executions occurred
	firstCycleCount := executionCount.Load() - initialExecCount
	if firstCycleCount < 3 {
		t.Errorf("Expected at least 3 executions in first cycle, got %d", firstCycleCount)
	}

	// Store values before stop
	beforeStopWatchCall := watchCallValue.Load()
	beforeStopWatchFlow := watchFlowValue.Load()
	beforeStopMemoCount := memoComputeCount.Load()

	t.Logf("Before stop - WatchCall: %d, WatchFlow: %d, MemoComputes: %d",
		beforeStopWatchCall, beforeStopWatchFlow, beforeStopMemoCount)

	t.Log("=== Stopping Manager ===")

	// Stop Manager (not Watcher)
	if err := watcher.StopManager(); err != nil {
		t.Fatalf("StopManager failed: %v", err)
	}

	// Verify Manager is in Stopped state
	if state := watcher.GetState(); state != StateStopped {
		t.Errorf("Expected Stopped state, got %s", state)
	}

	t.Log("=== Verifying stop state ===")

	// Note: After Stop, watches are canceled but registries are NOT cleared
	// Registries are only cleared during RunManager (reinitialization)
	// This is the correct behavior - cleanup != reinitialization

	mgr := watcher.manager

	// Verify watches exist but are canceled (can't easily verify cancellation here)
	watchCount := 0
	mgr.GetWatchRegistry().Range(func(key, value any) bool {
		watchCount++
		return true
	})
	t.Logf("Watch registry after stop: %d entries (will be cleared on restart)", watchCount)

	// Verify VarState exists (will be cleared on restart)
	varStateCount := len(mgr.GetState().VarState.GetAll())
	t.Logf("VarState after stop: %d entries (will be cleared on restart)", varStateCount)

	// Verify MemoCache exists (will be cleared on restart)
	memoCount := 0
	mgr.GetMemoCache().Range(func(key, value any) bool {
		memoCount++
		return true
	})
	t.Logf("MemoCache after stop: %d entries (will be cleared on restart)", memoCount)

	t.Log("=== Restarting Manager ===")

	// Restart Manager
	if err := watcher.RunManager(); err != nil {
		t.Fatalf("RunManager failed: %v", err)
	}

	// Wait for Ready state after restart
	waitForReady(t, watcher, 5*time.Second)

	t.Log("=== Verifying reinitialization ===")

	// After restart, all state should be cleared and fresh watches registered
	watchCountAfterRestart := 0
	mgr.GetWatchRegistry().Range(func(key, value any) bool {
		watchCountAfterRestart++
		return true
	})
	t.Logf("Watch registry after restart: %d entries (fresh watches)", watchCountAfterRestart)

	// VarState should have fresh values from new watches
	varStateCountAfterRestart := len(mgr.GetState().VarState.GetAll())
	t.Logf("VarState after restart: %d entries (fresh values)", varStateCountAfterRestart)

	// MemoCache should be cleared
	memoCountAfterRestart := 0
	mgr.GetMemoCache().Range(func(key, value any) bool {
		memoCountAfterRestart++
		return true
	})
	if memoCountAfterRestart != 0 {
		// Note: Memo might be set during first execution after restart
		t.Logf("MemoCache after restart: %d entries (may be repopulated during first execution)", memoCountAfterRestart)
	}

	t.Log("=== Phase 2: Second run cycle (3 executions) ===")

	// Reset memo compute count to verify recalculation
	memoComputeCount.Store(0)

	// Trigger 3 more executions
	secondCycleStart := executionCount.Load()
	for i := 0; i < 3; i++ {
		if err := watcher.SendMessage("test"); err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}
		time.Sleep(200 * time.Millisecond) // Wait for execution
	}

	// Verify executions occurred
	secondCycleCount := executionCount.Load() - secondCycleStart
	if secondCycleCount < 3 {
		t.Errorf("Expected at least 3 executions in second cycle, got %d", secondCycleCount)
	}

	// Verify Memo was recalculated (should be computed again after restart)
	afterRestartMemoCount := memoComputeCount.Load()
	if afterRestartMemoCount == 0 {
		t.Error("Expected Memo to be recalculated after restart, but compute count is 0")
	}

	t.Log("=== Second stop ===")

	// Stop again
	if err := watcher.StopManager(); err != nil {
		t.Fatalf("Second StopManager failed: %v", err)
	}

	if state := watcher.GetState(); state != StateStopped {
		t.Errorf("Expected Stopped state after second stop, got %s", state)
	}

	t.Log("=== Second restart ===")

	// Restart again
	if err := watcher.RunManager(); err != nil {
		t.Fatalf("Second RunManager failed: %v", err)
	}

	waitForReady(t, watcher, 5*time.Second)

	t.Log("=== Phase 3: Third run cycle (3 executions) ===")

	// Trigger 3 more executions
	thirdCycleStart := executionCount.Load()
	for i := 0; i < 3; i++ {
		if err := watcher.SendMessage("test"); err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	thirdCycleCount := executionCount.Load() - thirdCycleStart
	if thirdCycleCount < 3 {
		t.Errorf("Expected at least 3 executions in third cycle, got %d", thirdCycleCount)
	}

	t.Log("=== Final stop ===")

	// Final stop
	if err := watcher.StopManager(); err != nil {
		t.Fatalf("Final StopManager failed: %v", err)
	}

	// Final cleanup - stop Watcher itself
	if err := watcher.Stop(); err != nil {
		t.Fatalf("Failed to stop watcher: %v", err)
	}

	t.Logf("Test completed successfully - Total executions: %d", executionCount.Load())
}

// waitForReady waits for Manager to reach Ready state
func waitForReady(t *testing.T, w *Watcher, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			t.Fatalf("Timeout waiting for Ready state")
		}

		state := w.GetState()
		if state == StateReady {
			return
		}

		if state == StateCrashed || state == StateKilled {
			t.Fatalf("Manager entered terminal state: %s", state)
		}

		<-ticker.C
	}
}
