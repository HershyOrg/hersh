package test

import (
	"github.com/HershyOrg/hershy/hersh/manager"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HershyOrg/hershy/hersh"
	"github.com/HershyOrg/hershy/hersh/shared"
)

// ORIGINAL TESTS - Testing actual framework behavior expectations

// TestEdgeCase_StopDuringInitRun_Original tests that cleanup IS called when stopped during InitRun
func TestEdgeCase_StopDuringInitRun_Original(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	cleanupCalled := int32(0)
	executionCount := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		atomic.AddInt32(&executionCount, 1)

		// Register a slow watch to keep in InitRun state
		hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					time.Sleep(100 * time.Millisecond)
					return time.Now().Unix(), true, nil
				}, nil
			},
			"slowWatch",
			500*time.Millisecond,
			ctx,
		)

		return nil
	}

	watcher.Manage(managedFunc, "test").Cleanup(func(ctx shared.HershContext) {
		atomic.AddInt32(&cleanupCalled, 1)
		t.Log("Cleanup called")
	})

	// Start in background
	startDone := make(chan error, 1)
	go func() {
		startDone <- watcher.Start()
	}()

	// Wait a bit to ensure we're in InitRun
	time.Sleep(100 * time.Millisecond)

	// Stop during InitRun
	err := watcher.Stop()
	if err != nil {
		t.Logf("Stop returned error: %v", err)
	}

	// Wait for start to complete or timeout
	select {
	case startErr := <-startDone:
		t.Logf("Start completed with: %v", startErr)
	case <-time.After(1 * time.Second):
		t.Log("Start did not complete")
	}

	cleanup := atomic.LoadInt32(&cleanupCalled)
	executions := atomic.LoadInt32(&executionCount)

	// ORIGINAL EXPECTATION: Cleanup should be called
	if cleanup == 0 {
		t.Error("Cleanup was not called after stop")
	}

	t.Logf("Cleanup called: %d times, Executions: %d", cleanup, executions)
}

// TestEdgeCase_StopErrorHandling_Original expects second Stop() to return error
// SKIPPED: After context-based auto-shutdown implementation, StopError auto-stop behavior changed
func TestEdgeCase_StopErrorHandling_Original(t *testing.T) {
	t.Skip("Skipping legacy test - behavior changed with context-based shutdown")
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	executionCount := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		count := atomic.AddInt32(&executionCount, 1)

		if count == 2 {
			t.Log("Returning StopError")
			return &shared.StopError{Reason: "test stop"}
		}

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Trigger execution with StopError
	time.Sleep(100 * time.Millisecond)
	watcher.SendMessage("trigger")

	// Wait for StopError to trigger automatic stop
	time.Sleep(500 * time.Millisecond)

	// Check state - should be Stopped
	state := watcher.GetState()
	if state != shared.StateStopped {
		t.Errorf("Expected Stopped state after StopError, got %s", state)
	}

	// ORIGINAL EXPECTATION: Second Stop() should return error
	err = watcher.Stop()
	if err == nil {
		t.Error("Expected error from second Stop() after automatic stop, got nil")
	} else {
		t.Logf("Second stop returned error (expected): %v", err)
	}

	t.Log("Test complete - StopError handled")
}

// TestEdgeCase_CleanupTimeout_Original expects Stop() to wait for cleanup
func TestEdgeCase_CleanupTimeout_Original(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	cleanupStarted := int32(0)
	cleanupCompleted := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					return time.Now().Unix(), true, nil
				}, nil
			},
			"watch1",
			100*time.Millisecond,
			ctx,
		)
		return nil
	}

	watcher.Manage(managedFunc, "test").Cleanup(func(ctx shared.HershContext) {
		atomic.StoreInt32(&cleanupStarted, 1)
		t.Log("Cleanup started")

		// Simulate slow cleanup (200ms)
		time.Sleep(200 * time.Millisecond)

		atomic.StoreInt32(&cleanupCompleted, 1)
		t.Log("Cleanup completed")
	})

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Stop should wait for cleanup
	stopStart := time.Now()
	err = watcher.Stop()
	stopDuration := time.Since(stopStart)

	if err != nil {
		t.Logf("Stop returned: %v (duration: %v)", err, stopDuration)
	}

	started := atomic.LoadInt32(&cleanupStarted)
	completed := atomic.LoadInt32(&cleanupCompleted)

	if started == 0 {
		t.Error("Cleanup was not started")
	}

	// ORIGINAL EXPECTATION: Cleanup should complete BEFORE Stop() returns
	if completed == 0 {
		t.Error("Cleanup did not complete before Stop() returned")
	}

	// ORIGINAL EXPECTATION: Stop() should take at least 100ms (cleanup time)
	if stopDuration < 100*time.Millisecond {
		t.Errorf("Stop returned too quickly: %v (expected >= 100ms for cleanup)", stopDuration)
	}

	t.Logf("Cleanup: started=%d, completed=%d, stop duration=%v", started, completed, stopDuration)
}

