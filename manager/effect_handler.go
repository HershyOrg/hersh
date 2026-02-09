package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/HershyOrg/hershy/hersh/hctx"
	"github.com/HershyOrg/hershy/hersh/shared"
)

// ManagedFunc is the type of function that can be managed by the Watcher.
// It receives a message and HershContext, and returns an error for control flow.
type ManagedFunc func(message *shared.Message, ctx shared.HershContext) error

// Cleaner provides cleanup functionality for managed functions.
type Cleaner interface {
	ClearRun(ctx shared.HershContext) error
}

// EffectLogger logs effect execution results.
type EffectLogger interface {
	LogEffectResult(result *EffectResult)
	LogEffect(msg string)
	GetRecentResults(count int) []*EffectResult
}

// EffectHandler executes effects and manages the lifecycle of managed functions.
// This is now a synchronous component - effects are executed via direct function calls.
type EffectHandler struct {
	mu               sync.RWMutex
	managedFunc      ManagedFunc
	cleaner          Cleaner
	state            *ManagerState
	signals          *SignalChannels
	logger           EffectLogger
	config           shared.WatcherConfig
	expectedVars     []string // Variables registered by Watch calls
	rootCtx          context.Context
	rootCtxCancel    context.CancelFunc
	consecutiveFails int
	hershCtx         *hctx.HershContext // Persistent HershContext across executions
	cleanupDone      chan struct{}      // Signals when cleanup completes
}

// NewEffectHandler creates a new EffectHandler.
func NewEffectHandler(
	managedFunc ManagedFunc,
	cleaner Cleaner,
	state *ManagerState,
	signals *SignalChannels,
	logger EffectLogger,
	config shared.WatcherConfig,
) *EffectHandler {
	bgCtx, cancel := context.WithCancel(context.Background())

	// Create persistent HershContext
	hershCtx := hctx.New(bgCtx, "effect Handler ctx", logger.(hctx.Logger))

	return &EffectHandler{
		managedFunc:   managedFunc,
		cleaner:       cleaner,
		state:         state,
		signals:       signals,
		logger:        logger,
		config:        config,
		expectedVars:  make([]string, 0),
		rootCtx:       bgCtx,
		rootCtxCancel: cancel,
		hershCtx:      hershCtx,
		cleanupDone:   make(chan struct{}, 1), // Buffered to avoid blocking
	}
}

// RegisterVar registers a variable name that Watch will monitor.
func (eh *EffectHandler) RegisterVar(varName string) {
	eh.expectedVars = append(eh.expectedVars, varName)
}

// CheckInitializationComplete checks if all expected variables are initialized.
// Returns true if initialization is complete, false otherwise.
func (eh *EffectHandler) CheckInitializationComplete() bool {
	if len(eh.expectedVars) == 0 {
		return true
	}
	return eh.state.VarState.AllInitialized(eh.expectedVars)
}

// SetWatcher sets the Watcher reference in the HershContext.
// This must be called before running any effects.
// The watcher parameter should be of type *Watcher from hersh package.
func (eh *EffectHandler) SetWatcher(watcher any) {
	eh.hershCtx.SetWatcher(watcher)
}

// SetManagedFunc sets the managed function.
func (eh *EffectHandler) SetManagedFunc(managedFunc ManagedFunc) {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	eh.managedFunc = managedFunc
}

// SetCleaner sets the cleaner function.
func (eh *EffectHandler) SetCleaner(cleaner Cleaner) {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	eh.cleaner = cleaner
}

// HasManagedFunc returns whether a managed function has been set.
func (eh *EffectHandler) HasManagedFunc() bool {
	eh.mu.RLock()
	defer eh.mu.RUnlock()
	return eh.managedFunc != nil
}

// GetHershContext returns the persistent HershContext.
func (eh *EffectHandler) GetHershContext() shared.HershContext {
	return eh.hershCtx
}

// ExecuteEffect executes an effect and returns the resulting WatcherSig (if any).
// This is called synchronously by the Reducer.
// Returns nil if no further state transition is needed.
func (eh *EffectHandler) ExecuteEffect(effect EffectDefinition) *WatcherSig {
	return eh.executeEffect(effect)
}

