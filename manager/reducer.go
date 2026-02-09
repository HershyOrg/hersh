package manager

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/HershyOrg/hershy/hersh/shared"
)

// ReduceAction represents a state transition that occurred.
type ReduceAction struct {
	PrevState StateSnapshot
	Signal    shared.Signal
	NextState StateSnapshot
}

// Reducer manages state transitions based on signals.
// It implements priority-based signal processing.
type Reducer struct {
	state   *ManagerState
	signals *SignalChannels
	logger  ReduceLogger
}

// ReduceLogger handles logging of state transitions.
type ReduceLogger interface {
	LogReduce(action ReduceAction)
	LogWatchError(varName string, phase WatchErrorPhase, err error)
	LogStateTransitionFault(from, to shared.ManagerInnerState, reason string, err error)
}

// NewReducer creates a new Reducer.
func NewReducer(state *ManagerState, signals *SignalChannels, logger ReduceLogger) *Reducer {
	return &Reducer{
		state:   state,
		signals: signals,
		logger:  logger,
	}
}

// RunWithEffects starts the reducer loop with synchronous effect execution.
// This is the main loop following the specification:
// 1. Wait for signal
// 2. Reduce (state transition)
// 3. Call EffectCommander synchronously
// 4. Call EffectHandler synchronously
// 5. If effect returns WatcherSig, process it recursively
// Priority: WatcherSig > UserSig > VarSig
func (r *Reducer) RunWithEffects(ctx context.Context, commander *EffectCommander, handler *EffectHandler) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			// Panic recovery
			err = fmt.Errorf("reducer panic: %v", rec)

			// Transition to Crashed state
			r.state.SetManagerInnerState(shared.StateCrashed)

			// Log panic with stack trace
			fmt.Printf("[Reducer] PANIC RECOVERED: %v\n", rec)
			fmt.Printf("[Reducer] Stack trace:\n")

			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			fmt.Printf("%s\n", buf[:n])
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.signals.NewSigAppended:
			// Process all available signals respecting priority
			r.processAvailableSignalsWithEffects(ctx, commander, handler)
		}
	}
}

// processAvailableSignalsWithEffects drains signal channels respecting priority.
// For each signal: Reduce → CommandEffect → ExecuteEffect → handle result WatcherSig.
func (r *Reducer) processAvailableSignalsWithEffects(ctx context.Context, commander *EffectCommander, handler *EffectHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Try to process one signal, highest priority first
			if !r.tryProcessNextSignalWithEffects(commander, handler) {
				// No more signals to process
				return
			}
		}
	}
}

// tryProcessNextSignalWithEffects processes one signal following the specification:
// 1. Select signal by priority
// 2. Reduce (state transition)
// 3. CommandEffect (synchronous)
// 4. ExecuteEffect (synchronous)
// 5. If effect returns WatcherSig, process it recursively (atomic execution)
// Returns true if a signal was processed, false if no signals available.
func (r *Reducer) tryProcessNextSignalWithEffects(commander *EffectCommander, handler *EffectHandler) bool {
	currentState := r.state.GetManagerInnerState()

	// Priority 1: WatcherSig (highest)
	select {
	case sig := <-r.signals.WatcherSigChan:
		r.reduceAndExecuteEffect(sig, commander, handler)
		return true
	default:
	}

	// Priority 2: UserSig
	if r.canProcessUserSig(currentState) {
		select {
		case sig := <-r.signals.UserSigChan:
			r.reduceAndExecuteEffect(sig, commander, handler)
			return true
		default:
		}
	}

	// Priority 3: VarSig (lowest)
	if r.canProcessVarSig(currentState) {
		select {
		case sig := <-r.signals.VarSigChan:
			r.reduceAndExecuteEffect(sig, commander, handler)
			return true
		default:
		}
	}

	return false
}

