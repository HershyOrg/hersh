package test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HershyOrg/hersh"
	"github.com/HershyOrg/hersh/manager"
	"github.com/HershyOrg/hersh/shared"
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
	config.ServerPort = 0                    // Random port for test isolation
	config.DefaultTimeout = 10 * time.Second // Long timeout for 3s execution
	watcher := hersh.NewWatcher(config, nil)

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

	// Create channel function for WatchFlow (new signature)
	stopFeeding := make(chan struct{})
	getTimeChannel := func(ctx context.Context) (<-chan shared.FlowValue[time.Time], error) {
		timeChan := make(chan shared.FlowValue[time.Time], 10000) // Buffered to avoid blocking

		// Goroutine to feed channel
		go func() {
			defer close(timeChan)
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-stopFeeding:
					return
				case <-ticker.C:
					select {
					case timeChan <- shared.FlowValue[time.Time]{V: time.Now(), E: nil}:
						atomic.AddInt32(&ticksC, 1)
					default:
						// Channel full, skip
					}
				}
			}
		}()

		return timeChan, nil
	}

	managedFunc := func(msg *shared.Message, ctx shared.ManageContext) error {
		execNum := atomic.AddInt32(&executionCount, 1)
		t.Logf("[Execution %d] Started", execNum)

		// Variable A: Counter increment
		valA := hersh.WatchCall[int32](
			int32(0), // Initial value
			func() (manager.VarUpdateFunc[int32], bool, error) {
				return func(prev int32) (int32, error) {
					next := prev + 1
					atomic.AddInt32(&ticksA, 1)
					return next, nil
				}, false, nil // Don't skip signal
			},
			"counterA",
			5*time.Millisecond,
			ctx,
		)

		// Variable B: String append
		valB := hersh.WatchCall[string](
			"", // Initial empty string
			func() (manager.VarUpdateFunc[string], bool, error) {
				return func(prev string) (string, error) {
					next := prev + "X"
					atomic.AddInt32(&ticksB, 1)
					return next, nil
				}, false, nil
			},
			"stringB",
			5*time.Millisecond,
			ctx,
		)

		// Variable C: Timestamp flow
		valC := hersh.WatchFlow[time.Time](
			time.Time{}, // Initial zero time
			getTimeChannel,
			"timestampC",
			ctx,
		)

		t.Logf("  A=%v, B_len=%v, C=%v", valA.Value, len(valB.Value), valC.IsUpdated())

		// First execution: variables are nil, just register them
		if execNum == 1 {
			t.Log("[Execution 1] Variables registered, will be updated by watch loops")
			time.Sleep(6 * time.Second)
			return nil // Let VarSig signals trigger re-execution
		}

		// Second execution: should see accumulated batch updates
		if execNum == 2 {
			// Record final values from batched updates
			atomic.StoreInt32(&finalA, valA.Value)
			atomic.StoreInt32(&finalBLen, int32(len(valB.Value)))
			if valC.IsUpdated() {
				atomic.AddInt32(&finalCCount, 1)
			}

			t.Log("[Execution 2] Recorded values after batch updates, stopping test")
			return hersh.NewStopErr("test complete")
		}

		return nil
	}

	watcher.Manage(managedFunc, "test", nil).Cleanup(func(ctx shared.ManageContext) {
		close(stopFeeding)
		t.Log("Cleanup: stopped feeding channel")
	})

	t.Log("Starting watcher...")
	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for execution to complete
	time.Sleep(10 * time.Second)

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
	watcher := hersh.NewWatcher(config, nil)

	// Ensure watcher is stopped after test
	t.Cleanup(func() {
		if watcher != nil {
			_ = watcher.Stop()
		}
	})

	executionCount := int32(0)
	ticksA := int32(0)
	finalA := int32(0)

	managedFunc := func(msg *shared.Message, ctx shared.ManageContext) error {
		execNum := atomic.AddInt32(&executionCount, 1)

		valA := hersh.WatchCall[int32](
			int32(0), // Initial value
			func() (manager.VarUpdateFunc[int32], bool, error) {
				return func(prev int32) (int32, error) {
					atomic.AddInt32(&ticksA, 1)
					return prev + 1, nil
				}, false, nil
			},
			"counter",
			5*time.Millisecond,
			ctx,
		)

		// Short execution (100ms) but still long enough to accumulate signals
		time.Sleep(100 * time.Millisecond)

		atomic.StoreInt32(&finalA, valA.Value)

		if execNum%5 == 0 {
			t.Logf("Execution %d: counter=%v", execNum, valA.Value)
		}

		// Stop after 10 executions
		if execNum >= 10 {
			return hersh.NewStopErr("test complete")
		}

		return nil
	}

	watcher.Manage(managedFunc, "test", nil)

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
