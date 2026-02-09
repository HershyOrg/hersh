package test

import (
	"context"
	"testing"
	"time"

	"github.com/HershyOrg/hershy/hersh/manager"
	"github.com/HershyOrg/hershy/hersh/shared"
)

// TestManager_BasicWorkflow tests the complete Manager workflow.
func TestManager_BasicWorkflow(t *testing.T) {
	// Setup
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(10)
	logger := manager.NewLogger(100)

	// Create Reducer
	reducer := manager.NewReducer(state, signals, logger)

	// Create EffectCommander (synchronous, no channels)
	commander := manager.NewEffectCommander()

	// Create simple managed function
	executeCount := 0
	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		executeCount++
		return nil
	}

	// Create EffectHandler (synchronous, no effectCh)
	handler := manager.NewEffectHandler(
		managedFunc,
		nil, // no cleaner for this test
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start unified reducer loop (synchronous architecture)
	go reducer.RunWithEffects(ctx, commander, handler)

	// Test: Send a VarSig to trigger execution
	t.Log("Sending VarSig...")
	signals.SendVarSig(&manager.VarSig{
		ComputedTime:       time.Now(),
		TargetVarName:      "testVar",
		VarUpdateFunc:      func(prev any) (any, bool, error) { return 42, true, nil },
		IsStateIndependent: false,
	})

	// Wait for execution
	time.Sleep(200 * time.Millisecond)

	// Verify execution occurred
	if executeCount != 1 {
		t.Errorf("expected 1 execution, got %d", executeCount)
	}

	// Verify state transitioned back to Ready
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected StateReady after execution, got %s", state.GetManagerInnerState())
	}

	// Verify variable was set
	val, ok := state.VarState.Get("testVar")
	if !ok || val != 42 {
		t.Errorf("expected testVar=42, got %v", val)
	}

	logger.PrintSummary()
}

// TestManager_UserMessageFlow tests user message handling.
func TestManager_UserMessageFlow(t *testing.T) {
	// Setup
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(10)
	logger := manager.NewLogger(100)

	reducer := manager.NewReducer(state, signals, logger)
	commander := manager.NewEffectCommander()

	var receivedMessage string
	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		if msg != nil {
			receivedMessage = msg.String()
		}
		return nil
	}

	handler := manager.NewEffectHandler(
		managedFunc,
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Send user message
	t.Log("Sending UserSig...")
	signals.SendUserSig(&manager.UserSig{
		ReceivedTime: time.Now(),
		UserMessage: &shared.Message{
			Content:    "Hello, Watcher!",
			ReceivedAt: time.Now(),
		},
	})

	time.Sleep(200 * time.Millisecond)

	// Verify message was received
	if receivedMessage != "Hello, Watcher!" {
		t.Errorf("expected 'Hello, Watcher!', got %s", receivedMessage)
	}

	// Verify state returned to Ready
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected StateReady, got %s", state.GetManagerInnerState())
	}
}

// TestManager_ErrorHandling tests error control flow.
func TestManager_ErrorHandling(t *testing.T) {
	// Setup
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(10)
	logger := manager.NewLogger(100)

	reducer := manager.NewReducer(state, signals, logger)
	commander := manager.NewEffectCommander()

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		return shared.NewStopErr("intentional stop")
	}

	handler := manager.NewEffectHandler(
		managedFunc,
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Trigger execution
	t.Log("Sending VarSig to trigger StopErr...")
	signals.SendVarSig(&manager.VarSig{
		ComputedTime:       time.Now(),
		TargetVarName:      "trigger",
		VarUpdateFunc:      func(prev any) (any, bool, error) { return 1, true, nil },
		IsStateIndependent: false,
	})

	time.Sleep(200 * time.Millisecond)

	// Verify state transitioned to Stopped
	if state.GetManagerInnerState() != shared.StateStopped {
		t.Errorf("expected StateStopped, got %s", state.GetManagerInnerState())
	}

	t.Log("StopErr handled correctly")
}

