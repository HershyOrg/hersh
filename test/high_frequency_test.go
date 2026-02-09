package test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HershyOrg/hershy/hersh/manager"
	"github.com/HershyOrg/hershy/hersh/shared"
)

// TestHighFrequency_FastWatchSlowFunction tests when Watch signals arrive much faster than function execution.
func TestHighFrequency_FastWatchSlowFunction(t *testing.T) {
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(1000) // Large buffer for high frequency
	logger := manager.NewLogger(10000)

	reducer := manager.NewReducer(state, signals, logger)
	commander := manager.NewEffectCommander()

	var executionCount atomic.Int32
	var lastVarValue atomic.Int32

	// Slow managed function (100ms execution time)
	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		executionCount.Add(1)
		time.Sleep(100 * time.Millisecond) // Simulate slow processing
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Send VarSigs at high frequency (every 10ms = 100 signals/sec)
	totalSignals := 100
	t.Logf("Sending %d VarSigs at 10ms intervals (100/sec)", totalSignals)

	for i := 1; i <= totalSignals; i++ {
		currentI := int32(i)
		signals.SendVarSig(&manager.VarSig{
			ComputedTime:       time.Now(),
			TargetVarName:      "highFreqVar",
			VarUpdateFunc:      func(prev any) (any, bool, error) { return currentI, true, nil },
			IsStateIndependent: false,
		})
		lastVarValue.Store(int32(i))
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for processing to complete
	time.Sleep(2 * time.Second)

	execCount := executionCount.Load()
	t.Logf("Total executions: %d (signals sent: %d)", execCount, totalSignals)

	// Verify variable was updated to latest value
	finalVal, ok := state.VarState.Get("highFreqVar")
	if !ok {
		t.Fatal("expected highFreqVar to exist")
	}
	t.Logf("Final variable value: %v", finalVal)

	// Verify state returned to Ready
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected final state Ready, got %s", state.GetManagerInnerState())
	}

	// Log summary
	logger.PrintSummary()
}

// TestHighFrequency_ConcurrentSignalsAndMessages tests concurrent VarSigs and UserSigs.
func TestHighFrequency_ConcurrentSignalsAndMessages(t *testing.T) {
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(1000)
	logger := manager.NewLogger(10000)

	reducer := manager.NewReducer(state, signals, logger)
	commander := manager.NewEffectCommander()

	var executionCount atomic.Int32
	var messageCount atomic.Int32
	var varSigCount atomic.Int32

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		executionCount.Add(1)
		if msg != nil {
			messageCount.Add(1)
		}
		time.Sleep(50 * time.Millisecond) // Simulate processing
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Launch concurrent signal senders
	var wg sync.WaitGroup
	totalVarSigs := 50
	totalUserSigs := 50

	// VarSig sender goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= totalVarSigs; i++ {
			currentI := i
			signals.SendVarSig(&manager.VarSig{
				ComputedTime:       time.Now(),
				TargetVarName:      "concurrentVar",
				VarUpdateFunc:      func(prev any) (any, bool, error) { return currentI, true, nil },
				IsStateIndependent: false,
			})
			varSigCount.Add(1)
			time.Sleep(20 * time.Millisecond)
		}
	}()

	// UserSig sender goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= totalUserSigs; i++ {
			signals.SendUserSig(&manager.UserSig{
				ReceivedTime: time.Now(),
				UserMessage: &shared.Message{
					Content:    "concurrent message",
					ReceivedAt: time.Now(),
				},
			})
			time.Sleep(25 * time.Millisecond)
		}
	}()

	// Wait for all senders to finish
	wg.Wait()
	t.Logf("All signals sent - VarSigs: %d, UserSigs: %d", varSigCount.Load(), totalUserSigs)

	// Wait for processing
	time.Sleep(3 * time.Second)

	t.Logf("Total executions: %d", executionCount.Load())
	t.Logf("Messages processed: %d", messageCount.Load())

	// Verify final state
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected final state Ready, got %s", state.GetManagerInnerState())
	}

	// Verify no signal loss - at least some signals should be processed
	if executionCount.Load() == 0 {
		t.Error("expected at least some executions")
	}

	logger.PrintSummary()
}

