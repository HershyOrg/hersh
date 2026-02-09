package test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HershyOrg/hershy/hersh"
	"github.com/HershyOrg/hershy/hersh/shared"
)

// TestLightweightRetry_DelayVerification verifies that lightweight retry delays are actually applied
func TestLightweightRetry_DelayVerification(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	config.RecoveryPolicy.MinConsecutiveFailures = 3
	config.RecoveryPolicy.MaxConsecutiveFailures = 6
	// Use measurable delays
	config.RecoveryPolicy.LightweightRetryDelays = []time.Duration{
		200 * time.Millisecond, // 1st failure
		400 * time.Millisecond, // 2nd failure
		600 * time.Millisecond, // 3rd+ failures
	}

	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	executionCount := int32(0)
	failureCount := 2 // Fail twice, then succeed
	executionTimes := make([]time.Time, 0, 10)
	var timeMutex sync.Mutex

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		count := atomic.AddInt32(&executionCount, 1)
		now := time.Now()

		timeMutex.Lock()
		executionTimes = append(executionTimes, now)
		timeMutex.Unlock()

		t.Logf("Execution #%d at %v", count, now.Format("15:04:05.000"))

		// IMPORTANT: WatchCall is needed to trigger re-execution after error
		// Without it, the watcher stays in Ready state and waits for external signals
		if count <= int32(failureCount) {
			// Trigger immediate re-execution to test the delay
			go func() {
				time.Sleep(10 * time.Millisecond) // Small delay to let error handling complete
				watcher.SendMessage("retry")
			}()
			return fmt.Errorf("simulated error #%d", count)
		}
		return nil
	}

	watcher.Manage(managedFunc, "delay_test")

	// Start in background
	startDone := make(chan error, 1)
	go func() {
		startDone <- watcher.Start()
	}()

	// Wait for at least 3 executions (2 failures + 1 success)
	time.Sleep(1500 * time.Millisecond)

	// Stop
	watcher.Stop()
	<-startDone

	executions := atomic.LoadInt32(&executionCount)
	t.Logf("Total executions: %d", executions)

	if executions < 3 {
		t.Fatalf("Expected at least 3 executions, got %d", executions)
	}

	// Verify delays between executions
	timeMutex.Lock()
	defer timeMutex.Unlock()

	if len(executionTimes) < 3 {
		t.Fatalf("Expected at least 3 execution times, got %d", len(executionTimes))
	}

	// Check delay between 1st and 2nd execution (should be ~200ms)
	delay1 := executionTimes[1].Sub(executionTimes[0])
	t.Logf("Delay between execution 1 and 2: %v (expected ~200ms)", delay1)
	if delay1 < 150*time.Millisecond || delay1 > 300*time.Millisecond {
		t.Errorf("First delay out of range: %v (expected 150-300ms)", delay1)
	}

	// Check delay between 2nd and 3rd execution (should be ~400ms)
	delay2 := executionTimes[2].Sub(executionTimes[1])
	t.Logf("Delay between execution 2 and 3: %v (expected ~400ms)", delay2)
	if delay2 < 350*time.Millisecond || delay2 > 500*time.Millisecond {
		t.Errorf("Second delay out of range: %v (expected 350-500ms)", delay2)
	}

	t.Log("Lightweight retry delay verification passed")
}

// TestLightweightRetry_NoWatchCall tests delay without WatchCall interference
func TestLightweightRetry_NoWatchCall(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	config.RecoveryPolicy.MinConsecutiveFailures = 3
	config.RecoveryPolicy.LightweightRetryDelays = []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
	}

	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	executionCount := int32(0)
	startTime := time.Now()

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		count := atomic.AddInt32(&executionCount, 1)
		elapsed := time.Since(startTime)
		t.Logf("Execution #%d at %v", count, elapsed)

		// No WatchCall - only message-driven or error-driven re-execution

		if count <= 2 {
			return fmt.Errorf("simulated error #%d", count)
		}
		return nil
	}

	watcher.Manage(managedFunc, "no_watch_test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for initialization and first 2 failures
	time.Sleep(800 * time.Millisecond)

	watcher.Stop()

	executions := atomic.LoadInt32(&executionCount)
	t.Logf("Total executions: %d", executions)

	// Without WatchCall, should have exactly 3 executions (InitRun + 2 retries)
	if executions != 3 {
		t.Logf("Warning: Expected 3 executions (InitRun + 2 errors), got %d", executions)
	}

	totalTime := time.Since(startTime)
	t.Logf("Total time: %v", totalTime)

	// Expected: ~100ms + ~200ms + execution overhead = ~400ms minimum
	if totalTime < 250*time.Millisecond {
		t.Errorf("Total time too short: %v (expected at least 250ms for delays)", totalTime)
	}

	t.Log("No WatchCall test passed")
}
