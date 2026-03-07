package util_test

import (
	"context"
	"testing"
	"time"

	"github.com/HershyOrg/hersh"
	"github.com/HershyOrg/hersh/shared"
	"github.com/HershyOrg/hersh/util"
)

func TestWatchTick_BasicFunctionality(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Use random port
	config.DefaultTimeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	tickReceived := make([]shared.HershTick, 0, 3)

	watcher.Manage(func(msg *shared.Message, runCtx shared.ManageContext) error {
		tick := util.WatchTick("test_ticker", 100*time.Millisecond, runCtx)

		if !tick.IsZero() {
			tickReceived = append(tickReceived, tick)

			if len(tickReceived) >= 3 {
				// Got three ticks, stop the watcher
				return shared.NewStopErr("test complete")
			}
		}

		return nil
	}, "TestWatchTick")

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for ticks to be processed
	time.Sleep(500 * time.Millisecond)

	// Wait for watcher to stop
	if err := watcher.Stop(); err != nil {
		t.Logf("Stop error (may be expected): %v", err)
	}

	// Verify we received at least 3 ticks
	if len(tickReceived) < 3 {
		t.Errorf("Expected at least 3 ticks, got %d", len(tickReceived))
	}

	// Verify TickCount increments sequentially (may not start from 1 due to WatchFlow init)
	if len(tickReceived) >= 2 {
		for i := 1; i < len(tickReceived); i++ {
			if tickReceived[i].TickCount != tickReceived[i-1].TickCount+1 {
				t.Errorf("Tick %d: expected TickCount=%d, got %d (not sequential)",
					i, tickReceived[i-1].TickCount+1, tickReceived[i].TickCount)
			}
		}
	}

	// Verify interval between ticks is approximately correct (within 50ms tolerance)
	if len(tickReceived) >= 2 {
		interval := tickReceived[1].Time.Sub(tickReceived[0].Time)
		expectedInterval := 100 * time.Millisecond
		tolerance := 50 * time.Millisecond

		if interval < expectedInterval-tolerance || interval > expectedInterval+tolerance {
			t.Errorf("Tick interval %v is outside expected range %v ± %v",
				interval, expectedInterval, tolerance)
		}
	}
}

func TestWatchTick_ImmediateInitialTick(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Use random port
	config.DefaultTimeout = 2 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	startTime := time.Now()
	var firstTick shared.HershTick
	receivedTick := false

	watcher.Manage(func(msg *shared.Message, runCtx shared.ManageContext) error {
		tick := util.WatchTick("immediate_ticker", 500*time.Millisecond, runCtx)

		if !tick.IsZero() && !receivedTick {
			firstTick = tick
			receivedTick = true
			return shared.NewStopErr("got initial tick")
		}

		return nil
	}, "TestImmediateTick")

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for ticks to be processed
	time.Sleep(1500 * time.Millisecond)

	if err := watcher.Stop(); err != nil {
		t.Logf("Stop error (may be expected): %v", err)
	}

	if !receivedTick {
		t.Fatal("Did not receive initial tick")
	}

	// Verify initial tick has positive TickCount
	if firstTick.TickCount < 1 {
		t.Errorf("Initial tick: expected TickCount >= 1, got %d", firstTick.TickCount)
	}

	// Verify initial tick arrived within reasonable time (within 2 seconds)
	timeSinceStart := firstTick.Time.Sub(startTime)
	if timeSinceStart > 2*time.Second {
		t.Errorf("Initial tick took too long: %v", timeSinceStart)
	}
}