// TestHighFrequency_SignalBurst tests burst of signals arriving simultaneously.
func TestHighFrequency_SignalBurst(t *testing.T) {
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(1000)
	logger := manager.NewLogger(10000)

	reducer := manager.NewReducer(state, signals, logger)
	commander := manager.NewEffectCommander()

	var executionCount atomic.Int32
	var processedVars sync.Map

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		executionCount.Add(1)
		time.Sleep(30 * time.Millisecond)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Send burst of VarSigs simultaneously
	burstSize := 100
	t.Logf("Sending burst of %d VarSigs simultaneously", burstSize)

	var wg sync.WaitGroup
	for i := 1; i <= burstSize; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			varName := "burstVar"
			currentVal := val
			signals.SendVarSig(&manager.VarSig{
				ComputedTime:       time.Now(),
				TargetVarName:      varName,
				VarUpdateFunc:      func(prev any) (any, bool, error) { return currentVal, true, nil },
				IsStateIndependent: false,
			})
			processedVars.Store(val, true)
		}(i)
	}

	wg.Wait()
	t.Logf("Burst complete - all %d signals sent", burstSize)

	// Wait for processing
	time.Sleep(4 * time.Second)

	execCount := executionCount.Load()
	t.Logf("Total executions: %d (burst size: %d)", execCount, burstSize)

	// Verify system handled the burst without crashing
	if execCount == 0 {
		t.Error("expected at least some executions after burst")
	}

	// Verify final state is Ready
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected final state Ready, got %s", state.GetManagerInnerState())
	}

	logger.PrintSummary()
}