// TestEdgeCase_PanicRecovery_Original expects Ready state after panic
func TestEdgeCase_PanicRecovery_Original(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	// Use short lightweight retry delays for testing (panic is treated as error)
	config.RecoveryPolicy.LightweightRetryDelays = []time.Duration{
		100 * time.Millisecond, // 1st failure
		200 * time.Millisecond, // 2nd failure
		300 * time.Millisecond, // 3rd+ failures
	}
	watcher := hersh.NewWatcher(config, nil, nil)

	executionCount := int32(0)
	panicCount := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		count := atomic.AddInt32(&executionCount, 1)

		// Panic on second execution
		if count == 2 {
			atomic.AddInt32(&panicCount, 1)
			panic("test panic")
		}

		hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					return time.Now().Unix(), true, nil
				}, nil
			},
			"watch1",
			100*time.Millisecond,
			ctx,
		)

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(210 * time.Millisecond)

	// Trigger panic
	watcher.SendMessage("trigger panic")
	// Wait for panic recovery + lightweight retry delay (100ms)
	time.Sleep(500 * time.Millisecond)

	// System should continue working after panic
	watcher.SendMessage("after panic")
	time.Sleep(200 * time.Millisecond)

	executions := atomic.LoadInt32(&executionCount)
	panics := atomic.LoadInt32(&panicCount)

	if panics != 1 {
		t.Errorf("Expected 1 panic, got %d", panics)
	}

	if executions < 3 {
		t.Errorf("Expected at least 3 executions (including panic), got %d", executions)
	}

	// ORIGINAL EXPECTATION: State should be Ready (not Running)
	state := watcher.GetState()
	if state != shared.StateReady {
		t.Errorf("Expected Ready state after panic recovery, got %s", state)
	}

	t.Logf("Panic recovered, state: %s, executions: %d", state, executions)
}

// TestEdgeCase_ContextCancellation_Original expects timeout to be detected
func TestEdgeCase_ContextCancellation_Original(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	config.DefaultTimeout = 500 * time.Millisecond
	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	executionCount := int32(0)
	timeoutCount := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		count := atomic.AddInt32(&executionCount, 1)

		// On receiving "timeout" message, exceed timeout
		if msg != nil && msg.Content == "timeout" {
			t.Logf("Execution %d: Starting long operation that will timeout", count)
			time.Sleep(1 * time.Second) // Exceeds 500ms timeout
			t.Logf("Execution %d: Sleep completed (should not reach here if timeout works)", count)
			return nil // This should not be reached if timeout works
		}

		hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					return time.Now().Unix(), true, nil
				}, nil
			},
			"watch1",
			100*time.Millisecond,
			ctx,
		)

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(200 * time.Millisecond)

	// Trigger timeout
	watcher.SendMessage("timeout")
	time.Sleep(800 * time.Millisecond)

	// Check logs for timeout
	logger := watcher.GetLogger()
	results := logger.GetRecentResults(10)

	t.Logf("Checking %d recent results for timeout", len(results))
	for i, result := range results {
		t.Logf("Result %d: Success=%v, Error=%v", i, result.Success, result.Error)
		if result.Error != nil && result.Error.Error() == "context deadline exceeded" {
			atomic.AddInt32(&timeoutCount, 1)
		}
	}

	executions := atomic.LoadInt32(&executionCount)
	timeouts := atomic.LoadInt32(&timeoutCount)

	// ORIGINAL EXPECTATION: Timeout should be detected
	if timeouts < 1 {
		t.Error("Expected at least 1 timeout")
	}

	// System should recover from timeout - wait for state transition
	// After timeout, the system should return to Ready state
	maxWait := 500 * time.Millisecond
	deadline := time.Now().Add(maxWait)
	finalState := watcher.GetState()

	for time.Now().Before(deadline) && finalState != shared.StateReady {
		time.Sleep(10 * time.Millisecond)
		finalState = watcher.GetState()
	}

	if finalState != shared.StateReady {
		t.Errorf("Expected Ready state after timeout, got %s", finalState)
	}

	t.Logf("Timeout handled, executions: %d, timeouts: %d", executions, timeouts)
}