// executeEffect executes the effect and returns the resulting WatcherSig.
// Returns nil if no state transition is needed.
func (eh *EffectHandler) executeEffect(effect EffectDefinition) *WatcherSig {
	var result *EffectResult
	var sig *WatcherSig

	switch e := effect.(type) {
	case *RunScriptEffect:
		result, sig = eh.runScript()
	case *InitRunScriptEffect:
		result, sig = eh.initRunScript()
	case *ClearRunScriptEffect:
		result, sig = eh.clearRunScript(e.HookState)
	case *JustKillEffect:
		result, sig = eh.justKill()
	case *JustCrashEffect:
		result, sig = eh.justCrash()
	case *RecoverEffect:
		result, sig = eh.recover()
	default:
		result = &EffectResult{
			Effect:    effect,
			Success:   false,
			Error:     fmt.Errorf("unknown effect type: %T", effect),
			Timestamp: time.Now(),
		}
		sig = nil
	}

	if eh.logger != nil {
		eh.logger.LogEffectResult(result)
	}

	return sig
}

// runScript executes the managed function.
// Returns (result, sig) where sig is the state transition signal.
func (eh *EffectHandler) runScript() (*EffectResult, *WatcherSig) {
	result := &EffectResult{
		Effect:    &RunScriptEffect{},
		Timestamp: time.Now(),
	}

	// Create execution context with timeout from rootCtx
	// This ensures timeout propagates through all child contexts
	execCtx, cancel := context.WithTimeout(eh.rootCtx, eh.config.DefaultTimeout)
	defer cancel()

	// Consume message
	msg := eh.state.UserState.ConsumeMessage()

	// Update persistent HershContext with new context and message
	// All Watch calls will use this context and respect the timeout
	eh.hershCtx.UpdateContext(execCtx)
	eh.hershCtx.SetMessage(msg)

	// Get managedFunc with read lock
	eh.mu.RLock()
	fn := eh.managedFunc
	eh.mu.RUnlock()

	// Execute in goroutine with panic recovery
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic: %v", r)
			}
		}()
		done <- fn(msg, eh.hershCtx)
	}()

	// Wait for completion or timeout
	// Priority: timeout signal takes precedence over goroutine completion
	var sig *WatcherSig
	select {
	case <-execCtx.Done():
		// Timeout occurred - this is checked first for immediate response
		result.Success = false
		result.Error = execCtx.Err()
		sig = &WatcherSig{
			SignalTime:  time.Now(),
			TargetState: shared.StateReady,
			Reason:      "execution timeout",
		}
		// Note: goroutine may still be running, but context cancellation
		// will be propagated to all child contexts (WatchCall, etc.)
	case err := <-done:
		// Execution completed before timeout
		if err != nil {
			result.Success = false
			result.Error = err
			sig = eh.handleScriptError(err)
		} else {
			result.Success = true
			sig = &WatcherSig{
				SignalTime:  time.Now(),
				TargetState: shared.StateReady,
				Reason:      "execution completed successfully",
			}
		}
	}

	return result, sig
}

// handleScriptError processes errors from managed function execution.
// Returns the appropriate WatcherSig based on error type and recovery policy.
// Part 2: Determines state transition AFTER execution (delay was already applied in runScript)
func (eh *EffectHandler) handleScriptError(err error) *WatcherSig {
	switch err.(type) {
	case *shared.KillError:
		return &WatcherSig{
			SignalTime:  time.Now(),
			TargetState: shared.StateKilled,
			Reason:      err.Error(),
		}
	case *shared.StopError:
		return &WatcherSig{
			SignalTime:  time.Now(),
			TargetState: shared.StateStopped,
			Reason:      err.Error(),
		}
	default:
		// Count consecutive failures from recent logs
		consecutiveFails := eh.countConsecutiveFailures()

		if consecutiveFails < eh.config.RecoveryPolicy.MinConsecutiveFailures {
			// Apply lightweight retry delay
			delay := eh.calculateLightweightRetryDelay(consecutiveFails)
			if delay > 0 {
				time.Sleep(delay)
			}

			return &WatcherSig{
				SignalTime:  time.Now(),
				TargetState: shared.StateReady,
				Reason: fmt.Sprintf("error suppressed (%d/%d) after %v delay: %v",
					consecutiveFails, eh.config.RecoveryPolicy.MinConsecutiveFailures, delay, err),
			}
		}

		// Too many consecutive failures - enter recovery mode
		return &WatcherSig{
			SignalTime:  time.Now(),
			TargetState: shared.StateWaitRecover,
			Reason: fmt.Sprintf("consecutive failures (%d) >= threshold (%d): %v",
				consecutiveFails, eh.config.RecoveryPolicy.MinConsecutiveFailures, err),
		}
	}
}