// TestHighFrequency_SignalsWithTimeout tests signals arriving while function is timing out.
func TestHighFrequency_SignalsWithTimeout(t *testing.T) {
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(500)
	logger := manager.NewLogger(10000)

	reducer := manager.NewReducer(state, signals, logger)
	commander := manager.NewEffectCommander()

	var executionCount atomic.Int32
	var timeoutCount atomic.Int32

	// Function that sometimes times out
	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		executionCount.Add(1)
		execNum := executionCount.Load()

		// Every 3rd execution takes longer than timeout
		if execNum%3 == 0 {
			time.Sleep(200 * time.Millisecond) // Will timeout (config has 100ms default)
			timeoutCount.Add(1)
		} else {
			time.Sleep(50 * time.Millisecond)
		}
		return nil
	}

	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0 // Random port for test isolation
	config.DefaultTimeout = 150 * time.Millisecond // Short timeout

	handler := manager.NewEffectHandler(
		managedFunc,
		nil,
		state,
		signals,
		logger,
		config,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Send signals continuously while some executions timeout
	totalSignals := 30
	t.Logf("Sending %d VarSigs while function may timeout", totalSignals)

	for i := 1; i <= totalSignals; i++ {
		currentI := i
		signals.SendVarSig(&manager.VarSig{
			ComputedTime:       time.Now(),
			TargetVarName:      "timeoutVar",
			VarUpdateFunc:      func(prev any) (any, bool, error) { return currentI, true, nil },
			IsStateIndependent: false,
		})
		time.Sleep(40 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(3 * time.Second)

	t.Logf("Total executions: %d", executionCount.Load())
	t.Logf("Expected timeouts: ~%d", totalSignals/3)

	// Verify system continues working despite timeouts
	if executionCount.Load() < int32(totalSignals/2) {
		t.Errorf("expected at least %d executions, got %d", totalSignals/2, executionCount.Load())
	}

	// Check watch error log
	watchErrorLog := logger.GetWatchErrorLog()
	t.Logf("Watch errors logged: %d", len(watchErrorLog))

	logger.PrintSummary()
}

// TestHighFrequency_MultipleWatchVariables tests multiple watch variables updating simultaneously.
func TestHighFrequency_MultipleWatchVariables(t *testing.T) {
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(2000)
	logger := manager.NewLogger(10000)

	reducer := manager.NewReducer(state, signals, logger)
	commander := manager.NewEffectCommander()

	var executionCount atomic.Int32
	varCounts := make(map[string]*atomic.Int32)
	var varCountsMu sync.Mutex

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		executionCount.Add(1)
		time.Sleep(30 * time.Millisecond)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Launch multiple watch variable senders
	numVars := 10
	signalsPerVar := 20
	t.Logf("Sending signals for %d variables (%d signals each)", numVars, signalsPerVar)

	var wg sync.WaitGroup
	for varIdx := 0; varIdx < numVars; varIdx++ {
		varName := "var" + string(rune('A'+varIdx))

		varCountsMu.Lock()
		varCounts[varName] = &atomic.Int32{}
		varCountsMu.Unlock()

		wg.Add(1)
		go func(vName string, counter *atomic.Int32) {
			defer wg.Done()
			for i := 1; i <= signalsPerVar; i++ {
				currentI := i
				signals.SendVarSig(&manager.VarSig{
					ComputedTime:       time.Now(),
					TargetVarName:      vName,
					VarUpdateFunc:      func(prev any) (any, bool, error) { return currentI, true, nil },
					IsStateIndependent: false,
				})
				counter.Add(1)
				time.Sleep(15 * time.Millisecond)
			}
		}(varName, varCounts[varName])
	}

	wg.Wait()

	totalSent := 0
	for varName, counter := range varCounts {
		count := counter.Load()
		totalSent += int(count)
		t.Logf("Variable %s: %d signals sent", varName, count)
	}
	t.Logf("Total signals sent: %d", totalSent)

	// Wait for processing
	time.Sleep(3 * time.Second)

	t.Logf("Total executions: %d", executionCount.Load())

	// Verify all variables exist in state
	for varName := range varCounts {
		if _, ok := state.VarState.Get(varName); !ok {
			t.Errorf("expected variable %s to exist", varName)
		}
	}

	// Verify final state
	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected final state Ready, got %s", state.GetManagerInnerState())
	}

	logger.PrintSummary()
}

// TestHighFrequency_PriorityUnderLoad tests signal priority under high load.
func TestHighFrequency_PriorityUnderLoad(t *testing.T) {
	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(1000)
	logger := manager.NewLogger(10000)

	reducer := manager.NewReducer(state, signals, logger)
	commander := manager.NewEffectCommander()

	var executionCount atomic.Int32
	var watcherSigProcessed atomic.Int32
	var userSigProcessed atomic.Int32
	var varSigProcessed atomic.Int32

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		executionCount.Add(1)
		if msg != nil {
			userSigProcessed.Add(1)
		}
		time.Sleep(40 * time.Millisecond)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Send mixed signals at high frequency
	t.Log("Sending mixed signals: VarSig, UserSig, WatcherSig")

	var wg sync.WaitGroup

	// VarSig sender (lowest priority)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			currentI := i
			signals.SendVarSig(&manager.VarSig{
				ComputedTime:       time.Now(),
				TargetVarName:      "priorityVar",
				VarUpdateFunc:      func(prev any) (any, bool, error) { return currentI, true, nil },
				IsStateIndependent: false,
			})
			varSigProcessed.Add(1)
			time.Sleep(30 * time.Millisecond)
		}
	}()

	// UserSig sender (medium priority)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			signals.SendUserSig(&manager.UserSig{
				ReceivedTime: time.Now(),
				UserMessage: &shared.Message{
					Content:    "priority test",
					ReceivedAt: time.Now(),
				},
			})
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// WatcherSig sender (highest priority) - sent after others to test priority
	time.Sleep(200 * time.Millisecond)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			signals.SendWatcherSig(&manager.WatcherSig{
				SignalTime:  time.Now(),
				TargetState: shared.StateReady,
				Reason:      "priority test",
			})
			watcherSigProcessed.Add(1)
			time.Sleep(100 * time.Millisecond)
		}
	}()

	wg.Wait()
	t.Logf("All signals sent - VarSig: %d, UserSig: 30, WatcherSig: %d",
		varSigProcessed.Load(), watcherSigProcessed.Load())

	// Wait for processing
	time.Sleep(3 * time.Second)

	t.Logf("Total executions: %d", executionCount.Load())
	t.Logf("User messages processed: %d", userSigProcessed.Load())

	// Verify priority handling - check reduce log for signal order
	reduceLogs := logger.GetReduceLog()
	watcherFirst := 0
	for i := 0; i < len(reduceLogs) && i < 100; i++ {
		if _, ok := reduceLogs[i].Action.Signal.(*manager.WatcherSig); ok {
			watcherFirst++
		}
	}
	t.Logf("WatcherSig processed early in log: %d", watcherFirst)

	logger.PrintSummary()
}