// reduceAndExecuteEffect performs the complete Reduce-Effect cycle:
// Reduce → CommandEffect → ExecuteEffect → handle result.
func (r *Reducer) reduceAndExecuteEffect(sig shared.Signal, commander *EffectCommander, handler *EffectHandler) {
	// 1. Reduce: perform state transition
	prevSnapshot := r.state.Snapshot()

	switch s := sig.(type) {
	case *WatcherSig:
		r.reduceWatcherSig(s)
	case *UserSig:
		r.reduceUserSig(s)
	case *VarSig:
		r.reduceVarSig(s)
		// Note: InitRun completion check moved after effect execution for atomic processing
	default:
		return // Unknown signal type
	}

	// 2. Create action
	action := ReduceAction{
		PrevState: prevSnapshot,
		Signal:    sig,
		NextState: r.state.Snapshot(),
	}

	// Log the reduce action
	if r.logger != nil {
		r.logger.LogReduce(action)
	}

	// 3. Check for InitRun completion before CommandEffect
	// This ensures atomic InitRun → Ready transition without effect execution
	if r.state.GetManagerInnerState() == shared.StateInitRun {
		if _, ok := sig.(*VarSig); ok && handler.CheckInitializationComplete() {
			// All variables initialized - transition to Ready immediately
			initCompleteSig := &WatcherSig{
				SignalTime:  time.Now(),
				TargetState: shared.StateReady,
				Reason:      "initialization complete (all variables initialized)",
			}
			// Process Ready transition recursively (atomic)
			r.reduceAndExecuteEffect(initCompleteSig, commander, handler)
			return
		}
	}

	// 4. CommandEffect (synchronous)
	effectDef := commander.CommandEffect(action)
	if effectDef == nil {
		return // No effect to execute
	}

	// 5. ExecuteEffect (synchronous)
	resultSig := handler.ExecuteEffect(effectDef)
	if resultSig == nil {
		return // No further state transition needed
	}

	// 6. Process result WatcherSig recursively (atomic execution)
	r.reduceAndExecuteEffect(resultSig, commander, handler)
}

// tryProcessNextSignal attempts to process one signal following priority rules.
// Returns true if a signal was processed or skipped, false if no signals available.
// Signals are NEVER dropped - they are either processed or left in channel.
func (r *Reducer) tryProcessNextSignal() bool {
	currentState := r.state.GetManagerInnerState()

	// Priority 1: WatcherSig (highest)
	select {
	case sig := <-r.signals.WatcherSigChan:
		r.reduceWatcherSig(sig)
		return true
	default:
	}

	// Priority 2: UserSig
	// Check if current state can process UserSig
	if r.canProcessUserSig(currentState) {
		select {
		case sig := <-r.signals.UserSigChan:
			r.reduceUserSig(sig)
			return true
		default:
		}
	}
	// If cannot process, leave signal in channel (don't consume it)

	// Priority 3: VarSig (lowest)
	// Check if current state can process VarSig
	if r.canProcessVarSig(currentState) {
		select {
		case sig := <-r.signals.VarSigChan:
			r.reduceVarSig(sig)
			return true
		default:
		}
	}
	// If cannot process, leave signal in channel (don't consume it)

	// No signals available
	return false
}

// canProcessUserSig checks if current state can process UserSig.
func (r *Reducer) canProcessUserSig(state shared.ManagerInnerState) bool {
	return state == shared.StateReady
}

// canProcessVarSig checks if current state can process VarSig.
func (r *Reducer) canProcessVarSig(state shared.ManagerInnerState) bool {
	return state == shared.StateReady || state == shared.StateInitRun
}

// reduceVarSig handles VarSig according to transition rules.
// Only called when canProcessVarSig returns true.
// Logging is handled by reduceAndExecuteEffect.
func (r *Reducer) reduceVarSig(sig *VarSig) {
	currentState := r.state.GetManagerInnerState()

	switch currentState {
	case shared.StateReady:
		// Batch collect and apply all VarSigs
		updates := r.collectAndApplyVarSigs(sig)

		// Only transition to Running if there are actual updates
		if len(updates) > 0 {
			r.state.VarState.BatchSet(updates)
			r.state.SetManagerInnerState(shared.StateRunning)
		}
		// If no updates (all changed=false), stay in Ready state

	case shared.StateInitRun:
		// During initialization, collect and apply VarSigs to update state
		// This allows InitRun phase 2 to detect when all variables are initialized
		updates := r.collectAndApplyVarSigs(sig)

		// Only update VarState if there are actual updates
		if len(updates) > 0 {
			r.state.VarState.BatchSet(updates)
		}
		// Don't change ManagerInnerState, just update VarState

	default:
		// Should never reach here due to canProcessVarSig check
		panic(fmt.Sprintf("reduceVarSig called in invalid state: %s", currentState))
	}
}

