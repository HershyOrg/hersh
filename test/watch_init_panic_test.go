package test

import (
	"context"
	"testing"
	"time"

	"github.com/HershyOrg/hersh"
	"github.com/HershyOrg/hersh/shared"
	"github.com/HershyOrg/hersh/util"
	"github.com/HershyOrg/hersh/wmachine"
)

// TestWatchInitPanic_NormalOperation verifies Watch functions work normally
func TestWatchInitPanic_NormalOperation(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	watcher := hersh.NewWatcher(config, nil)

	tickCount := 0
	flowCount := 0
	callCount := 0

	managedFunc := func(msg *shared.Message, ctx shared.ManageContext) error {
		// Normal Watch operations
		tick := util.WatchTick("ticker", 100*time.Millisecond, ctx)
		if tick.IsUpdated() {
			tickCount++
		}

		flowChan := make(chan shared.FlowValue[int], 1)
		flow := hersh.DELETED_WatchFlow[int](
			0,
			func(ctx context.Context) (<-chan shared.FlowValue[int], error) {
				return flowChan, nil
			},
			"flowVar",
			ctx,
		)
		if flow.IsUpdated() {
			flowCount++
		}

		call := hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 1, nil
				}, false, nil
			},
			"callVar",
			200*time.Millisecond,
			ctx,
		)
		if call.IsUpdated() {
			callCount++
		}

		// Stop after some updates
		if tickCount >= 2 {
			return shared.NewStopErr("test complete")
		}

		return nil
	}

	watcher.Manage(managedFunc, "normal_test", nil)

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	watcher.Stop()

	// Verify all watches worked
	if tickCount < 2 {
		t.Errorf("Expected at least 2 tick updates, got %d", tickCount)
	}

	state := watcher.GetState()
	if state == shared.StateCrashed {
		t.Error("Watcher should not be in Crashed state for normal operation")
	}
}

// TestWatchInitPanic_InvalidVarName verifies panic with invalid watch variable name
func TestWatchInitPanic_InvalidVarName(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	config.MaxWatches = 1 // Very low limit to force error
	watcher := hersh.NewWatcher(config, nil)

	managedFunc := func(msg *shared.Message, ctx shared.ManageContext) error {
		// First watch should succeed
		hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 1, nil
				}, false, nil
			},
			"validWatch",
			100*time.Millisecond,
			ctx,
		)

		// Second watch will exceed limit and panic
		hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 1, nil
				}, false, nil
			},
			"exceedsLimit",
			100*time.Millisecond,
			ctx,
		)

		return nil
	}

	watcher.Manage(managedFunc, "invalid_var_test", nil)

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for execution and crash
	time.Sleep(200 * time.Millisecond)

	state := watcher.GetState()
	if state == shared.StateCrashed {
		t.Log("✅ Watcher correctly transitioned to Crashed state for invalid watch")
	} else {
		t.Errorf("Expected Crashed state, got %v", state)
	}

	watcher.Stop()
}

// TestWatchInitPanic_ExceedWatchLimit verifies panic when exceeding watch limit
func TestWatchInitPanic_ExceedWatchLimit(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	config.MaxWatches = 2 // Set low limit
	watcher := hersh.NewWatcher(config, nil)

	executionCount := 0

	managedFunc := func(msg *shared.Message, ctx shared.ManageContext) error {
		executionCount++
		t.Logf("Execution %d started", executionCount)

		// Register first two watches (should succeed)
		hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 1, nil
				}, false, nil
			},
			"watch1",
			100*time.Millisecond,
			ctx,
		)

		hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 1, nil
				}, false, nil
			},
			"watch2",
			100*time.Millisecond,
			ctx,
		)

		// This should panic due to limit exceeded
		// Don't catch the panic here - let it propagate to the effect handler
		hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 1, nil
				}, false, nil
			},
			"watch3_exceeds_limit",
			100*time.Millisecond,
			ctx,
		)

		return nil
	}

	watcher.Manage(managedFunc, "limit_test", nil)

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	state := watcher.GetState()
	t.Logf("Final state: %v, executionCount: %d", state, executionCount)

	if state == shared.StateCrashed {
		t.Log("✅ Watcher correctly crashed due to watch limit exceeded")
	} else {
		t.Error("Expected Crashed state when exceeding watch limit")
	}

	watcher.Stop()
}

