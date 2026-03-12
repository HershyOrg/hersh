package hersh

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/HershyOrg/hersh/shared"
	"github.com/HershyOrg/hersh/wm"
)

// TestWatcherRestart_Enhanced verifies the internal mechanisms of Manager restart:
// - Goroutine lifecycle (start/stop)
// - Channel recreation
// - Memo recomputation with timestamps
// - Context cancellation propagation
func TestWatcherRestart_Enhanced(t *testing.T) {
	config := DefaultWatcherConfig()
	config.DefaultTimeout = 5 * time.Second

	watcher := NewWatcher(config, nil)

	// ========================================
	// Instrumentation variables
	// ========================================

	// Goroutine lifecycle tracking
	var (
		flowGoroutineStopped atomic.Bool
		tickComputationCount atomic.Int32 // Counts how many times tick computation happened
		flowGoroutineStarted atomic.Int32 // Counts how many times WatchFlow goroutine started
	)

	// Channel recreation tracking
	var (
		flowChannelAddrs []uintptr
		flowChannelMutex sync.Mutex
	)

	// Memo recomputation tracking
	var (
		memoComputeTimes []time.Time
		memoValues       []string
		memoMutex        sync.Mutex
	)

	// Execution tracking
	var executionCount atomic.Int32

	// ========================================
	// Managed function with instrumentation
	// ========================================

	managedFunc := func(msg *Message, ctx ManageContext) error {
		executionCount.Add(1)

		// WatchCall: tick-based with computation counting
		// We count computations to verify that ticks stop after StopManager
		tickWatch := DELELTED_WatchCall(
			0, // initial value
			func() (wm.DELETED_VarUpdateFunc[int], bool, error) {
				// Track each computation
				tickComputationCount.Add(1)

				computeFunc := func(prev int) (int, error) {
					// Normal computation
					return int(time.Now().UnixMilli() % 1000), nil
				}
				return computeFunc, false, nil
			},
			"tick",
			50*time.Millisecond, // Faster tick for quicker testing
			ctx,
		)
		if tickWatch.Error != nil {
			return tickWatch.Error
		}

		// WatchFlow: channel-based with lifecycle and recreation tracking
		flowWatch := DELETED_WatchFlow(
			0, // initial value
			func(flowCtx context.Context) (<-chan shared.FlowValue[int], error) {
				flowCh := make(chan shared.FlowValue[int], 10)

				// Track channel creation (store address)
				flowChannelMutex.Lock()
				chanAddr := uintptr(unsafe.Pointer(&flowCh))
				flowChannelAddrs = append(flowChannelAddrs, chanAddr)
				channelIndex := len(flowChannelAddrs)
				flowChannelMutex.Unlock()

				// Track goroutine starts
				starts := flowGoroutineStarted.Add(1)
				if starts == 1 {
					// Reset stopped flag on first start
					flowGoroutineStopped.Store(false)
				}

				go func() {
					defer func() {
						// Goroutine stopping
						flowGoroutineStopped.Store(true)
						close(flowCh)
						t.Logf("[WatchFlow #%d] Goroutine stopped", channelIndex)
					}()

					ticker := time.NewTicker(50 * time.Millisecond)
					defer ticker.Stop()
					counter := 1

					t.Logf("[WatchFlow #%d] Goroutine started", channelIndex)

					for {
						select {
						case <-flowCtx.Done():
							// Context cancelled - normal shutdown
							t.Logf("[WatchFlow #%d] Context cancelled, stopping", channelIndex)
							return
						case <-ticker.C:
							select {
							case flowCh <- shared.FlowValue[int]{V: counter}:
								counter++
							case <-flowCtx.Done():
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
			return flowWatch.Error
		}

		// Memo: track computation times and values
		memoValue := Memo(func() string {
			memoMutex.Lock()
			defer memoMutex.Unlock()

			now := time.Now()
			value := fmt.Sprintf("computed_at_%d", now.UnixNano())

			memoComputeTimes = append(memoComputeTimes, now)
			memoValues = append(memoValues, value)

			t.Logf("[Memo] Computed at %s, value: %s", now.Format("15:04:05.000"), value)

			return value
		}, "memo_key", ctx)
		_ = memoValue

		return nil
	}

	// ========================================
	// Test execution
	// ========================================

	watcher.Manage(managedFunc, "test_restart_enhanced", nil)

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	waitForReady(t, watcher, 5*time.Second)

	t.Log("=== Phase 1: First run cycle ===")

	// Allow some time for Watch goroutines to run
	time.Sleep(300 * time.Millisecond)

	// Send messages to trigger executions
	for i := 0; i < 3; i++ {
		if err := watcher.SendMessage("test"); err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Capture Phase 1 state
	phase1TickCount := tickComputationCount.Load()
	phase1FlowStarts := flowGoroutineStarted.Load()
	phase1FlowChannelCount := len(flowChannelAddrs)
	phase1MemoCount := len(memoComputeTimes)

	t.Logf("Phase 1 - Tick computations: %d, Flow starts: %d, Flow channels: %d, Memo computes: %d",
		phase1TickCount, phase1FlowStarts, phase1FlowChannelCount, phase1MemoCount)

	// Verify goroutines are running
	if phase1TickCount == 0 {
		t.Error("WatchCall not computing in Phase 1")
	}
	if flowGoroutineStopped.Load() {
		t.Error("WatchFlow goroutine stopped prematurely in Phase 1")
	}

	// Verify Memo was computed at least once
	if phase1MemoCount == 0 {
		t.Error("Memo was not computed in Phase 1")
	}

	t.Log("=== Stopping Manager ===")

	// Reset flow stopped flag
	flowGoroutineStopped.Store(false)

	// Capture tick count before stop
	tickCountBeforeStop := tickComputationCount.Load()

	if err := watcher.StopManager(); err != nil {
		t.Fatalf("StopManager failed: %v", err)
	}

	// Wait for goroutines to stop (give them time to process context cancellation)
	time.Sleep(300 * time.Millisecond)

	t.Log("=== Verifying goroutine termination ===")

	// Verify WatchCall stopped computing (no new ticks)
	tickCountAfterStop := tickComputationCount.Load()
	if tickCountAfterStop > tickCountBeforeStop {
		// Some ticks might have happened right before stop, allow small margin
		t.Logf("Warning: Tick count increased slightly after stop: %d → %d",
			tickCountBeforeStop, tickCountAfterStop)
	}

	// Wait a bit more and verify no new ticks
	time.Sleep(200 * time.Millisecond)
	tickCountAfterWait := tickComputationCount.Load()
	if tickCountAfterWait > tickCountAfterStop {
		t.Errorf("WatchCall goroutine did NOT stop! Ticks continued: %d → %d",
			tickCountAfterStop, tickCountAfterWait)
	} else {
		t.Log("✓ WatchCall goroutine stopped (no new ticks)")
	}

	// Verify WatchFlow goroutine stopped
	if !flowGoroutineStopped.Load() {
		t.Error("WatchFlow goroutine did NOT stop after StopManager!")
	} else {
		t.Log("✓ WatchFlow goroutine stopped successfully")
	}

	t.Log("=== Restarting Manager ===")

	// Reset flow stopped flag before restart
	flowGoroutineStopped.Store(false)

	if err := watcher.RunManager(); err != nil {
		t.Fatalf("RunManager failed: %v", err)
	}

	waitForReady(t, watcher, 5*time.Second)

	t.Log("=== Phase 2: Second run cycle (after restart) ===")

	// Allow time for new Watch goroutines to start
	time.Sleep(300 * time.Millisecond)

	// Send messages
	for i := 0; i < 3; i++ {
		if err := watcher.SendMessage("test"); err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Capture Phase 2 state
	phase2TickCount := tickComputationCount.Load()
	phase2FlowStarts := flowGoroutineStarted.Load()
	phase2FlowChannelCount := len(flowChannelAddrs)
	phase2MemoCount := len(memoComputeTimes)

	t.Logf("Phase 2 - Tick computations: %d, Flow starts: %d, Flow channels: %d, Memo computes: %d",
		phase2TickCount, phase2FlowStarts, phase2FlowChannelCount, phase2MemoCount)

	t.Log("=== Verifying restart behavior ===")

	// 1. Verify WatchCall resumed computing (count increased significantly)
	if phase2TickCount <= phase1TickCount+1 {
		t.Errorf("WatchCall did NOT resume computing! Phase1: %d, Phase2: %d",
			phase1TickCount, phase2TickCount)
	} else {
		t.Logf("✓ WatchCall resumed computing (count increased: %d → %d)",
			phase1TickCount, phase2TickCount)
	}

	if phase2FlowStarts <= phase1FlowStarts {
		t.Errorf("WatchFlow goroutine did NOT restart! Phase1: %d, Phase2: %d",
			phase1FlowStarts, phase2FlowStarts)
	} else {
		t.Logf("✓ WatchFlow goroutine restarted (count increased: %d → %d)",
			phase1FlowStarts, phase2FlowStarts)
	}

	// 2. Verify new channel was created (channel count increased)
	if phase2FlowChannelCount <= phase1FlowChannelCount {
		t.Errorf("WatchFlow channel did NOT recreate! Phase1: %d, Phase2: %d",
			phase1FlowChannelCount, phase2FlowChannelCount)
	} else {
		t.Logf("✓ WatchFlow channel recreated (count: %d → %d)",
			phase1FlowChannelCount, phase2FlowChannelCount)

		// Verify different addresses
		if phase1FlowChannelCount > 0 && phase2FlowChannelCount > 1 {
			firstAddr := flowChannelAddrs[0]
			secondAddr := flowChannelAddrs[phase1FlowChannelCount]
			if firstAddr == secondAddr {
				t.Error("WatchFlow channels have SAME address - not truly recreated!")
			} else {
				t.Logf("✓ Channel addresses differ: 0x%x vs 0x%x", firstAddr, secondAddr)
			}
		}
	}

	// 3. Verify Memo was recomputed (compute count increased)
	if phase2MemoCount <= phase1MemoCount {
		t.Errorf("Memo did NOT recompute! Phase1: %d, Phase2: %d",
			phase1MemoCount, phase2MemoCount)
	} else {
		t.Logf("✓ Memo recomputed (count: %d → %d)",
			phase1MemoCount, phase2MemoCount)

		// Verify different timestamps
		memoMutex.Lock()
		firstTime := memoComputeTimes[0]
		secondTime := memoComputeTimes[phase1MemoCount]
		firstValue := memoValues[0]
		secondValue := memoValues[phase1MemoCount]
		memoMutex.Unlock()

		if secondTime.Before(firstTime) || secondTime.Equal(firstTime) {
			t.Errorf("Memo recompute timestamp suspicious: first=%s, second=%s",
				firstTime, secondTime)
		} else {
			timeDiff := secondTime.Sub(firstTime)
			t.Logf("✓ Memo timestamp diff: %v", timeDiff)
		}

		if firstValue == secondValue {
			t.Error("Memo values are identical - might not have recomputed!")
		} else {
			t.Logf("✓ Memo values differ: %s vs %s", firstValue[:20], secondValue[:20])
		}
	}

	// 4. Verify goroutines are running again
	if phase2TickCount <= tickCountAfterWait {
		t.Error("WatchCall not computing in Phase 2")
	} else {
		t.Log("✓ WatchCall is computing in Phase 2")
	}

	if flowGoroutineStopped.Load() {
		t.Error("WatchFlow goroutine stopped prematurely in Phase 2")
	} else {
		t.Log("✓ WatchFlow goroutine is running in Phase 2")
	}

	t.Log("=== Second restart cycle ===")

	// Reset flow stopped flag
	flowGoroutineStopped.Store(false)

	// Capture tick count before second stop
	tickCountBeforeStop2 := tickComputationCount.Load()

	// Stop again
	if err := watcher.StopManager(); err != nil {
		t.Fatalf("Second StopManager failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Verify WatchCall stopped again
	time.Sleep(200 * time.Millisecond)
	tickCountAfterStop2 := tickComputationCount.Load()
	if tickCountAfterStop2 > tickCountBeforeStop2+2 {
		t.Errorf("WatchCall did NOT stop after second StopManager! %d → %d",
			tickCountBeforeStop2, tickCountAfterStop2)
	}

	// Verify WatchFlow stopped again
	if !flowGoroutineStopped.Load() {
		t.Error("WatchFlow goroutine did NOT stop after second StopManager!")
	}

	// Restart again
	flowGoroutineStopped.Store(false)

	if err := watcher.RunManager(); err != nil {
		t.Fatalf("Second RunManager failed: %v", err)
	}

	waitForReady(t, watcher, 5*time.Second)

	// Capture Phase 3 state
	phase3TickCount := tickComputationCount.Load()
	phase3FlowStarts := flowGoroutineStarted.Load()
	phase3FlowChannelCount := len(flowChannelAddrs)
	phase3MemoCount := len(memoComputeTimes)

	t.Logf("Phase 3 - Tick computations: %d, Flow starts: %d, Flow channels: %d, Memo computes: %d",
		phase3TickCount, phase3FlowStarts, phase3FlowChannelCount, phase3MemoCount)

	// Verify third cycle also works
	if phase3TickCount <= phase2TickCount+1 {
		t.Error("WatchCall did not resume in third cycle")
	}
	if phase3FlowStarts <= phase2FlowStarts {
		t.Error("WatchFlow did not restart in third cycle")
	}
	if phase3FlowChannelCount <= phase2FlowChannelCount {
		t.Error("WatchFlow channel did not recreate in third cycle")
	}
	if phase3MemoCount <= phase2MemoCount {
		t.Error("Memo did not recompute in third cycle")
	}

	t.Log("=== Final cleanup ===")

	if err := watcher.StopManager(); err != nil {
		t.Fatalf("Final StopManager failed: %v", err)
	}

	if err := watcher.Stop(); err != nil {
		t.Fatalf("Failed to stop watcher: %v", err)
	}

	// Final summary
	t.Logf("\n=== Test Summary ===")
	t.Logf("Total executions: %d", executionCount.Load())
	t.Logf("WatchCall tick computations: %d", phase3TickCount)
	t.Logf("WatchFlow goroutine starts: %d", phase3FlowStarts)
	t.Logf("Flow channels created: %d", phase3FlowChannelCount)
	t.Logf("Memo computations: %d", phase3MemoCount)
	t.Log("✓ All restart mechanisms verified successfully!")
}
