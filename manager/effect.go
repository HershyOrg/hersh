package manager

import (
	"fmt"
	"time"

	"github.com/HershyOrg/hershy/hersh/shared"
)

// EffectDefinition defines the type of effect to execute.
type EffectDefinition interface {
	Type() EffectType
	String() string
}

// EffectType categorizes different effect types.
type EffectType uint8

const (
	EffectRunScript EffectType = iota
	EffectClearRunScript
	EffectInitRunScript
	EffectJustKill
	EffectJustCrash
	EffectRecover
)

func (et EffectType) String() string {
	switch et {
	case EffectRunScript:
		return "RunScript"
	case EffectClearRunScript:
		return "ClearRunScript"
	case EffectInitRunScript:
		return "InitRunScript"
	case EffectJustKill:
		return "JustKill"
	case EffectJustCrash:
		return "JustCrash"
	case EffectRecover:
		return "Recover"
	default:
		return "Unknown"
	}
}

// RunScriptEffect executes the managed function.
type RunScriptEffect struct{}

func (e *RunScriptEffect) Type() EffectType { return EffectRunScript }
func (e *RunScriptEffect) String() string   { return "RunScript" }

// ClearRunScriptEffect executes cleanup with hook information.
type ClearRunScriptEffect struct {
	HookState shared.ManagerInnerState // Which state triggered this cleanup
}

func (e *ClearRunScriptEffect) Type() EffectType { return EffectClearRunScript }
func (e *ClearRunScriptEffect) String() string {
	return fmt.Sprintf("ClearRunScript{hook=%s}", e.HookState)
}

// InitRunScriptEffect initializes and runs the managed function.
type InitRunScriptEffect struct{}

func (e *InitRunScriptEffect) Type() EffectType { return EffectInitRunScript }
func (e *InitRunScriptEffect) String() string   { return "InitRunScript" }

// JustKillEffect transitions to Killed without cleanup.
type JustKillEffect struct{}

func (e *JustKillEffect) Type() EffectType { return EffectJustKill }
func (e *JustKillEffect) String() string   { return "JustKill" }

// JustCrashEffect transitions to Crashed without cleanup.
type JustCrashEffect struct{}

func (e *JustCrashEffect) Type() EffectType { return EffectJustCrash }
func (e *JustCrashEffect) String() string   { return "JustCrash" }

// RecoverEffect attempts recovery or crashes.
type RecoverEffect struct{}

func (e *RecoverEffect) Type() EffectType { return EffectRecover }
func (e *RecoverEffect) String() string   { return "Recover" }

// EffectCommander determines which effect to execute based on state transitions.
// This is now a synchronous component - no goroutines, just function calls.
type EffectCommander struct{}

// NewEffectCommander creates a new EffectCommander.
func NewEffectCommander() *EffectCommander {
	return &EffectCommander{}
}

// CommandEffect determines which effect to trigger based on state transition.
// Returns nil if no effect should be executed.
// This is called synchronously by the Reducer.
func (ec *EffectCommander) CommandEffect(action ReduceAction) EffectDefinition {
	prevState := action.PrevState.ManagerInnerState
	nextState := action.NextState.ManagerInnerState

	// Ignore same-state transitions
	if prevState == nextState {
		return nil
	}

	return ec.determineEffect(prevState, nextState)
}

// determineEffect implements the trigger rules from the design.
func (ec *EffectCommander) determineEffect(prevState, nextState shared.ManagerInnerState) EffectDefinition {
	switch prevState {
	case shared.StateRunning:
		return ec.fromRunning(nextState)
	case shared.StateReady:
		return ec.fromReady(nextState)
	case shared.StateInitRun:
		return ec.fromInitRun(nextState)
	case shared.StateStopped:
		return ec.fromStopped(nextState)
	case shared.StateKilled:
		return ec.fromKilled(nextState)
	case shared.StateCrashed:
		return nil // Terminal state, ignore
	case shared.StateWaitRecover:
		return ec.fromWaitRecover(nextState)
	}
	return nil
}

func (ec *EffectCommander) fromRunning(nextState shared.ManagerInnerState) EffectDefinition {
	switch nextState {
	case shared.StateRunning:
		return nil // Ignore
	case shared.StateReady:
		return nil // Ignore (normal completion handled by EffectHandler)
	case shared.StateInitRun:
		return &InitRunScriptEffect{}
	case shared.StateStopped, shared.StateKilled, shared.StateCrashed:
		return &ClearRunScriptEffect{HookState: nextState}
	case shared.StateWaitRecover:
		return &RecoverEffect{}
	}
	return nil
}

func (ec *EffectCommander) fromReady(nextState shared.ManagerInnerState) EffectDefinition {
	switch nextState {
	case shared.StateReady:
		return nil
	case shared.StateRunning:
		return &RunScriptEffect{}
	case shared.StateInitRun:
		return &InitRunScriptEffect{}
	case shared.StateKilled, shared.StateStopped, shared.StateCrashed:
		return &ClearRunScriptEffect{HookState: nextState}
	case shared.StateWaitRecover:
		return &RecoverEffect{}
	}
	return nil
}

func (ec *EffectCommander) fromInitRun(nextState shared.ManagerInnerState) EffectDefinition {
	switch nextState {
	case shared.StateInitRun:
		return nil
	case shared.StateRunning:
		return &RunScriptEffect{}
	case shared.StateReady:
		return nil // Normal completion
	case shared.StateKilled, shared.StateStopped, shared.StateCrashed:
		return &ClearRunScriptEffect{HookState: nextState}
	case shared.StateWaitRecover:
		return &RecoverEffect{}
	}
	return nil
}

func (ec *EffectCommander) fromStopped(nextState shared.ManagerInnerState) EffectDefinition {
	switch nextState {
	case shared.StateStopped:
		return nil
	case shared.StateInitRun:
		return &InitRunScriptEffect{}
	case shared.StateKilled:
		return &JustKillEffect{}
	case shared.StateCrashed:
		return &JustCrashEffect{}
	case shared.StateReady, shared.StateRunning:
		return nil // Invalid, ignore
	case shared.StateWaitRecover:
		return &RecoverEffect{}
	}
	return nil
}

func (ec *EffectCommander) fromKilled(nextState shared.ManagerInnerState) EffectDefinition {
	switch nextState {
	case shared.StateKilled:
		return nil
	case shared.StateCrashed:
		return &JustCrashEffect{}
	case shared.StateWaitRecover:
		return &RecoverEffect{}
	default:
		return nil // All other transitions ignored
	}
}

func (ec *EffectCommander) fromWaitRecover(nextState shared.ManagerInnerState) EffectDefinition {
	switch nextState {
	case shared.StateWaitRecover:
		return &RecoverEffect{}
	case shared.StateCrashed, shared.StateKilled, shared.StateStopped:
		return &ClearRunScriptEffect{HookState: shared.StateCrashed}
	case shared.StateInitRun:
		return &InitRunScriptEffect{}
	default:
		return &RecoverEffect{}
	}
}

// EffectResult represents the result of executing an effect.
type EffectResult struct {
	Effect    EffectDefinition
	Success   bool
	Error     error
	Timestamp time.Time
}

func (er *EffectResult) String() string {
	status := "Ok"
	if !er.Success {
		status = fmt.Sprintf("Err(%v)", er.Error)
	}
	return fmt.Sprintf("EffectResult{effect=%s, status=%s, time=%s}",
		er.Effect, status, er.Timestamp.Format(time.RFC3339))
}
