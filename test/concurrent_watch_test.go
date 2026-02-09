package test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/HershyOrg/hershy/hersh"
	"github.com/HershyOrg/hershy/hersh/manager"
	"github.com/HershyOrg/hershy/hersh/shared"
)

// TestConcurrentWatch_MultipleWatchCall tests multiple WatchCall instances with different intervals
func TestConcurrentWatch_MultipleWatchCall(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	watch1Count := int32(0)
	watch2Count := int32(0)
	watch3Count := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		// Watch 1: 50ms interval
		hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					return atomic.AddInt32(&watch1Count, 1), true, nil
				}, nil
			},
			"watch1",
			50*time.Millisecond,
			ctx,
		)

		// Watch 2: 100ms interval (should be ~2x slower)
		hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					return atomic.AddInt32(&watch2Count, 1), true, nil
				}, nil
			},
			"watch2",
			100*time.Millisecond,
			ctx,
		)

		// Watch 3: 200ms interval (should be ~4x slower)
		hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					return atomic.AddInt32(&watch3Count, 1), true, nil
				}, nil
			},
			"watch3",
			200*time.Millisecond,
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

	// Let watches run for 2.5 seconds
	time.Sleep(2500 * time.Millisecond)

	w1 := atomic.LoadInt32(&watch1Count)
	w2 := atomic.LoadInt32(&watch2Count)
	w3 := atomic.LoadInt32(&watch3Count)

	// Verify frequency ratios (allow some variance)
	// Expected: w1:w2:w3 ≈ 2:1:0.5
	t.Logf("Watch counts - watch1: %d (50ms), watch2: %d (100ms), watch3: %d (200ms)", w1, w2, w3)

	if w1 < 20 {
		t.Errorf("Expected watch1 >= 20 updates, got %d", w1)
	}

	if w2 < 10 {
		t.Errorf("Expected watch2 >= 10 updates, got %d", w2)
	}

	if w3 < 5 {
		t.Errorf("Expected watch3 >= 5 updates, got %d", w3)
	}

	// Check approximate 2:1 ratio between watch1 and watch2
	ratio := float64(w1) / float64(w2)
	if ratio < 1.5 || ratio > 2.5 {
		t.Errorf("Expected watch1:watch2 ratio ≈ 2.0, got %.2f", ratio)
	}

	t.Logf("Test complete - frequency ratios verified")
}

// TestConcurrentWatch_WatchPlusMessages tests WatchCall and UserSig working together
// ORIGINAL TEST - checking for race condition
func TestConcurrentWatch_WatchPlusMessages(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	watchCounter := int32(0)
	messagesReceived := []string{}
	executionCount := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		atomic.AddInt32(&executionCount, 1)

		// Watch updates every 100ms
		hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					return atomic.AddInt32(&watchCounter, 1), true, nil
				}, nil
			},
			"counter",
			100*time.Millisecond,
			ctx,
		)

		// Capture messages
		if msg != nil && msg.Content != "" {
			messagesReceived = append(messagesReceived, msg.Content)
			t.Logf("Received message: %s", msg.Content)
		}

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Wait for initialization
	time.Sleep(300 * time.Millisecond)

	// Send messages
	watcher.SendMessage("msg1")
	time.Sleep(100 * time.Millisecond)
	watcher.SendMessage("msg2")
	time.Sleep(100 * time.Millisecond)
	watcher.SendMessage("msg3")

	// ORIGINAL: Check immediately without extra wait
	time.Sleep(100 * time.Millisecond)

	executions := atomic.LoadInt32(&executionCount)
	watchCount := atomic.LoadInt32(&watchCounter)

	t.Logf("Executions: %d, Watch updates: %d, Messages received: %v", executions, watchCount, messagesReceived)

	// ORIGINAL: Expect all 3 messages
	if len(messagesReceived) != 3 {
		t.Errorf("Expected 3 messages, got %d: %v", len(messagesReceived), messagesReceived)
	}

	// Verify watch continued updating
	if watchCount < 5 {
		t.Errorf("Expected at least 5 watch updates, got %d", watchCount)
	}

	t.Log("Test complete - watch and messages working together")
}

// TestConcurrentWatch_ManyWatches tests scaling with 20 concurrent watch variables
// ORIGINAL TEST - 20 watches with 30s Start() timeout
func TestConcurrentWatch_ManyWatches(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	watchCount := 20 // ORIGINAL: 20 watches
	counters := make([]int32, watchCount)
	executionCount := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		exec := atomic.AddInt32(&executionCount, 1)

		// Register all watches
		for i := 0; i < watchCount; i++ {
			idx := i // Capture for closure
			hersh.WatchCall(
				func() (manager.VarUpdateFunc, error) {
					return func(prev any) (any, bool, error) {
						return atomic.AddInt32(&counters[idx], 1), true, nil
					}, nil
				},
				"watch"+string(rune('0'+i)),
				100*time.Millisecond,
				ctx,
			)
		}

		if exec == 4 {
			t.Logf("Execution %d: All %d watches initialized", exec, watchCount)
		}

		return nil
	}

	watcher.Manage(managedFunc, "test")

	// ORIGINAL: Start() with implicit 30s timeout expectation
	startChan := make(chan error, 1)
	go func() {
		startChan <- watcher.Start()
	}()

	select {
	case err := <-startChan:
		if err != nil {
			t.Fatalf("Failed to start watcher: %v", err)
		}
		t.Log("Watcher started and reached Ready state")
	case <-time.After(30 * time.Second):
		t.Fatal("Watcher did not reach Ready state within 30 seconds")
	}
	defer watcher.Stop()

	// Let watches run
	time.Sleep(1 * time.Second)

	executions := atomic.LoadInt32(&executionCount)

	// Check all counters
	allUpdating := true
	for i := 0; i < watchCount; i++ {
		count := atomic.LoadInt32(&counters[i])
		if count < 3 {
			allUpdating = false
			t.Errorf("Watch %d only updated %d times (expected >= 3)", i, count)
		}
	}

	if allUpdating {
		t.Logf("Test complete - all %d watches updating correctly, total executions: %d", watchCount, executions)
	}
}

// TestConcurrentWatch_RapidStateChanges tests rapid watch updates and re-executions
func TestConcurrentWatch_RapidStateChanges(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	counter := int32(0)
	executionCount := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		atomic.AddInt32(&executionCount, 1)

		// Very fast watch updates (20ms)
		hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					return atomic.AddInt32(&counter, 1), true, nil
				}, nil
			},
			"rapidCounter",
			20*time.Millisecond,
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

	// Let it run for 1.2 seconds (expecting many rapid updates)
	time.Sleep(1200 * time.Millisecond)

	executions := atomic.LoadInt32(&executionCount)
	counterValue := atomic.LoadInt32(&counter)

	t.Logf("Executions: %d, Counter increments: %d", executions, counterValue)

	// With 20ms interval over 1.2s, expect ~60 counter updates
	if counterValue < 30 {
		t.Errorf("Expected at least 30 counter updates, got %d", counterValue)
	}

	// Should trigger many re-executions
	if executions < 30 {
		t.Errorf("Expected at least 30 executions, got %d", executions)
	}

	t.Log("Test complete - rapid state changes handled")
}