// collectAndApplyVarSigs collects all VarSigs and applies them correctly.
// For IsStateIndependent=true (Flow): only apply the last signal's function
// For IsStateIndependent=false (Tick): apply all functions sequentially
func (r *Reducer) collectAndApplyVarSigs(first *VarSig) map[string]any {
	sigs := []*VarSig{first}

	// Collect all available VarSigs from the channel
	for {
		select {
		case sig := <-r.signals.VarSigChan:
			sigs = append(sigs, sig)
		default:
			// break사용이 불가하므로 goto이용.
			goto APPLY
		}
	}

APPLY:
	// Group signals by variable name
	byVar := make(map[string][]*VarSig)
	for _, sig := range sigs {
		byVar[sig.TargetVarName] = append(byVar[sig.TargetVarName], sig)
	}

	updates := make(map[string]any)

	for varName, varSigs := range byVar {
		// Check if this variable is state-independent (check first signal)
		isIndependent := varSigs[0].IsStateIndependent

		if isIndependent {
			// State-independent (Flow): only apply the last signal
			lastSig := varSigs[len(varSigs)-1]

			// Get current value from VarState (may be ignored by the function)
			currentValue, _ := r.state.VarState.Get(varName)

			nextValue, changed, err := lastSig.VarUpdateFunc(currentValue)
			if err != nil {
				// Skip this signal on error
				if r.logger != nil {
					r.logger.LogWatchError(varName, ErrorPhaseExecuteComputeFunc, err)
				}
				continue
			}

			if changed {
				updates[varName] = nextValue
			}

		} else {
			// State-dependent (Tick): apply all signals sequentially
			currentValue, exists := r.state.VarState.Get(varName)
			if !exists {
				currentValue = nil
			}

			hasAnyChange := false
			for _, sig := range varSigs {
				nextValue, changed, err := sig.VarUpdateFunc(currentValue)
				if err != nil {
					// Skip this signal on error
					if r.logger != nil {
						r.logger.LogWatchError(varName, ErrorPhaseExecuteComputeFunc, err)
					}
					continue
				}

				if changed {
					currentValue = nextValue // Next function's input
					hasAnyChange = true
				}
			}

			// Only add to updates if at least one signal reported a change
			if hasAnyChange {
				updates[varName] = currentValue
			}
		}
	}

	return updates
}

// reduceUserSig handles UserSig according to transition rules.
// Only called when canProcessUserSig returns true.
// Logging is handled by reduceAndExecuteEffect.
func (r *Reducer) reduceUserSig(sig *UserSig) {
	currentState := r.state.GetManagerInnerState()

	switch currentState {
	case shared.StateReady:
		r.state.UserState.SetMessage(sig.UserMessage)
		r.state.SetManagerInnerState(shared.StateRunning)

	default:
		// Should never reach here due to canProcessUserSig check
		panic(fmt.Sprintf("reduceUserSig called in invalid state: %s", currentState))
	}
}

// reduceWatcherSig handles WatcherSig according to transition rules.
// Logging is handled by reduceAndExecuteEffect.
func (r *Reducer) reduceWatcherSig(sig *WatcherSig) {
	currentState := r.state.GetManagerInnerState()
	targetState := sig.TargetState

	// Ignore same-state transitions
	if currentState == targetState {
		return
	}

	// Validate transition
	if err := r.validateTransition(currentState, targetState); err != nil {
		// Log error but don't crash reducer
		if r.logger != nil {
			r.logger.LogStateTransitionFault(currentState, targetState, sig.Reason, err)
		}
		return
	}

	// Special case: Crashed is terminal
	if currentState == shared.StateCrashed {
		if r.logger != nil {
			r.logger.LogStateTransitionFault(
				currentState,
				targetState,
				sig.Reason,
				fmt.Errorf("cannot transition from Crashed state"),
			)
		}
		return
	}

	// Special case: InitRun clears VarState
	if targetState == shared.StateInitRun {
		r.state.VarState.Clear()
	}

	// Perform transition
	r.state.SetManagerInnerState(targetState)
}

// validateTransition checks if a state transition is valid.
func (r *Reducer) validateTransition(from, to shared.ManagerInnerState) error {
	// Crashed is terminal
	if from == shared.StateCrashed {
		return fmt.Errorf("cannot transition from Crashed state")
	}

	// Some basic validation - full FSM rules would go here
	switch from {
	case shared.StateStopped:
		// From Stopped, only InitRun, Killed, Crashed, WaitRecover allowed
		if to != shared.StateInitRun && to != shared.StateKilled && to != shared.StateCrashed && to != shared.StateWaitRecover {
			return fmt.Errorf("invalid transition from Stopped to %s", to)
		}
	case shared.StateKilled:
		// From Killed, only Crashed or WaitRecover allowed
		if to != shared.StateCrashed && to != shared.StateWaitRecover {
			return fmt.Errorf("invalid transition from Killed to %s", to)
		}
	}

	return nil
}