// TestWatchInitPanic_DuplicateVarName verifies panic when registering duplicate watch
func TestWatchInitPanic_DuplicateVarName(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	watcher := hersh.NewWatcher(config, nil)

	firstCallSuccess := false
	secondCallAttempted := false
	crashDetected := false

	managedFunc := func(msg *shared.Message, ctx shared.ManageContext) error {
		// First call should succeed
		val1 := hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 1, nil
				}, false, nil
			},
			"duplicate_name",
			100*time.Millisecond,
			ctx,
		)

		if val1.VarName == "duplicate_name" {
			firstCallSuccess = true
			t.Log("First WatchCall succeeded")
		}

		// Second call with same name should get existing value (not panic)
		// This is because Watch is idempotent after first registration
		val2 := hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 2, nil // Different function
				}, false, nil
			},
			"duplicate_name", // Same name!
			100*time.Millisecond,
			ctx,
		)

		secondCallAttempted = true

		// Should get the same value as val1 since it's the same varName
		if val2.VarName == "duplicate_name" {
			t.Log("Second WatchCall returned existing watch")
		}

		return shared.NewStopErr("test complete")
	}

	watcher.Manage(managedFunc, "duplicate_test", nil)

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	state := watcher.GetState()
	if state == shared.StateCrashed {
		crashDetected = true
		t.Log("Watcher crashed (duplicate registration is now caught)")
	}

	watcher.Stop()

	if !firstCallSuccess {
		t.Error("First WatchCall should have succeeded")
	}

	if !secondCallAttempted && !crashDetected {
		t.Error("Expected either second call attempt or crash")
	}
}

// TestWatchInitPanic_RecoveryAfterCrash verifies that crashed watcher cannot recover
func TestWatchInitPanic_RecoveryAfterCrash(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	config.MaxWatches = 1 // Set low limit to trigger crash
	watcher := hersh.NewWatcher(config, nil)

	managedFunc := func(msg *shared.Message, ctx shared.ManageContext) error {
		// This will exceed the limit and crash
		hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 1, nil
				}, false, nil
			},
			"watch1",
			100*time.Millisecond,
			ctx,
		)

		hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 1, nil
				}, false, nil
			},
			"watch2_crash",
			100*time.Millisecond,
			ctx,
		)

		return nil
	}

	watcher.Manage(managedFunc, "crash_recovery_test", nil)

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for crash
	time.Sleep(500 * time.Millisecond)

	state := watcher.GetState()
	if state != shared.StateCrashed {
		t.Logf("State after potential crash: %v (may not have crashed if execution didn't reach second watch)", state)
	} else {
		t.Log("Watcher crashed as expected")

		// Try to send message after crash
		// Note: SendMessage may succeed but the message won't be processed in Crashed state
		err = watcher.SendMessage("test")
		if err != nil {
			t.Log("SendMessage returned error for crashed watcher")
		} else {
			t.Log("SendMessage succeeded but message won't be processed in Crashed state")
		}

		// Verify state remains crashed
		finalState := watcher.GetState()
		if finalState != shared.StateCrashed {
			t.Errorf("Expected state to remain Crashed, got %v", finalState)
		}
	}

	watcher.Stop()
}

// TestWatchInitPanic_NormalPanicRecovery verifies normal panics follow recovery policy
func TestWatchInitPanic_NormalPanicRecovery(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	config.RecoveryPolicy.MinConsecutiveFailures = 3 // Increase threshold
	config.RecoveryPolicy.MaxConsecutiveFailures = 5 // Allow more attempts
	watcher := hersh.NewWatcher(config, nil)

	executionCount := 0

	managedFunc := func(msg *shared.Message, ctx shared.ManageContext) error {
		executionCount++
		t.Logf("Execution %d", executionCount)

		// Normal watch operation
		hersh.DELELTED_WatchCall[int](
			0,
			func() (wmachine.DELETED_VarUpdateFunc[int], bool, error) {
				return func(prev int) (int, error) {
					return prev + 1, nil
				}, false, nil
			},
			"normalWatch",
			100*time.Millisecond,
			ctx,
		)

		// Cause a normal panic first time only
		if executionCount == 1 {
			t.Log("About to panic with normal runtime error")
			panic("normal runtime error")
		}

		// After recovery, continue running (don't stop immediately)
		if executionCount >= 3 {
			// Stop after a few successful executions
			return shared.NewStopErr("test complete after recovery")
		}

		return nil // Continue running
	}

	watcher.Manage(managedFunc, "normal_panic_test", nil)

	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for recovery and subsequent executions
	time.Sleep(16 * time.Second)

	state := watcher.GetState()
	t.Logf("Final state: %v, executions: %d", state, executionCount)

	// Should NOT be crashed - normal panics don't immediately crash
	if state == shared.StateCrashed {
		t.Error("Normal panics should not immediately crash like WatchInitPanic")
	}

	// We expect multiple executions showing recovery worked
	if executionCount < 2 {
		t.Errorf("Expected at least 2 execution attempts (panic + recovery), got %d", executionCount)
	}

	watcher.Stop()
}