// initRunScript performs initialization run.
// Returns (result, sig).
func (eh *EffectHandler) initRunScript() (*EffectResult, *WatcherSig) {
	result := &EffectResult{
		Effect:    &InitRunScriptEffect{},
		Timestamp: time.Now(),
	}

	// Phase 1: Run once to trigger Watch registrations
	phase1Result := eh.runScriptOnce()
	if phase1Result.Error != nil {
		// Check if it's VarNotInitializedError (expected during init)
		if _, ok := phase1Result.Error.(*shared.VarNotInitializedError); !ok {
			// Unexpected error
			result.Success = false
			result.Error = phase1Result.Error
			sig := eh.handleScriptError(phase1Result.Error)
			return result, sig
		}
	}

	// Phase 2: Check if all variables are already initialized
	// If no variables to initialize, transition to Ready immediately
	if len(eh.expectedVars) == 0 {
		result.Success = true
		sig := &WatcherSig{
			SignalTime:  time.Now(),
			TargetState: shared.StateReady,
			Reason:      "initialization complete (no variables to watch)",
		}
		return result, sig
	}

	// Check if all variables are already initialized (from the first run)
	if eh.state.VarState.AllInitialized(eh.expectedVars) {
		result.Success = true
		sig := &WatcherSig{
			SignalTime:  time.Now(),
			TargetState: shared.StateReady,
			Reason:      "initialization complete",
		}
		return result, sig
	}

	// Not all variables initialized yet
	// Stay in InitRun state and let reducer process VarSig signals
	// Reducer will check initialization status on each VarSig
	result.Success = true
	return result, nil // nil sig means stay in current state
}

// runScriptOnce executes the managed function once (for initialization).
func (eh *EffectHandler) runScriptOnce() *EffectResult {
	result := &EffectResult{
		Timestamp: time.Now(),
	}

	execCtx, cancel := context.WithTimeout(eh.rootCtx, eh.config.DefaultTimeout)
	defer cancel()

	// Update persistent HershContext
	eh.hershCtx.UpdateContext(execCtx)
	eh.hershCtx.SetMessage(nil)

	// Get managedFunc with read lock
	eh.mu.RLock()
	fn := eh.managedFunc
	eh.mu.RUnlock()

	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic: %v", r)
			}
		}()
		done <- fn(nil, eh.hershCtx)
	}()

	select {
	case err := <-done:
		result.Error = err
		result.Success = err == nil
	case <-execCtx.Done():
		result.Error = execCtx.Err()
		result.Success = false
	}

	return result
}

// clearRunScript executes cleanup.
// Returns (result, sig).
func (eh *EffectHandler) clearRunScript(hookState shared.ManagerInnerState) (*EffectResult, *WatcherSig) {
	result := &EffectResult{
		Effect:    &ClearRunScriptEffect{HookState: hookState},
		Timestamp: time.Now(),
	}

	// Cancel root context
	eh.rootCtxCancel()

	// Create new root context
	eh.rootCtx, eh.rootCtxCancel = context.WithCancel(context.Background())

	// Execute cleanup using persistent HershContext
	if eh.cleaner != nil {
		// Update context with 5-minute timeout for cleanup
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		eh.hershCtx.UpdateContext(cleanCtx)

		err := eh.cleaner.ClearRun(eh.hershCtx)
		if err != nil {
			result.Success = false
			result.Error = err
		} else {
			result.Success = true
		}
	} else {
		result.Success = true
	}

	// Signal cleanup completion
	// This allows Stop() to wait for actual cleanup completion, not just state transition
	select {
	case eh.cleanupDone <- struct{}{}:
	default:
		// Channel already has a signal, skip
	}

	// Return signal to transition to hook state
	sig := &WatcherSig{
		SignalTime:  time.Now(),
		TargetState: hookState,
		Reason:      fmt.Sprintf("cleanup completed for %s", hookState),
	}

	return result, sig
}

// GetCleanupDone returns the cleanup completion channel.
// This allows Watcher.Stop() to wait for cleanup to actually complete.
func (eh *EffectHandler) GetCleanupDone() <-chan struct{} {
	return eh.cleanupDone
}

// justKill returns Kill signal without cleanup.
// Returns (result, sig).
func (eh *EffectHandler) justKill() (*EffectResult, *WatcherSig) {
	sig := &WatcherSig{
		SignalTime:  time.Now(),
		TargetState: shared.StateKilled,
		Reason:      "kill requested",
	}
	result := &EffectResult{
		Effect:    &JustKillEffect{},
		Success:   true,
		Timestamp: time.Now(),
	}
	return result, sig
}

