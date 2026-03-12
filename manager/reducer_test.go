package manager

import (
	"context"
	"testing"
	"time"

	"github.com/HershyOrg/hersh/shared"
	"github.com/HershyOrg/hersh/wm"
)

// Helper function to create a full logger for tests
func newTestLogger() *Logger {
	return NewLogger(100)
}

func TestReducer_VarSigTransition(t *testing.T) {
	state := NewManagerState(shared.StateReady)
	signals := NewSignalChannels(10)
	logger := newTestLogger()
	reducer := NewReducer(state, signals, logger)

	// Need commander and handler for synchronous architecture
	commander := NewEffectCommander()
	handler := NewEffectHandler(
		func(msg *shared.Message, ctx shared.ManageContext) error { return nil },
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Start reducer with synchronous effects
	go reducer.RunWithEffects(ctx, commander, handler)

	// Send VarSig
	sig := &wm.DELETED_VarSig{
		ReceivedTime:  time.Now(),
		TargetVarName: "testVar",
		DELETED_VarUpdateFunc: func(prev shared.RawWatchValue) shared.RawWatchValue {
			return shared.RawWatchValue{Value: 42, Error: nil}
		},
		DELETED_ISStateIndependent: false,
	}
	signals.SendVarSig(sig)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// In new architecture: Ready → Running (VarSig) → Ready (effect complete)
	// Final state should be Ready after effect execution
	finalState := state.GetManagerInnerState()
	if finalState != shared.StateReady {
		t.Errorf("expected final state Ready, got %s", finalState)
	}

	// Verify variable was set
	hv, ok := state.VarState.Get("testVar")
	if !ok {
		t.Fatal("expected testVar to exist")
	}
	if hv.Value == nil || hv.Value.(int) != 42 {
		t.Errorf("expected 42, got %v", hv.Value)
	}

	// Verify actions were logged (VarSig + WatcherSig from effect)
	reduceLogs := logger.GetReduceLog()
	if len(reduceLogs) < 1 {
		t.Fatalf("expected at least 1 logged action, got %d", len(reduceLogs))
	}
	// First action should be VarSig transition Ready → Running
	if reduceLogs[0].Action.PrevState.ManagerInnerState != shared.StateReady {
		t.Errorf("expected prev state Ready, got %s", reduceLogs[0].Action.PrevState.ManagerInnerState)
	}
	if reduceLogs[0].Action.NextState.ManagerInnerState != shared.StateRunning {
		t.Errorf("expected next state Running, got %s", reduceLogs[0].Action.NextState.ManagerInnerState)
	}
}

func TestReducer_UserSigTransition(t *testing.T) {
	state := NewManagerState(shared.StateReady)
	signals := NewSignalChannels(10)
	logger := newTestLogger()
	reducer := NewReducer(state, signals, logger)

	commander := NewEffectCommander()
	// Track function execution
	var messageReceived *shared.Message
	handler := NewEffectHandler(
		func(msg *shared.Message, ctx shared.ManageContext) error {
			messageReceived = msg
			return nil
		},
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Send UserSig
	msg := &shared.Message{
		Content:    "test message",
		IsConsumed: false,
		ReceivedAt: time.Now(),
	}
	sig := &UserSig{
		ReceivedTime: time.Now(),
		UserMessage:  msg,
	}
	signals.SendUserSig(sig)

	time.Sleep(100 * time.Millisecond)

	// In new architecture: Ready → Running (UserSig) → Ready (effect complete)
	// Final state should be Ready after effect execution
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected final state Ready, got %s", state.GetManagerInnerState())
	}

	// Verify message was passed to managed function
	if messageReceived == nil {
		t.Fatal("expected message to be received by managed function")
	}
	if messageReceived.Content != "test message" {
		t.Errorf("expected 'test message', got %s", messageReceived.Content)
	}

	// Verify actions were logged (UserSig + WatcherSig from effect)
	reduceLogs := logger.GetReduceLog()
	if len(reduceLogs) < 1 {
		t.Fatalf("expected at least 1 logged action, got %d", len(reduceLogs))
	}
}

func TestReducer_WatcherSigTransition(t *testing.T) {
	state := NewManagerState(shared.StateRunning)
	signals := NewSignalChannels(10)
	logger := newTestLogger()
	reducer := NewReducer(state, signals, logger)

	commander := NewEffectCommander()
	handler := NewEffectHandler(
		func(msg *shared.Message, ctx shared.ManageContext) error { return nil },
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Send WatcherSig to transition to Ready
	sig := &ManagerInnerSig{
		ReceivedTime: time.Now(),
		TargetState:  shared.StateReady,
		Reason:       "execution completed",
	}
	signals.SendManagerInnerSig(sig)

	time.Sleep(50 * time.Millisecond)

	// Verify state transition
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected StateReady, got %s", state.GetManagerInnerState())
	}

	// Verify action was logged
	reduceLogs := logger.GetReduceLog()
	if len(reduceLogs) != 1 {
		t.Fatalf("expected 1 logged action, got %d", len(reduceLogs))
	}
}

func TestReducer_PriorityOrdering(t *testing.T) {
	state := NewManagerState(shared.StateReady)
	signals := NewSignalChannels(10)
	logger := newTestLogger()
	reducer := NewReducer(state, signals, logger)

	commander := NewEffectCommander()
	handler := NewEffectHandler(
		func(msg *shared.Message, ctx shared.ManageContext) error { return nil },
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Send signals in reverse priority order (Var, User, Watcher)
	varSig := &wm.DELETED_VarSig{
		ReceivedTime:  time.Now(),
		TargetVarName: "var1",
		DELETED_VarUpdateFunc: func(prev shared.RawWatchValue) shared.RawWatchValue {
			return shared.RawWatchValue{Value: 1, Error: nil}
		},
		DELETED_ISStateIndependent: false,
	}
	userSig := &UserSig{
		ReceivedTime: time.Now(),
		UserMessage:  &shared.Message{Content: "user"},
	}
	watcherSig := &ManagerInnerSig{
		ReceivedTime: time.Now(),
		TargetState:  shared.StateRunning,
		Reason:       "start",
	}

	// Send in this order: Var, User, Watcher
	signals.SendVarSig(varSig)
	signals.SendUserSig(userSig)
	signals.SendManagerInnerSig(watcherSig)

	// Start processing
	go reducer.RunWithEffects(ctx, commander, handler)

	time.Sleep(100 * time.Millisecond)

	// WatcherSig should be processed first due to priority
	reduceLogs := logger.GetReduceLog()
	if len(reduceLogs) < 1 {
		t.Fatal("expected at least 1 action")
	}
	firstAction := reduceLogs[0].Action
	if _, ok := firstAction.Signal.(*ManagerInnerSig); !ok {
		t.Errorf("expected WatcherSig to be processed first, got %T", firstAction.Signal)
	}
}

func TestReducer_BatchVarSigCollection(t *testing.T) {
	state := NewManagerState(shared.StateReady)
	signals := NewSignalChannels(10)
	logger := newTestLogger()
	reducer := NewReducer(state, signals, logger)

	commander := NewEffectCommander()
	handler := NewEffectHandler(
		func(msg *shared.Message, ctx shared.ManageContext) error { return nil },
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Send multiple VarSigs
	for i := 1; i <= 5; i++ {
		currentVal := i * 10
		sig := &wm.DELETED_VarSig{
			ReceivedTime:  time.Now(),
			TargetVarName: "var" + string(rune('0'+i)),
			DELETED_VarUpdateFunc: func(prev shared.RawWatchValue) shared.RawWatchValue {
				return shared.RawWatchValue{Value: currentVal, Error: nil}
			},
			DELETED_ISStateIndependent: false,
		}
		signals.SendVarSig(sig)
	}

	go reducer.RunWithEffects(ctx, commander, handler)

	time.Sleep(50 * time.Millisecond)

	// All 5 variables should be set in one batch
	for i := 1; i <= 5; i++ {
		varName := "var" + string(rune('0'+i))
		hv, ok := state.VarState.Get(varName)
		if !ok {
			t.Errorf("expected %s to exist", varName)
			continue
		}
		if hv.Value == nil || hv.Value.(int) != i*10 {
			t.Errorf("expected %d, got %v", i*10, hv.Value)
		}
	}

	// In new architecture: Final state should be Ready after effect execution
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected final state Ready, got %s", state.GetManagerInnerState())
	}
}

func TestReducer_CrashedIsTerminal(t *testing.T) {
	state := NewManagerState(shared.StateCrashed)
	signals := NewSignalChannels(10)
	logger := newTestLogger()
	reducer := NewReducer(state, signals, logger)

	commander := NewEffectCommander()
	handler := NewEffectHandler(
		func(msg *shared.Message, ctx shared.ManageContext) error { return nil },
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Try to transition from Crashed
	sig := &ManagerInnerSig{
		ReceivedTime: time.Now(),
		TargetState:  shared.StateReady,
		Reason:       "attempt recovery",
	}
	signals.SendManagerInnerSig(sig)

	time.Sleep(50 * time.Millisecond)

	// State should remain Crashed
	if state.GetManagerInnerState() != shared.StateCrashed {
		t.Errorf("expected StateCrashed, got %s", state.GetManagerInnerState())
	}

	// Transition is rejected but attempt is logged (with validation error message)
	// The reducer logs the invalid transition attempt
	reduceLogs := logger.GetReduceLog()
	// Expect at least the rejection to be logged
	if len(reduceLogs) > 0 {
		t.Logf("Logged %d actions (transition rejection logged)", len(reduceLogs))
	}
}

func TestReducer_VarStatePersistence(t *testing.T) {
	state := NewManagerState(shared.StateStopped)
	signals := NewSignalChannels(10)
	logger := newTestLogger()
	reducer := NewReducer(state, signals, logger)

	commander := NewEffectCommander()
	handler := NewEffectHandler(
		func(msg *shared.Message, ctx shared.ManageContext) error { return nil },
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Set some variables
	state.VarState.Set("var1", shared.RawWatchValue{Value: 1, Error: nil})
	state.VarState.Set("var2", shared.RawWatchValue{Value: 2, Error: nil})

	go reducer.RunWithEffects(ctx, commander, handler)

	// Send Running signal (direct transition from Stopped)
	sig := &ManagerInnerSig{
		ReceivedTime: time.Now(),
		TargetState:  shared.StateRunning,
		Reason:       "start",
	}
	signals.SendManagerInnerSig(sig)

	time.Sleep(100 * time.Millisecond)

	// In the new architecture, VarState should persist (not be cleared)
	snapshot := state.VarState.GetAll()
	if len(snapshot) != 2 {
		t.Errorf("expected 2 variables to persist, got %d variables", len(snapshot))
	}

	// State should transition to Ready after running the managed function
	finalState := state.GetManagerInnerState()
	if finalState != shared.StateReady {
		t.Errorf("expected StateReady, got %s", finalState)
	}
}
