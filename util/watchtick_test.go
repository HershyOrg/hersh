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

	watcher := hersh.NewWatcher(config, ctx)

	tickReceived := make([]shared.TickValue, 0, 3)

	watcher.Manage(func(msg *shared.Message, runCtx shared.ManageContext) error {
		tick := util.WatchTick("test_ticker", 100*time.Millisecond, runCtx)

		if tick.IsUpdated() {
			tickReceived = append(tickReceived, tick)

			if len(tickReceived) >= 3 {
				// Got three ticks, stop the watcher
				return shared.NewStopErr("test complete")
			}
		}

		return nil
	}, "TestWatchTick", nil)

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

	watcher := hersh.NewWatcher(config, ctx)

	startTime := time.Now()
	var firstTick shared.TickValue
	receivedTick := false

	watcher.Manage(func(msg *shared.Message, runCtx shared.ManageContext) error {
		tick := util.WatchTick("immediate_ticker", 500*time.Millisecond, runCtx)

		if !tick.IsUpdated() && !receivedTick {
			firstTick = tick
			receivedTick = true
			return shared.NewStopErr("got initial tick")
		}

		return nil
	}, "TestImmediateTick", nil)

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

	// Initial tick should have TickCount=0 (as set in init)
	if firstTick.TickCount != 0 {
		t.Errorf("Initial tick: expected TickCount = 0, got %d", firstTick.TickCount)
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

	watcher := hersh.NewWatcher(config, ctx)

	tickCounts := make([]int, 0, 5)

	watcher.Manage(func(msg *shared.Message, runCtx shared.ManageContext) error {
		tick := util.WatchTick("count_ticker", 150*time.Millisecond, runCtx)

		if tick.IsUpdated() {
			tickCounts = append(tickCounts, tick.TickCount)

			// Stop after receiving 5 ticks
			if len(tickCounts) >= 5 {
				return shared.NewStopErr("test complete")
			}
		}

		return nil
	}, "TestTickCount", nil)

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

func TestWatchTick_IsUpdated(t *testing.T) {
	// Test initial tick (NotUpdated = true)
	initialTick := shared.TickValue{
		Time:       time.Now(),
		TickCount:  0,
		NotUpdated: true,
	}
	if initialTick.IsUpdated() {
		t.Error("Initial tick with NotUpdated=true should report IsUpdated() = false")
	}

	// Test updated tick (NotUpdated = false)
	updatedTick := shared.TickValue{
		Time:       time.Now(),
		TickCount:  1,
		NotUpdated: false,
	}
	if !updatedTick.IsUpdated() {
		t.Error("Updated tick with NotUpdated=false should report IsUpdated() = true")
	}
}