// justCrash returns Crash signal without cleanup.
// Returns (result, sig).
func (eh *EffectHandler) justCrash() (*EffectResult, *WatcherSig) {
	sig := &WatcherSig{
		SignalTime:  time.Now(),
		TargetState: shared.StateCrashed,
		Reason:      "crash requested",
	}
	result := &EffectResult{
		Effect:    &JustCrashEffect{},
		Success:   true,
		Timestamp: time.Now(),
	}
	return result, sig
}

// recover implements the recovery logic (Erlang Supervisor pattern).
// Returns (result, sig).
func (eh *EffectHandler) recover() (*EffectResult, *WatcherSig) {
	result := &EffectResult{
		Effect:    &RecoverEffect{},
		Timestamp: time.Now(),
	}

	// Count consecutive failures from logs
	consecutiveFails := eh.countConsecutiveFailures()

	if consecutiveFails >= eh.config.RecoveryPolicy.MaxConsecutiveFailures {
		// Too many failures - crash
		result.Success = false
		result.Error = fmt.Errorf("max consecutive failures reached: %d", consecutiveFails)
		sig := &WatcherSig{
			SignalTime:  time.Now(),
			TargetState: shared.StateCrashed,
			Reason:      "max consecutive failures exceeded",
		}
		return result, sig
	}

	// Calculate recovery backoff delay
	delay := eh.calculateRecoveryBackoff(consecutiveFails)
	time.Sleep(delay)

	// Attempt recovery - return InitRun signal
	result.Success = true
	sig := &WatcherSig{
		SignalTime:  time.Now(),
		TargetState: shared.StateInitRun,
		Reason:      fmt.Sprintf("recovery attempt after %d failures (backoff: %v)", consecutiveFails, delay),
	}

	return result, sig
}

// countConsecutiveFailures counts recent consecutive failures from logs.
// Used by handleScriptError to include the current failure (+1).
func (eh *EffectHandler) countConsecutiveFailures() int {
	// Get recent results from log
	recentResults := eh.logger.GetRecentResults(eh.config.RecoveryPolicy.MaxConsecutiveFailures + 1)

	consecutiveFails := 0
	for i := len(recentResults) - 1; i >= 0; i-- {
		if !recentResults[i].Success {
			consecutiveFails++
		} else {
			break
		}
	}

	// +1 for current failure (not yet logged)
	return consecutiveFails + 1
}

// calculateRecoveryBackoff calculates exponential backoff for recovery attempts.
func (eh *EffectHandler) calculateRecoveryBackoff(failures int) time.Duration {
	delay := eh.config.RecoveryPolicy.BaseRetryDelay

	// Recovery 진입 이후의 실패 횟수만 계산
	recoveryAttempts := failures - eh.config.RecoveryPolicy.MinConsecutiveFailures
	if recoveryAttempts < 0 {
		recoveryAttempts = 0
	}

	for i := 0; i < recoveryAttempts; i++ {
		delay *= 2
		if delay > eh.config.RecoveryPolicy.MaxRetryDelay {
			return eh.config.RecoveryPolicy.MaxRetryDelay
		}
	}
	return delay
}

// calculateLightweightRetryDelay calculates delay for failures below MinConsecutiveFailures.
// Returns 0 if no delay configured.
func (eh *EffectHandler) calculateLightweightRetryDelay(failures int) time.Duration {
	delays := eh.config.RecoveryPolicy.LightweightRetryDelays
	if len(delays) == 0 {
		return 0 // No lightweight retry configured (legacy behavior)
	}

	// failures는 1부터 시작 (첫 실패 = 1)
	index := failures - 1
	if index < 0 {
		return 0
	}
	if index >= len(delays) {
		// 마지막 delay 재사용
		index = len(delays) - 1
	}

	return delays[index]
}

// calculateBackoff calculates exponential backoff delay (deprecated - use calculateRecoveryBackoff).
func (eh *EffectHandler) calculateBackoff(failures int) time.Duration {
	delay := eh.config.RecoveryPolicy.BaseRetryDelay
	for range failures {
		delay *= 2
		if delay > eh.config.RecoveryPolicy.MaxRetryDelay {
			return eh.config.RecoveryPolicy.MaxRetryDelay
		}
	}
	return delay
}
