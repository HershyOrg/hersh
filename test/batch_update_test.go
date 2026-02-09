package test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/HershyOrg/hershy/hersh"
	"github.com/HershyOrg/hershy/hersh/manager"
	"github.com/HershyOrg/hershy/hersh/shared"
)

// TestBatchUpdate_LongExecution tests batch update behavior during a long manage execution.
// During a single 3-second manage execution:
// - Variable 'a': WatchTick increments counter every 5ms
// - Variable 'b': WatchTick appends string every 5ms
// - Variable 'c': WatchFlow receives timestamps via channel every 5ms
//
// Expected: All updates should be properly batched and applied.
// This test exposes issues where batch updates lose intermediate states.
func TestBatchUpdate_LongExecution(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	config.DefaultTimeout = 10 * time.Second // Long timeout for 3s execution
	watcher := hersh.NewWatcher(config, nil, nil)

	// Ensure watcher is stopped after test (even if test fails)
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	// Track execution count
	executionCount := int32(0)

	// Track final values
	var finalA int32
	var finalBLen int32
	var finalCCount int32

	// Track tick counts
	ticksA := int32(0)
	ticksB := int32(0)
	ticksC := int32(0)

	// Create channel for WatchFlow
	timeChan := make(chan any, 10000) // Buffered to avoid blocking

	// Goroutine to feed channel
	stopFeeding := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopFeeding:
				close(timeChan)
				return
			case <-ticker.C:
				select {
				case timeChan <- time.Now():
					atomic.AddInt32(&ticksC, 1)
				default:
					// Channel full, skip
				}
			}
		}
	}()

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		execNum := atomic.AddInt32(&executionCount, 1)
		t.Logf("[Execution %d] Started", execNum)

		// Variable A: Counter increment
		valA := hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					var current int32
					if prev == nil {
						current = 0
					} else {
						current = prev.(int32)
					}

					next := current + 1
					atomic.AddInt32(&ticksA, 1)

					return next, true, nil
				}, nil
			},
			"counterA",
			5*time.Millisecond,
			ctx,
		)

		// Variable B: String append
		valB := hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					var current string
					if prev == nil {
						current = ""
					} else {
						current = prev.(string)
					}

					next := current + "X"
					atomic.AddInt32(&ticksB, 1)

					return next, true, nil
				}, nil
			},
			"stringB",
			5*time.Millisecond,
			ctx,
		)

		// Variable C: Timestamp flow
		valC := hersh.WatchFlow(
			timeChan,
			"timestampC",
			ctx,
		)

		t.Logf("  A=%v, B_len=%v, C=%v", valA,
			func() int {
				if valB != nil {
					return len(valB.(string))
				} else {
					return 0
				}
			}(),
			valC != nil)
		time.Sleep(6 * time.Second)
		// First execution: variables are nil, just register them
		if execNum == 1 {
			t.Log("[Execution 1] Variables registered, will be updated by watch loops")
			return nil // Let VarSig signals trigger re-execution
		}

		// Second execution: should see accumulated batch updates
		if execNum == 2 {
			// Record final values from batched updates
			if valA != nil {
				atomic.StoreInt32(&finalA, valA.(int32))
			}
			if valB != nil {
				atomic.StoreInt32(&finalBLen, int32(len(valB.(string))))
			}
			if valC != nil {
				atomic.AddInt32(&finalCCount, 1)
			}

			t.Log("[Execution 2] Recorded values after batch updates, stopping test")
			return hersh.NewStopErr("test complete")
		}

		return nil
	}

	watcher.Manage(managedFunc, "test").Cleanup(func(ctx shared.HershContext) {
		close(stopFeeding)
		t.Log("Cleanup: stopped feeding channel")
	})

	t.Log("Starting watcher...")
	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for execution to complete
	time.Sleep(5 * time.Second)

	t.Log("Stopping watcher...")
	err = watcher.Stop()
	if err != nil && err.Error() != "watcher already stopped (state: Stopped)" {
		t.Logf("Stop returned: %v", err)
	}

	// Analyze results
	execCount := atomic.LoadInt32(&executionCount)
	tA := atomic.LoadInt32(&ticksA)
	tB := atomic.LoadInt32(&ticksB)
	tC := atomic.LoadInt32(&ticksC)
	fA := atomic.LoadInt32(&finalA)
	fBLen := atomic.LoadInt32(&finalBLen)
	fCCount := atomic.LoadInt32(&finalCCount)

	t.Logf("\n=== Results ===")
	t.Logf("Executions: %d", execCount)
	t.Logf("Variable A (counter):")
	t.Logf("  - ComputeFunc calls: %d", tA)
	t.Logf("  - Final value: %d", fA)
	t.Logf("  - Lost updates: %d (%.1f%%)", tA-fA, float64(tA-fA)/float64(tA)*100)

	t.Logf("Variable B (string):")
	t.Logf("  - ComputeFunc calls: %d", tB)
	t.Logf("  - Final length: %d", fBLen)
	t.Logf("  - Lost updates: %d (%.1f%%)", tB-fBLen, float64(tB-fBLen)/float64(tB)*100)

	t.Logf("Variable C (flow):")
	t.Logf("  - Channel sends: %d", tC)
	t.Logf("  - Received in managed: %d", fCCount)

	// Expectations:
	// In 3 seconds with 5ms ticks, we expect ~600 ticks per variable
	expectedTicks := int32(500) // Conservative estimate

	if tA < expectedTicks {
		t.Errorf("Too few ticks for A: expected at least %d, got %d", expectedTicks, tA)
	}
	if tB < expectedTicks {
		t.Errorf("Too few ticks for B: expected at least %d, got %d", expectedTicks, tB)
	}

	// Check if final values match tick counts
	// This is the KEY test - batch updates should preserve all increments
	if fA != tA {
		t.Errorf("❌ Variable A lost updates: expected %d, got %d (lost %d)", tA, fA, tA-fA)
		t.Errorf("   This indicates batch updates are losing intermediate states!")
	} else {
		t.Logf("✅ Variable A: All increments preserved (%d == %d)", fA, tA)
	}

	if fBLen != tB {
		t.Errorf("❌ Variable B lost updates: expected length %d, got %d (lost %d)", tB, fBLen, tB-fBLen)
		t.Errorf("   This indicates batch updates are losing intermediate states!")
	} else {
		t.Logf("✅ Variable B: All appends preserved (%d == %d)", fBLen, tB)
	}

	// For WatchFlow, we just check that we received some updates
	if fCCount < 1 {
		t.Errorf("Variable C received no updates from channel")
	}
}

