package test

import (
	"context"
	"testing"
	"time"

	"github.com/HershyOrg/hersh"
	"github.com/HershyOrg/hersh/manager"
	"github.com/HershyOrg/hersh/shared"
	"github.com/HershyOrg/hersh/util"
)

// TestHershValue_IsTriggered tests the IsTriggered method on HershValue
func TestHershValue_IsTriggered(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Use random port
	config.DefaultTimeout = 2 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	// Track which variables were triggered
	var priceTriggered, volumeTriggered bool
	var priceValue, volumeValue float64

	watcher.Manage(func(msg *shared.Message, runCtx shared.ManageContext) error {
		// Watch two variables with generic types
		price, _ := hersh.WatchCall[float64](
			func() (manager.VarUpdateFunc[float64], bool, error) {
				return func(prev float64) (float64, error) {
					return 100.0, nil
				}, false, nil
			},
			"price",
			100*time.Millisecond,
			runCtx,
		)

		volume, _ := hersh.WatchCall[float64](
			func() (manager.VarUpdateFunc[float64], bool, error) {
				return func(prev float64) (float64, error) {
					return 50.0, nil
				}, false, nil
			},
			"volume",
			100*time.Millisecond,
			runCtx,
		)

		// Use IsTriggered() convenience method
		if price.IsTriggered(runCtx) {
			priceTriggered = true
			priceValue = price.Value // Type-safe, no assertion needed
		}

		if volume.IsTriggered(runCtx) {
			volumeTriggered = true
			volumeValue = volume.Value // Type-safe, no assertion needed
		}

		// Stop after both triggered
		if priceTriggered && volumeTriggered {
			return shared.NewStopErr("test complete")
		}

		return nil
	}, "TestHershValue_IsTriggered")

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for ticks to be processed
	time.Sleep(500 * time.Millisecond)

	if err := watcher.Stop(); err != nil {
		t.Logf("Stop error (may be expected): %v", err)
	}

	// Verify both variables were triggered
	if !priceTriggered {
		t.Error("Expected price to be triggered")
	}

	if !volumeTriggered {
		t.Error("Expected volume to be triggered")
	}

	// Verify values were set correctly
	if priceValue != 100.0 {
		t.Errorf("Expected price value=100.0, got %f", priceValue)
	}

	if volumeValue != 50.0 {
		t.Errorf("Expected volume value=50.0, got %f", volumeValue)
	}
}

// TestHershTick_IsTriggered tests the IsTriggered method on HershTick
func TestHershTick_IsTriggered(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Use random port
	config.DefaultTimeout = 2 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	// Track which tickers were triggered
	var ticker1Triggered, ticker2Triggered bool
	var ticker1Count, ticker2Count int

	watcher.Manage(func(msg *shared.Message, runCtx shared.ManageContext) error {
		// Watch two tickers with different intervals
		tick1 := util.WatchTick("ticker1", 100*time.Millisecond, runCtx)
		tick2 := util.WatchTick("ticker2", 150*time.Millisecond, runCtx)

		// Use IsTriggered() convenience method
		if tick1.IsTriggered(runCtx) && !tick1.IsZero() {
			ticker1Triggered = true
			ticker1Count = tick1.TickCount
		}

		if tick2.IsTriggered(runCtx) && !tick2.IsZero() {
			ticker2Triggered = true
			ticker2Count = tick2.TickCount
		}

		// Stop after both triggered at least once
		if ticker1Triggered && ticker2Triggered {
			return shared.NewStopErr("test complete")
		}

		return nil
	}, "TestHershTick_IsTriggered")

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for ticks to be processed
	time.Sleep(500 * time.Millisecond)

	if err := watcher.Stop(); err != nil {
		t.Logf("Stop error (may be expected): %v", err)
	}

	// Verify both tickers were triggered
	if !ticker1Triggered {
		t.Error("Expected ticker1 to be triggered")
	}

	if !ticker2Triggered {
		t.Error("Expected ticker2 to be triggered")
	}

	// Verify tick counts are positive
	if ticker1Count < 1 {
		t.Errorf("Expected ticker1Count >= 1, got %d", ticker1Count)
	}

	if ticker2Count < 1 {
		t.Errorf("Expected ticker2Count >= 1, got %d", ticker2Count)
	}
}

// TestIsTriggered_Convenience tests the convenience of IsTriggered vs manual checking
func TestIsTriggered_Convenience(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Use random port
	config.DefaultTimeout = 2 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	// Track calls to verify both methods work identically
	var manualCheck, convenienceCheck bool

	watcher.Manage(func(msg *shared.Message, runCtx shared.ManageContext) error {
		price, _ := hersh.WatchCall[float64](
			func() (manager.VarUpdateFunc[float64], bool, error) {
				return func(prev float64) (float64, error) {
					return 42.0, nil
				}, false, nil
			},
			"price",
			100*time.Millisecond,
			runCtx,
		)

		// Manual check (old way)
		trigger := runCtx.GetTriggeredSignal()
		if trigger != nil && trigger.HasVarTrigger("price") {
			manualCheck = true
		}

		// Convenience method (new way)
		if price.IsTriggered(runCtx) {
			convenienceCheck = true
		}

		// Both should match
		if manualCheck != convenienceCheck {
			t.Errorf("Manual check (%v) != convenience check (%v)", manualCheck, convenienceCheck)
		}

		if manualCheck && convenienceCheck {
			return shared.NewStopErr("test complete")
		}

		return nil
	}, "TestIsTriggered_Convenience")

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for tick to be processed
	time.Sleep(300 * time.Millisecond)

	if err := watcher.Stop(); err != nil {
		t.Logf("Stop error (may be expected): %v", err)
	}

	// Verify both methods detected the trigger
	if !manualCheck {
		t.Error("Manual check failed to detect trigger")
	}

	if !convenienceCheck {
		t.Error("Convenience method failed to detect trigger")
	}
}