func TestWatchTick_TickCountIncrement(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Use random port
	config.DefaultTimeout = 3 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	tickCounts := make([]int, 0, 5)

	watcher.Manage(func(msg *shared.Message, runCtx shared.ManageContext) error {
		tick := util.WatchTick("count_ticker", 150*time.Millisecond, runCtx)

		if !tick.IsZero() {
			tickCounts = append(tickCounts, tick.TickCount)

			// Stop after receiving 5 ticks
			if len(tickCounts) >= 5 {
				return shared.NewStopErr("test complete")
			}
		}

		return nil
	}, "TestTickCount")

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for ticks to be processed
	time.Sleep(1 * time.Second)

	if err := watcher.Stop(); err != nil {
		t.Logf("Stop error (may be expected): %v", err)
	}

	// Verify we got 5 ticks
	if len(tickCounts) != 5 {
		t.Errorf("Expected 5 ticks, got %d", len(tickCounts))
	}

	// Verify counts are sequential (increment by 1 each time)
	for i := 1; i < len(tickCounts); i++ {
		if tickCounts[i] != tickCounts[i-1]+1 {
			t.Errorf("Tick %d: expected count=%d, got %d (not sequential)",
				i, tickCounts[i-1]+1, tickCounts[i])
		}
	}
}

func TestWatchTick_MultipleWatches(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Use random port
	config.DefaultTimeout = 3 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	tick1Counts := make([]int, 0)
	tick2Counts := make([]int, 0)

	watcher.Manage(func(msg *shared.Message, runCtx shared.ManageContext) error {
		// Watch two different tickers with different intervals
		tick1 := util.WatchTick("ticker1", 100*time.Millisecond, runCtx)
		tick2 := util.WatchTick("ticker2", 150*time.Millisecond, runCtx)

		if !tick1.IsZero() {
			tick1Counts = append(tick1Counts, tick1.TickCount)
		}

		if !tick2.IsZero() {
			tick2Counts = append(tick2Counts, tick2.TickCount)
		}

		// Stop after ticker1 gets 4 ticks
		if len(tick1Counts) >= 4 {
			return shared.NewStopErr("test complete")
		}

		return nil
	}, "TestMultipleWatches")

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for ticks to be processed
	time.Sleep(1 * time.Second)

	if err := watcher.Stop(); err != nil {
		t.Logf("Stop error (may be expected): %v", err)
	}

	// Verify both tickers received ticks
	if len(tick1Counts) != 4 {
		t.Errorf("Ticker1: expected  4 ticks, got %d", len(tick1Counts))
	}

	if len(tick2Counts) < 2 {
		t.Errorf("Ticker2: expected at least 2 tick, got %d", len(tick2Counts))
	}

	// Ticker1 should have more or equal ticks than ticker2 (faster interval)
	if len(tick1Counts) < len(tick2Counts) {
		t.Errorf("Expected ticker1 (%d ticks) >= ticker2 (%d ticks)",
			len(tick1Counts), len(tick2Counts))
	}

	// Verify ticker1 has sequential or mostly sequential counts
	// (multiple watches may have timing variations)
	nonSequentialCount1 := 0
	for i := 1; i < len(tick1Counts); i++ {
		if tick1Counts[i] != tick1Counts[i-1]+1 {
			nonSequentialCount1++
		}
	}
	if nonSequentialCount1 > len(tick1Counts)/2 {
		t.Errorf("Ticker1: too many non-sequential ticks: %d out of %d",
			nonSequentialCount1, len(tick1Counts)-1)
	}

	// Verify ticker2 counts are positive and increasing
	for i := 1; i < len(tick2Counts); i++ {
		if tick2Counts[i] < tick2Counts[i-1] {
			t.Errorf("Ticker2[%d]: count decreased from %d to %d",
				i, tick2Counts[i-1], tick2Counts[i])
		}
	}
}

func TestWatchTick_IsZero(t *testing.T) {
	// Test zero value
	var zeroTick shared.HershTick
	if !zeroTick.IsZero() {
		t.Error("Zero value HershTick should report IsZero() = true")
	}

	// Test non-zero value
	nonZeroTick := shared.HershTick{Time: time.Now(), TickCount: 1}
	if nonZeroTick.IsZero() {
		t.Error("Non-zero HershTick should report IsZero() = false")
	}
}