// TestBatchUpdate_RapidExecutions tests batch behavior with rapid manage executions.
// This uses shorter execution times (100ms) to trigger more frequent batch processing.
func TestBatchUpdate_RapidExecutions(t *testing.T) {
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
	ticksA := int32(0)
	finalA := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		execNum := atomic.AddInt32(&executionCount, 1)

		valA := hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					var current int32
					if prev == nil {
						current = 0
					} else {
						current = prev.(int32)
					}

					atomic.AddInt32(&ticksA, 1)
					return current + 1, true, nil
				}, nil
			},
			"counter",
			5*time.Millisecond,
			ctx,
		)

		// Short execution (100ms) but still long enough to accumulate signals
		time.Sleep(100 * time.Millisecond)

		if valA != nil {
			atomic.StoreInt32(&finalA, valA.(int32))
		}

		if execNum%5 == 0 {
			t.Logf("Execution %d: counter=%v", execNum, valA)
		}

		// Stop after 10 executions
		if execNum >= 10 {
			return hersh.NewStopErr("test complete")
		}

		return nil
	}

	watcher.Manage(managedFunc, "test")

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for test to complete
	time.Sleep(3 * time.Second)

	err = watcher.Stop()
	if err != nil && err.Error() != "watcher already stopped (state: Stopped)" {
		t.Logf("Stop returned: %v", err)
	}

	ticks := atomic.LoadInt32(&ticksA)
	final := atomic.LoadInt32(&finalA)

	t.Logf("\nTotal ticks: %d", ticks)
	t.Logf("Final value: %d", final)

	if final != ticks {
		lostPercent := float64(ticks-final) / float64(ticks) * 100
		t.Errorf("Lost %.1f%% of updates (%d out of %d)", lostPercent, ticks-final, ticks)
	} else {
		t.Logf("✅ All updates preserved")
	}
}
