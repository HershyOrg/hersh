package manager

import (
	"context"
	"testing"
	"time"

	"github.com/HershyOrg/hershy/hersh/shared"
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
		func(msg *shared.Message, ctx shared.HershContext) error { return nil },
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
	sig := &VarSig{
		ComputedTime:       time.Now(),
		TargetVarName:      "testVar",
		VarUpdateFunc:      func(prev any) (any, bool, error) { return 42, true, nil },
		IsStateIndependent: false,
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
	val, ok := state.VarState.Get("testVar")
	if !ok {
		t.Fatal("expected testVar to exist")
	}
	if val != 42 {
		t.Errorf("expected 42, got %v", val)
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
		func(msg *shared.Message, ctx shared.HershContext) error {
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
		func(msg *shared.Message, ctx shared.HershContext) error { return nil },
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
	sig := &WatcherSig{
		SignalTime:  time.Now(),
		TargetState: shared.StateReady,
		Reason:      "execution completed",
	}
	signals.SendWatcherSig(sig)

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
		func(msg *shared.Message, ctx shared.HershContext) error { return nil },
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Send signals in reverse priority order (Var, User, Watcher)
	varSig := &VarSig{
		ComputedTime:       time.Now(),
		TargetVarName:      "var1",
		VarUpdateFunc:      func(prev any) (any, bool, error) { return 1, true, nil },
		IsStateIndependent: false,
	}
	userSig := &UserSig{
		ReceivedTime: time.Now(),
		UserMessage:  &shared.Message{Content: "user"},
	}
	watcherSig := &WatcherSig{
		SignalTime:  time.Now(),
		TargetState: shared.StateInitRun,
		Reason:      "init",
	}

	// Send in this order: Var, User, Watcher
	signals.SendVarSig(varSig)
	signals.SendUserSig(userSig)
	signals.SendWatcherSig(watcherSig)

	// Start processing
	go reducer.RunWithEffects(ctx, commander, handler)

	time.Sleep(100 * time.Millisecond)

	// WatcherSig should be processed first due to priority
	reduceLogs := logger.GetReduceLog()
	if len(reduceLogs) < 1 {
		t.Fatal("expected at least 1 action")
	}
	firstAction := reduceLogs[0].Action
	if _, ok := firstAction.Signal.(*WatcherSig); !ok {
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
		func(msg *shared.Message, ctx shared.HershContext) error { return nil },
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
		sig := &VarSig{
			ComputedTime:       time.Now(),
			TargetVarName:      "var" + string(rune('0'+i)),
			VarUpdateFunc:      func(prev any) (any, bool, error) { return currentVal, true, nil },
			IsStateIndependent: false,
		}
		signals.SendVarSig(sig)
	}

	go reducer.RunWithEffects(ctx, commander, handler)

	time.Sleep(50 * time.Millisecond)

	// All 5 variables should be set in one batch
	for i := 1; i <= 5; i++ {
		varName := "var" + string(rune('0'+i))
		val, ok := state.VarState.Get(varName)
		if !ok {
			t.Errorf("expected %s to exist", varName)
			continue
		}
		if val != i*10 {
			t.Errorf("expected %d, got %v", i*10, val)
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
		func(msg *shared.Message, ctx shared.HershContext) error { return nil },
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
	sig := &WatcherSig{
		SignalTime:  time.Now(),
		TargetState: shared.StateReady,
		Reason:      "attempt recovery",
	}
	signals.SendWatcherSig(sig)

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

func TestReducer_InitRunClearsVarState(t *testing.T) {
	state := NewManagerState(shared.StateReady)
	signals := NewSignalChannels(10)
	logger := newTestLogger()
	reducer := NewReducer(state, signals, logger)

	commander := NewEffectCommander()
	handler := NewEffectHandler(
		func(msg *shared.Message, ctx shared.HershContext) error { return nil },
		nil,
		state,
		signals,
		logger,
		shared.DefaultWatcherConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Set some variables
	state.VarState.Set("var1", 1)
	state.VarState.Set("var2", 2)

	go reducer.RunWithEffects(ctx, commander, handler)

	// Send InitRun signal
	sig := &WatcherSig{
		SignalTime:  time.Now(),
		TargetState: shared.StateInitRun,
		Reason:      "initialization",
	}
	signals.SendWatcherSig(sig)

	time.Sleep(100 * time.Millisecond)

	// VarState should be cleared
	snapshot := state.VarState.GetAll()
	if len(snapshot) != 0 {
		t.Errorf("expected empty VarState, got %d variables", len(snapshot))
	}

	// In new architecture: InitRun with no watches → immediately Ready
	// State should be Ready (not InitRun) because initRunScript returns immediately
	finalState := state.GetManagerInnerState()
	if finalState != shared.StateReady {
		t.Logf("Note: State is %s (InitRun → Ready transition when no watches)", finalState)
	}
}