// TestManager_PriorityProcessing tests signal priority ordering.
func TestManager_PriorityProcessing(t *testing.T) {
	// Setup
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(10)
	logger := manager.NewLogger(100)

	reducer := manager.NewReducer(state, signals, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Create minimal commander and handler for synchronous loop
	commander := manager.NewEffectCommander()
	handler := manager.NewEffectHandler(
		func(msg *shared.Message, ctx shared.HershContext) error { return nil },
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	// Send signals in reverse priority order
	signals.SendVarSig(&manager.VarSig{
		ComputedTime:       time.Now(),
		TargetVarName:      "var1",
		VarUpdateFunc:      func(prev any) (any, bool, error) { return 1, true, nil },
		IsStateIndependent: false,
	})

	signals.SendUserSig(&manager.UserSig{
		ReceivedTime: time.Now(),
		UserMessage:  &shared.Message{Content: "user"},
	})

	signals.SendWatcherSig(&manager.WatcherSig{
		SignalTime:  time.Now(),
		TargetState: shared.StateKilled,
		Reason:      "priority test",
	})

	// Process signals
	go reducer.RunWithEffects(ctx, commander, handler)

	time.Sleep(100 * time.Millisecond)

	// WatcherSig should be processed first
	if state.GetManagerInnerState() != shared.StateKilled {
		t.Errorf("expected StateKilled from WatcherSig priority, got %s", state.GetManagerInnerState())
	}

	// Check that WatcherSig was processed first in logs
	reduceLogs := logger.GetReduceLog()
	if len(reduceLogs) < 1 {
		t.Fatal("expected at least 1 reduce log entry")
	}

	firstLog := reduceLogs[0]
	if _, ok := firstLog.Action.Signal.(*manager.WatcherSig); !ok {
		t.Errorf("expected first processed signal to be WatcherSig, got %T", firstLog.Action.Signal)
	}

	t.Log("Priority processing verified")
}

// TestManager_MultipleVarBatching tests batching of multiple VarSigs.
func TestManager_MultipleVarBatching(t *testing.T) {
	// Setup
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(10)
	logger := manager.NewLogger(100)

	reducer := manager.NewReducer(state, signals, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	commander := manager.NewEffectCommander()
	handler := manager.NewEffectHandler(
		func(msg *shared.Message, ctx shared.HershContext) error { return nil },
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	// Send multiple VarSigs
	for i := 1; i <= 10; i++ {
		currentVal := i * 10
		signals.SendVarSig(&manager.VarSig{
			ComputedTime:       time.Now(),
			TargetVarName:      "var" + string(rune('0'+i)),
			VarUpdateFunc:      func(prev any) (any, bool, error) { return currentVal, true, nil },
			IsStateIndependent: false,
		})
	}

	go reducer.RunWithEffects(ctx, commander, handler)

	time.Sleep(100 * time.Millisecond)

	// All variables should be set
	for i := 1; i <= 10; i++ {
		varName := "var" + string(rune('0'+i))
		val, ok := state.VarState.Get(varName)
		if !ok {
			t.Errorf("expected %s to exist", varName)
			continue
		}
		if val != i*10 {
			t.Errorf("expected %s=%d, got %v", varName, i*10, val)
		}
	}

	// In new architecture: Final state should be Ready after effect execution
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected final state Ready, got %s", state.GetManagerInnerState())
	}

	t.Log("Batch VarSig processing verified")
}

// TestManager_FullCycle tests a complete execution cycle.
func TestManager_FullCycle(t *testing.T) {
	// Setup
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(10)
	logger := manager.NewLogger(100)

	reducer := manager.NewReducer(state, signals, logger)
	commander := manager.NewEffectCommander()

	executionLog := []string{}
	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		executionLog = append(executionLog, "executed")
		return nil
	}

	handler := manager.NewEffectHandler(
		managedFunc,
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Cycle 1: VarSig triggers execution
	t.Log("Cycle 1: VarSig")
	signals.SendVarSig(&manager.VarSig{
		ComputedTime:       time.Now(),
		TargetVarName:      "trigger1",
		VarUpdateFunc:      func(prev any) (any, bool, error) { return 1, true, nil },
		IsStateIndependent: false,
	})
	time.Sleep(200 * time.Millisecond)

	// Cycle 2: UserSig triggers execution
	t.Log("Cycle 2: UserSig")
	signals.SendUserSig(&manager.UserSig{
		ReceivedTime: time.Now(),
		UserMessage:  &shared.Message{Content: "trigger2"},
	})
	time.Sleep(200 * time.Millisecond)

	// Cycle 3: Another VarSig
	t.Log("Cycle 3: VarSig again")
	signals.SendVarSig(&manager.VarSig{
		ComputedTime:       time.Now(),
		TargetVarName:      "trigger3",
		VarUpdateFunc:      func(prev any) (any, bool, error) { return 3, true, nil },
		IsStateIndependent: false,
	})
	time.Sleep(200 * time.Millisecond)

	// Verify 3 executions
	if len(executionLog) != 3 {
		t.Errorf("expected 3 executions, got %d", len(executionLog))
	}

	// Verify final state is Ready
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected final state Ready, got %s", state.GetManagerInnerState())
	}

	logger.PrintSummary()
	t.Log("Full cycle test completed successfully")
}
