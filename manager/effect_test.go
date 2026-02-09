package manager

import (
	"testing"

	"github.com/HershyOrg/hershy/hersh/shared"
)

// TestEffectCommander_RunScriptEffect tests Ready -> Running transition.
func TestEffectCommander_RunScriptEffect(t *testing.T) {
	commander := NewEffectCommander()

	action := ReduceAction{
		PrevState: StateSnapshot{ManagerInnerState: shared.StateReady},
		NextState: StateSnapshot{ManagerInnerState: shared.StateRunning},
	}

	effect := commander.CommandEffect(action)
	if effect == nil {
		t.Fatal("expected effect, got nil")
	}

	if _, ok := effect.(*RunScriptEffect); !ok {
		t.Errorf("expected RunScriptEffect, got %T", effect)
	}
}

// TestEffectCommander_InitRunScriptEffect tests Ready -> InitRun transition.
func TestEffectCommander_InitRunScriptEffect(t *testing.T) {
	commander := NewEffectCommander()

	action := ReduceAction{
		PrevState: StateSnapshot{ManagerInnerState: shared.StateReady},
		NextState: StateSnapshot{ManagerInnerState: shared.StateInitRun},
	}

	effect := commander.CommandEffect(action)
	if effect == nil {
		t.Fatal("expected effect, got nil")
	}

	if _, ok := effect.(*InitRunScriptEffect); !ok {
		t.Errorf("expected InitRunScriptEffect, got %T", effect)
	}
}

// TestEffectCommander_ClearRunScriptEffect tests Running -> Stopped transition.
func TestEffectCommander_ClearRunScriptEffect(t *testing.T) {
	commander := NewEffectCommander()

	action := ReduceAction{
		PrevState: StateSnapshot{ManagerInnerState: shared.StateRunning},
		NextState: StateSnapshot{ManagerInnerState: shared.StateStopped},
	}

	effect := commander.CommandEffect(action)
	if effect == nil {
		t.Fatal("expected effect, got nil")
	}

	clearEffect, ok := effect.(*ClearRunScriptEffect)
	if !ok {
		t.Fatalf("expected ClearRunScriptEffect, got %T", effect)
	}
	if clearEffect.HookState != shared.StateStopped {
		t.Errorf("expected hook state Stopped, got %s", clearEffect.HookState)
	}
}

// TestEffectCommander_JustKillEffect tests Stopped -> Killed transition.
func TestEffectCommander_JustKillEffect(t *testing.T) {
	commander := NewEffectCommander()

	action := ReduceAction{
		PrevState: StateSnapshot{ManagerInnerState: shared.StateStopped},
		NextState: StateSnapshot{ManagerInnerState: shared.StateKilled},
	}

	effect := commander.CommandEffect(action)
	if effect == nil {
		t.Fatal("expected effect, got nil")
	}

	if _, ok := effect.(*JustKillEffect); !ok {
		t.Errorf("expected JustKillEffect, got %T", effect)
	}
}

// TestEffectCommander_JustCrashEffect tests Killed -> Crashed transition.
func TestEffectCommander_JustCrashEffect(t *testing.T) {
	commander := NewEffectCommander()

	action := ReduceAction{
		PrevState: StateSnapshot{ManagerInnerState: shared.StateKilled},
		NextState: StateSnapshot{ManagerInnerState: shared.StateCrashed},
	}

	effect := commander.CommandEffect(action)
	if effect == nil {
		t.Fatal("expected effect, got nil")
	}

	if _, ok := effect.(*JustCrashEffect); !ok {
		t.Errorf("expected JustCrashEffect, got %T", effect)
	}
}

// TestEffectCommander_RecoverEffect tests Running -> WaitRecover transition.
func TestEffectCommander_RecoverEffect(t *testing.T) {
	commander := NewEffectCommander()

	action := ReduceAction{
		PrevState: StateSnapshot{ManagerInnerState: shared.StateRunning},
		NextState: StateSnapshot{ManagerInnerState: shared.StateWaitRecover},
	}

	effect := commander.CommandEffect(action)
	if effect == nil {
		t.Fatal("expected effect, got nil")
	}

	if _, ok := effect.(*RecoverEffect); !ok {
		t.Errorf("expected RecoverEffect, got %T", effect)
	}
}

// TestEffectCommander_IgnoreSameState tests same-state transition (should return nil).
func TestEffectCommander_IgnoreSameState(t *testing.T) {
	commander := NewEffectCommander()

	action := ReduceAction{
		PrevState: StateSnapshot{ManagerInnerState: shared.StateReady},
		NextState: StateSnapshot{ManagerInnerState: shared.StateReady},
	}

	effect := commander.CommandEffect(action)
	if effect != nil {
		t.Errorf("expected no effect for same-state transition, got %T", effect)
	}
}

// TestEffectCommander_CrashedIsTerminal tests Crashed state is terminal (returns nil).
func TestEffectCommander_CrashedIsTerminal(t *testing.T) {
	commander := NewEffectCommander()

	action := ReduceAction{
		PrevState: StateSnapshot{ManagerInnerState: shared.StateCrashed},
		NextState: StateSnapshot{ManagerInnerState: shared.StateReady},
	}

	effect := commander.CommandEffect(action)
	if effect != nil {
		t.Errorf("expected no effect from Crashed state, got %T", effect)
	}
}