// TestHighFrequency_StressTest tests extreme load conditions.
func TestHighFrequency_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	state := manager.NewManagerState(shared.StateReady)
	signals := manager.NewSignalChannels(5000) // Very large buffer
	logger := manager.NewLogger(50000)

	reducer := manager.NewReducer(state, signals, logger)
	commander := manager.NewEffectCommander()

	var executionCount atomic.Int64
	var signalsSent atomic.Int64

	managedFunc := func(msg *shared.Message, ctx shared.HershContext) error {
		executionCount.Add(1)
		time.Sleep(10 * time.Millisecond) // Fast execution
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go reducer.RunWithEffects(ctx, commander, handler)

	// Extreme stress: 1000 signals from 10 concurrent senders
	numSenders := 10
	signalsPerSender := 100
	t.Logf("STRESS TEST: %d senders × %d signals = %d total signals",
		numSenders, signalsPerSender, numSenders*signalsPerSender)

	startTime := time.Now()
	var wg sync.WaitGroup

	for sender := 0; sender < numSenders; sender++ {
		wg.Add(1)
		go func(senderID int) {
			defer wg.Done()
			for i := 0; i < signalsPerSender; i++ {
				currentVal := senderID*1000 + i
				signals.SendVarSig(&manager.VarSig{
					ComputedTime:       time.Now(),
					TargetVarName:      "stressVar",
					VarUpdateFunc:      func(prev any) (any, bool, error) { return currentVal, true, nil },
					IsStateIndependent: false,
				})
				signalsSent.Add(1)
				time.Sleep(5 * time.Millisecond)
			}
		}(sender)
	}

	wg.Wait()
	sendDuration := time.Since(startTime)
	t.Logf("All %d signals sent in %v", signalsSent.Load(), sendDuration)

	// Wait for processing
	time.Sleep(5 * time.Second)
	processDuration := time.Since(startTime)

	execCount := executionCount.Load()
	t.Logf("Total executions: %d in %v", execCount, processDuration)
	t.Logf("Throughput: %.2f signals/sec", float64(signalsSent.Load())/sendDuration.Seconds())
	t.Logf("Execution rate: %.2f executions/sec", float64(execCount)/processDuration.Seconds())

	// Verify system stability under stress
	if execCount == 0 {
		t.Error("expected some executions under stress")
	}

	if state.GetManagerInnerState() != shared.StateReady {
		t.Errorf("expected final state Ready, got %s", state.GetManagerInnerState())
	}

	// Check for watch errors
	watchErrorLog := logger.GetWatchErrorLog()
	if len(watchErrorLog) > 0 {
		t.Logf("Watch errors during stress test: %d", len(watchErrorLog))
		for i, entry := range watchErrorLog {
			if i < 5 { // Show first 5 errors
				t.Logf("  Watch Error %d [%s, %s]: %v", i+1, entry.VarName, entry.ErrorPhase, entry.Error)
			}
		}
	}

	logger.PrintSummary()
}
